package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// chatRequest is the wire representation of a Chat Completions call.
type chatRequest struct {
	Model         string         `json:"model"`
	Stream        bool           `json:"stream,omitempty"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

// streamOptions asks the endpoint to include a usage chunk when streaming, so
// token accounting (and compaction) works for streamed turns too.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	// Index is only used while aggregating streamed tool calls.
	Index    int              `json:"-"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatToolSpec `json:"function"`
}

type chatToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (client *Client) newRequestBody(request llm.Request, stream bool) ([]byte, error) {
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = client.options.Model
	}
	input := request.ConversationItems()
	if text := strings.TrimSpace(request.Prompt.BaseInstructions); text != "" {
		input = append([]llm.ResponseItem{llm.SystemMessage(text)}, input...)
	}
	if len(input) == 0 {
		return nil, errors.New("chat completions request messages are empty")
	}
	messages := make([]chatMessage, 0, len(input))
	for index, message := range input {
		converted, err := convertMessage(message)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", index, err)
		}
		messages = append(messages, converted)
	}
	tools, err := convertTools(request.ToolSpecs())
	if err != nil {
		return nil, err
	}
	payload := chatRequest{Model: model, Stream: stream, Messages: messages, Tools: tools}
	if stream {
		payload.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return json.Marshal(payload)
}

func convertMessage(message llm.ResponseItem) (chatMessage, error) {
	converted := chatMessage{Role: string(message.Role), Content: message.Content, ToolCallID: message.ToolCallID}
	switch message.Role {
	case llm.RoleSystem, llm.RoleDeveloper, llm.RoleUser:
		if len(message.ToolCalls) != 0 {
			return converted, errors.New("message cannot contain tool calls")
		}
	case llm.RoleAssistant:
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) == "" {
				return converted, errors.New("assistant tool call id is empty")
			}
			converted.ToolCalls = append(converted.ToolCalls, chatToolCall{
				ID: call.ID, Type: "function",
				Function: chatToolFunction{Name: call.Name, Arguments: string(call.Arguments)},
			})
		}
	case llm.RoleTool:
		if strings.TrimSpace(message.ToolCallID) == "" {
			return converted, errors.New("tool result call id is empty")
		}
	default:
		return converted, fmt.Errorf("unsupported role %q", message.Role)
	}
	return converted, nil
}

func convertTools(specs []llm.ToolSpec) ([]chatTool, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tools := make([]chatTool, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for index, spec := range specs {
		if strings.TrimSpace(spec.Name) == "" {
			return nil, fmt.Errorf("tools[%d]: name is empty", index)
		}
		if _, exists := seen[spec.Name]; exists {
			return nil, fmt.Errorf("tools[%d]: duplicate tool name %q", index, spec.Name)
		}
		seen[spec.Name] = struct{}{}
		schema := spec.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, chatTool{Type: "function", Function: chatToolSpec{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  schema,
		}})
	}
	return tools, nil
}
