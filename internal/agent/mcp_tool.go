package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPCaller invokes a tool on an MCP server. It is implemented by
// mcp.Session (kept as an interface so the agent package does not depend on
// mcp directly).
type MCPCaller interface {
	Call(ctx context.Context, remoteName string, arguments json.RawMessage) (string, error)
}

// MCPTool bridges one MCP server tool into the agent tool set. Its name is
// namespaced as mcp__<server>__<tool> to avoid clashing with built-ins.
type MCPTool struct {
	Server          string
	RemoteName      string
	DescriptionText string
	Schema          json.RawMessage
	Caller          MCPCaller
}

func (tool MCPTool) Name() string {
	return mcpToolName(tool.Server, tool.RemoteName)
}

func (tool MCPTool) Description() string {
	if strings.TrimSpace(tool.DescriptionText) != "" {
		return tool.DescriptionText
	}
	return fmt.Sprintf("MCP tool %q from server %q", tool.RemoteName, tool.Server)
}

func (tool MCPTool) InputSchema() json.RawMessage {
	if len(tool.Schema) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return tool.Schema
}

// ApprovalKind reports the policy category for this tool.
func (tool MCPTool) ApprovalKind() approvalKind { return approvalKindExternal }

// ApprovalSummary returns the server/tool for the approval prompt.
func (tool MCPTool) ApprovalSummary(arguments []byte) (string, bool) {
	return fmt.Sprintf("mcp %s/%s", tool.Server, tool.RemoteName), true
}

func (tool MCPTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	if tool.Caller == nil {
		return "", fmt.Errorf("mcp tool %s has no caller", tool.RemoteName)
	}
	return tool.Caller.Call(ctx, tool.RemoteName, arguments)
}

// mcpToolName builds the namespaced tool name exposed to the model.
func mcpToolName(server, remote string) string {
	return "mcp__" + sanitizeToolSegment(server) + "__" + sanitizeToolSegment(remote)
}

// sanitizeToolSegment replaces characters that may confuse tool-name parsing.
func sanitizeToolSegment(value string) string {
	return strings.NewReplacer(" ", "_", "__", "_").Replace(strings.TrimSpace(value))
}

// RegisterMCPTools adds every tool from the given MCP server to the session's
// registry, namespacing each by server name.
func (session *Session) RegisterMCPTools(server string, tools []MCPToolSpec) error {
	session.mu.Lock()
	registry := session.options.Registry
	session.mu.Unlock()
	if registry == nil {
		return fmt.Errorf("session has no tool registry")
	}
	for _, spec := range tools {
		err := registry.register(MCPTool{
			Server:          server,
			RemoteName:      spec.Name,
			DescriptionText: spec.Description,
			Schema:          spec.InputSchema,
			Caller:          spec.Caller,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// MCPToolSpec describes one MCP tool to register.
type MCPToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Caller      MCPCaller
}
