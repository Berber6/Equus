package llm

import "encoding/json"

// ToolSpec declares one tool offered to the model.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// Prompt bundles everything sent to the model besides the model name.
type Prompt struct {
	Input            []ResponseItem
	Tools            []ToolSpec
	BaseInstructions string
}

// Request is a full model request in the neutral domain.
type Request struct {
	Model  string
	Prompt Prompt
}

// NewRequest builds a minimal text-only request.
func NewRequest(model string, messages []ResponseItem) Request {
	return Request{
		Model:  model,
		Prompt: Prompt{Input: cloneResponseItems(messages)},
	}
}

// ConversationItems returns a defensive copy of the conversation history.
func (request Request) ConversationItems() []ResponseItem {
	return cloneResponseItems(request.Prompt.Input)
}

// ToolSpecs returns a defensive copy of the tool declarations.
func (request Request) ToolSpecs() []ToolSpec {
	cloned := make([]ToolSpec, len(request.Prompt.Tools))
	for index, definition := range request.Prompt.Tools {
		cloned[index] = definition
		cloned[index].InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	}
	return cloned
}

func cloneResponseItems(messages []ResponseItem) []ResponseItem {
	cloned := make([]ResponseItem, len(messages))
	for index, message := range messages {
		cloned[index] = message
		cloned[index].ToolCalls = cloneToolCalls(message.ToolCalls)
	}
	return cloned
}
