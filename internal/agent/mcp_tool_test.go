package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeCaller struct {
	gotName string
	gotArgs string
	result  string
}

func (c *fakeCaller) Call(_ context.Context, remoteName string, arguments json.RawMessage) (string, error) {
	c.gotName = remoteName
	c.gotArgs = string(arguments)
	return c.result, nil
}

func TestMCPToolNaming(t *testing.T) {
	tool := MCPTool{Server: "fs", RemoteName: "read_file"}
	if tool.Name() != "mcp__fs__read_file" {
		t.Fatalf("name = %q", tool.Name())
	}
}

func TestMCPToolApprovalKind(t *testing.T) {
	tool := MCPTool{Server: "fs", RemoteName: "read"}
	if tool.ApprovalKind() != approvalKindExternal {
		t.Fatalf("kind = %q", tool.ApprovalKind())
	}
	if summary, ok := tool.ApprovalSummary(nil); !ok || summary != "mcp fs/read" {
		t.Fatalf("summary = %q %v", summary, ok)
	}
}

func TestMCPToolExecuteBridges(t *testing.T) {
	caller := &fakeCaller{result: "ok"}
	tool := MCPTool{Server: "fs", RemoteName: "read", Caller: caller}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" || caller.gotName != "read" || caller.gotArgs != `{"path":"a"}` {
		t.Fatalf("bridge = %q %q %q", result, caller.gotName, caller.gotArgs)
	}
}

func TestRegisterMCPTools(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	session, err := NewSession(&fakeClient{}, Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	err = session.RegisterMCPTools("fs", []MCPToolSpec{
		{Name: "read", Description: "read a file", InputSchema: json.RawMessage(`{"type":"object"}`), Caller: &fakeCaller{}},
		{Name: "write", InputSchema: json.RawMessage(`{"type":"object"}`), Caller: &fakeCaller{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.lookup("mcp__fs__read"); !ok {
		t.Fatal("mcp tool not registered")
	}
	names := strings.Join(session.ToolNames(), ",")
	if !strings.Contains(names, "mcp__fs__read") || !strings.Contains(names, "mcp__fs__write") {
		t.Fatalf("tool names = %s", names)
	}
}
