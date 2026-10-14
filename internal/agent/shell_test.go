package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Godric-W/Equus/internal/llm"
)

func TestShellToolRunsCommand(t *testing.T) {
	tool := ShellTool{Root: t.TempDir()}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "exit code: 0") || !strings.Contains(result, "hello") {
		t.Fatalf("result = %q", result)
	}
}

func TestShellToolReportsExitCode(t *testing.T) {
	tool := ShellTool{Root: t.TempDir()}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"exit 3"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "exit code: 3") {
		t.Fatalf("result = %q", result)
	}
}

func TestShellToolTimeout(t *testing.T) {
	tool := ShellTool{Root: t.TempDir()}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 5","timeout_seconds":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "timed out") {
		t.Fatalf("result = %q", result)
	}
}

func TestShellToolTruncatesOutput(t *testing.T) {
	tool := ShellTool{Root: t.TempDir()}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"head -c 40000 /dev/zero | tr '\\0' 'a'"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "truncated") {
		t.Fatalf("expected truncation notice, got len=%d", len(result))
	}
}

func TestShellToolRunsInRoot(t *testing.T) {
	root := t.TempDir()
	tool := ShellTool{Root: root}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, root) {
		t.Fatalf("pwd not in root: %q", result)
	}
}

// TestApprovalDeniedBlocksExecution verifies a denied operation never runs.
func TestApprovalDeniedBlocksExecution(t *testing.T) {
	root := t.TempDir()
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeAsk },
		Approver: func(context.Context, ApprovalRequest) (ApprovalDecision, error) {
			return ApprovalDenied, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := session.runTool(context.Background(),
		mustCall(t, "shell", `{"command":"touch should-not-exist"}`), func(Event) {})
	if !strings.Contains(result, "denied") {
		t.Fatalf("result = %q", result)
	}
}

func TestApprovalAllowRuns(t *testing.T) {
	root := t.TempDir()
	registry, _ := NewDefaultRegistry(root)
	called := 0
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeAsk },
		Approver: func(context.Context, ApprovalRequest) (ApprovalDecision, error) {
			called++
			return ApprovalOnce, nil
		},
	})
	result := session.runTool(context.Background(),
		mustCall(t, "shell", `{"command":"echo ran"}`), func(Event) {})
	if called != 1 {
		t.Fatalf("approver called %d times", called)
	}
	if !strings.Contains(result, "ran") {
		t.Fatalf("result = %q", result)
	}
}

func TestApprovalSessionNotReAsked(t *testing.T) {
	root := t.TempDir()
	registry, _ := NewDefaultRegistry(root)
	called := 0
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeAsk },
		Approver: func(context.Context, ApprovalRequest) (ApprovalDecision, error) {
			called++
			return ApprovalSession, nil
		},
	})
	for i := 0; i < 3; i++ {
		session.runTool(context.Background(),
			mustCall(t, "shell", `{"command":"echo hi"}`), func(Event) {})
	}
	if called != 1 {
		t.Fatalf("approver called %d times, want 1 (session grant)", called)
	}
}

func TestApprovalModeAllowSkipsPrompt(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	called := 0
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeAllow },
		Approver: func(context.Context, ApprovalRequest) (ApprovalDecision, error) {
			called++
			return ApprovalOnce, nil
		},
	})
	session.runTool(context.Background(), mustCall(t, "shell", `{"command":"echo x"}`), func(Event) {})
	if called != 0 {
		t.Fatalf("approver should not be called when policy allows: %d", called)
	}
}

func TestApprovalModeDenyRejects(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeDeny },
	})
	result := session.runTool(context.Background(), mustCall(t, "shell", `{"command":"echo x"}`), func(Event) {})
	if !strings.Contains(result, "denied") {
		t.Fatalf("result = %q", result)
	}
}

func TestNoApproverDenies(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(string) ApprovalMode { return ApprovalModeAsk },
	})
	result := session.runTool(context.Background(), mustCall(t, "shell", `{"command":"echo x"}`), func(Event) {})
	if !strings.Contains(result, "approval required") {
		t.Fatalf("result = %q", result)
	}
}

func TestWriteToolNeedsNoApprovalWhenAllowed(t *testing.T) {
	root := t.TempDir()
	registry, _ := NewDefaultRegistry(root)
	session, _ := NewSession(&fakeClient{}, Options{
		Registry:     registry,
		ApprovalMode: func(category string) ApprovalMode { return ApprovalModeAllow },
	})
	result := session.runTool(context.Background(),
		mustCall(t, "write_file", `{"path":"a.txt","content":"x"}`), func(Event) {})
	if strings.Contains(result, "denied") {
		t.Fatalf("write should be allowed: %q", result)
	}
}

func mustCall(t *testing.T, name, arguments string) llm.ToolCall {
	t.Helper()
	return llm.ToolCall{ID: "call_1", Name: name, Arguments: json.RawMessage(arguments)}
}
