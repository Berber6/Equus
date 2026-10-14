package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolInfo is one tool offered by an MCP server.
type ToolInfo struct {
	// RemoteName is the name as the server knows it.
	RemoteName string
	// Description is the human-readable summary.
	Description string
	// InputSchema is the JSON Schema of the tool arguments.
	InputSchema json.RawMessage
}

// Session is a live connection to one MCP server.
type Session struct {
	Name    string
	Tools   []ToolInfo
	timeout time.Duration
	client  *client.Client
}

// Connect launches an MCP server over stdio and lists its tools.
func Connect(ctx context.Context, name string, config ServerConfig) (*Session, error) {
	if strings.TrimSpace(config.Command) == "" {
		return nil, fmt.Errorf("mcp server %q: command is empty", name)
	}
	env := environMap(config.Env)
	c, err := client.NewStdioMCPClient(config.Command, env, config.Args...)
	if err != nil {
		return nil, fmt.Errorf("mcp server %q: %w", name, err)
	}
	initCtx, cancel := context.WithTimeout(ctx, config.timeout())
	defer cancel()
	request := mcp.InitializeRequest{}
	request.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcp.Implementation{Name: "equus", Version: "0.1.0"}
	if _, err := c.Initialize(initCtx, request); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp server %q: initialize: %w", name, err)
	}
	tools, err := listTools(initCtx, c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp server %q: list tools: %w", name, err)
	}
	return &Session{Name: name, Tools: tools, timeout: config.timeout(), client: c}, nil
}

func listTools(ctx context.Context, c *client.Client) ([]ToolInfo, error) {
	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, tool := range result.Tools {
		schema := tool.RawInputSchema
		if len(schema) == 0 {
			if encoded, err := json.Marshal(tool.InputSchema); err == nil {
				schema = encoded
			}
		}
		tools = append(tools, ToolInfo{
			RemoteName:  tool.Name,
			Description: tool.Description,
			InputSchema: schema,
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].RemoteName < tools[j].RemoteName })
	return tools, nil
}

// Call invokes a tool by its remote name and returns the textual result.
func (s *Session) Call(ctx context.Context, remoteName string, arguments json.RawMessage) (string, error) {
	request := mcp.CallToolRequest{}
	request.Params.Name = remoteName
	if len(arguments) > 0 {
		var args map[string]any
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("mcp %s/%s: decode arguments: %w", s.Name, remoteName, err)
		}
		request.Params.Arguments = args
	}
	result, err := s.client.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("mcp %s/%s: %w", s.Name, remoteName, err)
	}
	return formatResult(result), nil
}

// Close terminates the MCP server subprocess.
func (s *Session) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// formatResult flattens an MCP tool result into text.
func formatResult(result *mcp.CallToolResult) string {
	var builder strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			builder.WriteString(text.Text)
		}
	}
	text := strings.TrimSpace(builder.String())
	if result.IsError && text != "" {
		return "error: " + text
	}
	if text == "" {
		return "(no output)"
	}
	return text
}

// environMap renders an env map as KEY=VALUE entries.
func environMap(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(values))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
