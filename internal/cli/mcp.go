package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/mcp"
)

// connectMCPServers starts every enabled MCP server and registers its tools on
// the session. Failures are reported to stderr and skipped.
func connectMCPServers(ctx context.Context, configured config.Config, session *agent.Session, stderr io.Writer) []*mcp.Session {
	if len(configured.MCP.Servers) == 0 {
		return nil
	}
	names := make([]string, 0, len(configured.MCP.Servers))
	for name := range configured.MCP.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	var sessions []*mcp.Session
	for _, name := range names {
		server := configured.MCP.Servers[name]
		if !server.IsEnabled() {
			continue
		}
		connection, err := mcp.Connect(ctx, name, mcp.ServerConfig{
			Command: server.Command,
			Args:    server.Args,
			Env:     server.Env,
			Timeout: server.Timeout,
		})
		if err != nil {
			fmt.Fprintf(stderr, "equus: mcp server %q skipped: %v\n", name, err)
			continue
		}
		specs := make([]agent.MCPToolSpec, 0, len(connection.Tools))
		for _, tool := range connection.Tools {
			specs = append(specs, agent.MCPToolSpec{
				Name:        tool.RemoteName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
				Caller:      connection,
			})
		}
		if err := session.RegisterMCPTools(name, specs); err != nil {
			fmt.Fprintf(stderr, "equus: mcp server %q tool registration failed: %v\n", name, err)
			connection.Close()
			continue
		}
		sessions = append(sessions, connection)
	}
	return sessions
}

// closeMCPServers terminates all connected MCP server subprocesses.
func closeMCPServers(sessions []*mcp.Session) {
	for _, session := range sessions {
		session.Close()
	}
}
