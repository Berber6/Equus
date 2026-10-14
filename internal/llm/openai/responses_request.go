package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// responsesRequest is the wire representation of a Responses API call.
type responsesRequest struct {
	Model        string          `json:"model"`
	Instructions string          `json:"instructions,omitempty"`
	Input        []responsesItem `json:"input"`
	Tools        []responsesTool `json:"tools,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
}

// responsesItem is one entry of the Responses "input" list.
type responsesItem struct {
	Type    string             `json:"type"`
	Role    string             `json:"role,omitempty"`
	Content []responsesContent `json:"content,omitempty"`
	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// function_call_output
	Output string `json:"output,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// newResponsesBodyStream builds a streaming Responses request body.
func (client *Client) newResponsesBodyStream(request llm.Request) ([]byte, error) {
	return client.newResponsesBodyWithStream(request, true)
}

// newResponsesBody builds the request body for the Responses dialect.
func (client *Client) newResponsesBody(request llm.Request) ([]byte, error) {
	return client.newResponsesBodyWithStream(request, false)
}

func (client *Client) newResponsesBodyWithStream(request llm.Request, stream bool) ([]byte, error) {
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = client.options.Model
	}
	input, err := responsesInputItems(request.ConversationItems())
	if err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, errors.New("responses request input is empty")
	}
	tools, err := responsesTools(request.ToolSpecs())
	if err != nil {
		return nil, err
	}
	return json.Marshal(responsesRequest{
		Model:        model,
		Instructions: strings.TrimSpace(request.Prompt.BaseInstructions),
		Input:        input,
		Tools:        tools,
		Stream:       stream,
	})
}

// responsesInputItems converts neutral history into Responses input items.
func responsesInputItems(items []llm.ResponseItem) ([]responsesItem, error) {
	var out []responsesItem
	for _, item := range items {
		switch item.Role {
		case llm.RoleSystem, llm.RoleDeveloper:
			continue // carried by Instructions
		case llm.RoleUser:
			out = append(out, responsesItem{Type: "message", Role: "user", Content: []responsesContent{{Type: "input_text", Text: item.Content}}})
		case llm.RoleAssistant:
			if item.Content != "" {
				out = append(out, responsesItem{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: item.Content}}})
			}
			for _, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" {
					return nil, errors.New("assistant tool call id is empty")
				}
				args := string(call.Arguments)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				out = append(out, responsesItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: args})
			}
		case llm.RoleTool:
			if strings.TrimSpace(item.ToolCallID) == "" {
				return nil, errors.New("tool result call id is empty")
			}
			out = append(out, responsesItem{Type: "function_call_output", CallID: item.ToolCallID, Output: item.Content})
		default:
			return nil, fmt.Errorf("unsupported role %q", item.Role)
		}
	}
	return out, nil
}

func responsesTools(specs []llm.ToolSpec) ([]responsesTool, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tools := make([]responsesTool, 0, len(specs))
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
		tools = append(tools, responsesTool{Type: "function", Name: spec.Name, Description: spec.Description, Parameters: schema})
	}
	return tools, nil
}
