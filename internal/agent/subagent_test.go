package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Godric-W/Equus/internal/llm"
)

func TestReadOnlyRegistryExcludesWriters(t *testing.T) {
	registry, err := NewReadOnlyRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"write_file", "apply_patch", "shell", "web_fetch", "spawn_agent"} {
		if _, ok := registry.lookup(banned); ok {
			t.Errorf("read-only registry must not contain %q", banned)
		}
	}
	for _, allowed := range []string{"read_file", "list_files", "glob", "grep"} {
		if _, ok := registry.lookup(allowed); !ok {
			t.Errorf("read-only registry missing %q", allowed)
		}
	}
}

func TestEnableSubagentsRegistersTools(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	session, err := NewSession(&fakeClient{}, Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.EnableSubagents(t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	names := strings.Join(session.ToolNames(), ",")
	for _, want := range []string{"spawn_agent", "wait_agent", "close_agent"} {
		if !strings.Contains(names, want) {
			t.Errorf("missing tool %q in %s", want, names)
		}
	}
}

func TestSpawnAndWait(t *testing.T) {
	notified := make(chan string, 1)
	registry, _ := NewDefaultRegistry(t.TempDir())
	client := &fakeClient{responses: []llm.Response{{Message: llm.AssistantMessage("subagent result"), FinishReason: llm.FinishReasonStop}}}
	session, err := NewSession(client, Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	err = session.EnableSubagents(t.TempDir(), func(id, status, result string) {
		select {
		case notified <- result:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := session.options.Registry
	spawn, _ := manager.lookup("spawn_agent")
	out, err := spawn.Execute(context.Background(), json.RawMessage(`{"prompt":"do a thing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "agent-1") {
		t.Fatalf("spawn output = %q", out)
	}
	wait, _ := manager.lookup("wait_agent")
	done, err := wait.Execute(context.Background(), json.RawMessage(`{"id":"agent-1","timeout_seconds":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(done, "subagent result") {
		t.Fatalf("wait output = %q", done)
	}
	// The notification callback should fire with the result.
	select {
	case result := <-notified:
		if !strings.Contains(result, "subagent result") {
			t.Fatalf("notification result = %q", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification callback did not fire")
	}
}

func TestSpawnLimit(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	// A client whose Ask blocks until cancelled keeps subagents "running".
	client := &blockingClient{}
	session, err := NewSession(client, Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.EnableSubagents(t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	spawn, _ := session.options.Registry.lookup("spawn_agent")
	for i := 0; i < maxSubagents; i++ {
		if _, err := spawn.Execute(context.Background(), json.RawMessage(`{"prompt":"x"}`)); err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
	}
	if _, err := spawn.Execute(context.Background(), json.RawMessage(`{"prompt":"x"}`)); err == nil {
		t.Fatal("expected error when exceeding max subagents")
	}
}

func TestSpawnRejectsEmptyPrompt(t *testing.T) {
	registry, _ := NewDefaultRegistry(t.TempDir())
	session, _ := NewSession(&fakeClient{}, Options{Registry: registry})
	session.EnableSubagents(t.TempDir(), nil)
	spawn, _ := session.options.Registry.lookup("spawn_agent")
	if _, err := spawn.Execute(context.Background(), json.RawMessage(`{"prompt":"  "}`)); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

// blockingClient never returns from Stream/Complete, keeping a turn running.
type blockingClient struct{}

func (c *blockingClient) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}
func (c *blockingClient) Stream(ctx context.Context, _ llm.Request) (llm.Stream, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (c *blockingClient) Model() llm.ModelInfo { return llm.ModelInfo{Name: "blocking"} }
