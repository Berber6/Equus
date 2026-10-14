package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/Godric-W/Equus/internal/llm"
)

// messageResponse is the wire representation of a non-streaming reply.
type messageResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

func decodeMessageResponse(payload []byte) (llm.Response, error) {
	var decoded messageResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return llm.Response{}, fmt.Errorf("decode message response: %w", err)
	}
	message := llm.ResponseItem{Role: llm.RoleAssistant}
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			message.Content += block.Text
		case "tool_use":
			input := block.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			message.ToolCalls = append(message.ToolCalls, llm.ToolCall{ID: block.ID, Name: block.Name, Arguments: input})
		}
	}
	response := llm.Response{
		ID:                   decoded.ID,
		Message:              message,
		FinishReason:         mapStopReason(decoded.StopReason),
		ProviderFinishReason: decoded.StopReason,
	}
	if decoded.Usage != nil {
		response.TokenUsage = llm.TokenUsage{
			InputTokens:  decoded.Usage.InputTokens,
			OutputTokens: decoded.Usage.OutputTokens,
			TotalTokens:  decoded.Usage.InputTokens + decoded.Usage.OutputTokens,
		}
	}
	return response, nil
}

func mapStopReason(reason string) llm.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return llm.FinishReasonStop
	case "max_tokens":
		return llm.FinishReasonLength
	case "tool_use":
		return llm.FinishReasonToolCalls
	case "":
		return ""
	default:
		return llm.FinishReasonUnknown
	}
}
