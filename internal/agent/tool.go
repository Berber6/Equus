// Package agent implements the minimal Equus agent loop: assemble a prompt
// from conversation history, call the model, execute requested tools, and
// repeat until the model produces a final answer.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is the contract every built-in tool implements.
type Tool interface {
	// Name is the identifier the model calls the tool by.
	Name() string
	// Description is the natural-language tool summary shown to the model.
	Description() string
	// InputSchema is the JSON schema of the tool arguments.
	InputSchema() json.RawMessage
	// Execute runs the tool and returns a textual result.
	Execute(ctx context.Context, arguments json.RawMessage) (string, error)
}

// toolRegistry collects the tools offered to the model.
type toolRegistry struct {
	tools []Tool
}

func (registry *toolRegistry) register(tool Tool) error {
	for _, existing := range registry.tools {
		if existing.Name() == tool.Name() {
			return fmt.Errorf("duplicate tool %q", tool.Name())
		}
	}
	registry.tools = append(registry.tools, tool)
	return nil
}

func (registry *toolRegistry) lookup(name string) (Tool, bool) {
	for _, tool := range registry.tools {
		if tool.Name() == name {
			return tool, true
		}
	}
	return nil, false
}

func (registry *toolRegistry) specs() []toolSpecView {
	views := make([]toolSpecView, 0, len(registry.tools))
	for _, tool := range registry.tools {
		views = append(views, toolSpecView{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}
	return views
}

// toolSpecView is the transport-facing projection of a Tool.
type toolSpecView struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}
