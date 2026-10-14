package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestMain lets the test binary act as a fake MCP server when invoked with a
// sentinel argument (self-exec pattern).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "mcp-test-server" {
		runFakeServer()
		return
	}
	os.Exit(m.Run())
}

// runFakeServer runs a stdio MCP server exposing a single "echo" tool.
func runFakeServer() {
	s := server.NewMCPServer("fake", "1.0.0")
	s.AddTool(
		mcp.NewTool("echo", mcp.WithDescription("echo back"), mcp.WithString("text", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text := request.GetString("text", "")
			return mcp.NewToolResultText("echo: " + text), nil
		},
	)
	_ = server.ServeStdio(s)
}

// selfCommand returns the argv to re-invoke this test binary as a fake server.
func selfCommand() (string, []string) {
	return os.Args[0], []string{"mcp-test-server"}
}

func TestConnectListsTools(t *testing.T) {
	command, args := selfCommand()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := Connect(ctx, "fake", ServerConfig{Command: command, Args: args, Timeout: 8 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if len(session.Tools) != 1 || session.Tools[0].RemoteName != "echo" {
		t.Fatalf("tools = %+v", session.Tools)
	}

	out, err := session.Call(ctx, "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != "echo: hi" {
		t.Fatalf("call result = %q", out)
	}
}

func TestConnectMissingCommand(t *testing.T) {
	if _, err := Connect(context.Background(), "x", ServerConfig{}); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestConnectBadCommand(t *testing.T) {
	_, err := Connect(context.Background(), "x", ServerConfig{Command: "/nonexistent/binary", Timeout: time.Second})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
