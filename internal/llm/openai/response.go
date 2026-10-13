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

// ProviderError is a terminal error returned by the upstream endpoint: the
// transport succeeded but the server rejected the request (non-2xx status).
// It is deterministic — retrying the same request against the same endpoint
// would fail identically — so callers must not fall back to another wire
// format on seeing it.
type ProviderError struct {
	StatusCode int
	Body       string
}

func (err *ProviderError) Error() string {
	return fmt.Sprintf("openai provider error %d: %s", err.StatusCode, truncate(err.Body, 512))
}

// Retryable reports whether the error is worth retrying in another form.
// A definitive provider rejection is not.
func (err *ProviderError) Retryable() bool { return false }

func providerError(status int, payload []byte) error {
	return &ProviderError{StatusCode: status, Body: string(payload)}
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
