package llm

import (
	"encoding/json"
	"strings"
)

// Role enumerates the conversation roles used by the neutral message model.
type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ResponseItem is one entry of the provider-neutral conversation history.
type ResponseItem struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func SystemMessage(content string) ResponseItem {
	return ResponseItem{Role: RoleSystem, Content: content}
}

func UserMessage(content string) ResponseItem {
	return ResponseItem{Role: RoleUser, Content: content}
}

func AssistantMessage(content string) ResponseItem {
	return ResponseItem{Role: RoleAssistant, Content: content}
}

func AssistantToolCallMessage(content string, calls ...ToolCall) ResponseItem {
	return ResponseItem{Role: RoleAssistant, Content: content, ToolCalls: cloneToolCalls(calls)}
}

func ToolResultMessage(callID, content string) ResponseItem {
	return ResponseItem{Role: RoleTool, ToolCallID: callID, Content: content}
}

func cloneToolCalls(calls []ToolCall) []ToolCall {
	cloned := make([]ToolCall, len(calls))
	for index, call := range calls {
		cloned[index] = call
		cloned[index].Arguments = append(json.RawMessage(nil), call.Arguments...)
	}
	return cloned
}

func (role Role) Valid() bool {
	switch role {
	case RoleSystem, RoleDeveloper, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

// Display renders a short human-readable label for the role.
func (role Role) Display() string {
	switch role {
	case RoleSystem, RoleDeveloper:
		return "system"
	case RoleUser:
		return "you"
	case RoleAssistant:
		return "equus"
	case RoleTool:
		return "tool"
	default:
		return strings.ToLower(string(role))
	}
}
