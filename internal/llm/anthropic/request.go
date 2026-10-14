package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// messageRequest is the wire representation of a Messages API call.
type messageRequest struct {
	Model     string    `json:"model"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
	Tools     []toolDef `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens"`
	Stream    bool      `json:"stream,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock is one entry in a message's content array.
type contentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func (client *Client) newRequestBody(request llm.Request, stream bool) ([]byte, error) {
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = client.options.Model
	}
	messages, err := convertMessages(request.ConversationItems())
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errors.New("anthropic request messages are empty")
	}
	tools, err := convertTools(request.ToolSpecs())
	if err != nil {
		return nil, err
	}
	payload := messageRequest{
		Model:     model,
		System:    strings.TrimSpace(request.Prompt.BaseInstructions),
		Messages:  messages,
		Tools:     tools,
		MaxTokens: client.options.MaxTokens,
		Stream:    stream,
	}
	return marshal(payload)
}

// convertMessages maps the neutral history into Anthropic messages. Anthropic
// requires tool_result blocks to live in a user message and forbids empty
// content, so tool results are folded into a following user message.
func convertMessages(items []llm.ResponseItem) ([]message, error) {
	var messages []message
	for _, item := range items {
		switch item.Role {
		case llm.RoleSystem, llm.RoleDeveloper:
			// The system prompt is sent separately; skip stray system items.
			continue
		case llm.RoleUser:
			messages = append(messages, message{Role: "user", Content: []contentBlock{{Type: "text", Text: item.Content}}})
		case llm.RoleAssistant:
			blocks := make([]contentBlock, 0, 1+len(item.ToolCalls))
			if item.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: item.Content})
			}
			for _, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" {
					return nil, errors.New("assistant tool call id is empty")
				}
				input := call.Arguments
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: input})
			}
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, message{Role: "assistant", Content: blocks})
		case llm.RoleTool:
			if strings.TrimSpace(item.ToolCallID) == "" {
				return nil, errors.New("tool result call id is empty")
			}
			block := contentBlock{Type: "tool_result", ToolUseID: item.ToolCallID, Content: item.Content}
			// Append to the previous user message when it is a tool_result
			// batch; otherwise start a new user message.
			if n := len(messages); n > 0 && messages[n-1].Role == "user" && isToolResultMessage(messages[n-1]) {
				messages[n-1].Content = append(messages[n-1].Content, block)
			} else {
				messages = append(messages, message{Role: "user", Content: []contentBlock{block}})
			}
		default:
			return nil, fmt.Errorf("unsupported role %q", item.Role)
		}
	}
	return messages, nil
}

func isToolResultMessage(m message) bool {
	for _, block := range m.Content {
		if block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func convertTools(specs []llm.ToolSpec) ([]toolDef, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tools := make([]toolDef, 0, len(specs))
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
		tools = append(tools, toolDef{Name: spec.Name, Description: spec.Description, InputSchema: schema})
	}
	return tools, nil
}
