package openai

import (
	"encoding/json"
	"fmt"

	"github.com/Godric-W/Equus/internal/llm"
)

// responsesResponse is the non-streaming Responses API reply.
type responsesResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"output"`
	Usage *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

func decodeResponsesResponse(payload []byte) (llm.Response, error) {
	var decoded responsesResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return llm.Response{}, fmt.Errorf("decode responses response: %w", err)
	}
	message := llm.ResponseItem{Role: llm.RoleAssistant}
	for _, item := range decoded.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					message.Content += content.Text
				}
			}
		case "function_call":
			args := item.Arguments
			if args == "" {
				args = "{}"
			}
			message.ToolCalls = append(message.ToolCalls, llm.ToolCall{ID: item.CallID, Name: item.Name, Arguments: json.RawMessage(args)})
		}
	}
	response := llm.Response{
		ID:           decoded.ID,
		Message:      message,
		FinishReason: responsesFinishReason(message),
	}
	if decoded.Usage != nil {
		response.TokenUsage = llm.TokenUsage{
			InputTokens:  decoded.Usage.InputTokens,
			OutputTokens: decoded.Usage.OutputTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		}
	}
	return response, nil
}

// responsesFinishReason derives a finish reason from the output shape.
func responsesFinishReason(message llm.ResponseItem) llm.FinishReason {
	if len(message.ToolCalls) > 0 {
		return llm.FinishReasonToolCalls
	}
	return llm.FinishReasonStop
}
