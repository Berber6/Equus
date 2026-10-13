package openai

import (
	"encoding/json"
	"fmt"

	"github.com/Godric-W/Equus/internal/llm"
)

const maxResponseBytes = int64(32 << 20)

// completionResponse is the wire representation of a non-streaming reply.
type completionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role             string         `json:"role"`
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning_content"`
			ToolCalls        []chatToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

func decodeCompletionResponse(payload []byte) (llm.Response, error) {
	var decoded completionResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return llm.Response{}, fmt.Errorf("decode completion response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("completion response has no choices")
	}
	choice := decoded.Choices[0]
	message := llm.ResponseItem{Role: llm.RoleAssistant, Content: choice.Message.Content}
	for _, call := range choice.Message.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, llm.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	response := llm.Response{
		ID:                   decoded.ID,
		Message:              message,
		FinishReason:         mapFinishReason(choice.FinishReason),
		ProviderFinishReason: choice.FinishReason,
	}
	if decoded.Usage != nil {
		response.TokenUsage = llm.TokenUsage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		}
	}
	return response, nil
}

func mapFinishReason(reason string) llm.FinishReason {
	switch reason {
	case "stop":
		return llm.FinishReasonStop
	case "length":
		return llm.FinishReasonLength
	case "tool_calls", "function_call":
		return llm.FinishReasonToolCalls
	case "content_filter":
		return llm.FinishReasonContentFilter
	case "":
		return ""
	default:
		return llm.FinishReasonUnknown
	}
}

func providerError(status int, payload []byte) error {
	return fmt.Errorf("openai provider error %d: %s", status, truncate(string(payload), 512))
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
