package threadstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Godric-W/Equus/internal/llm"
)

func TestCreateAppendLoadRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	writer, err := store.Create(Meta{Model: "glm", CWD: "/w", Title: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	id := writer.ID()
	if err := writer.Append(llm.UserMessage("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(llm.AssistantMessage("hi there")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	meta, items, err := store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != "glm" || meta.CWD != "/w" {
		t.Fatalf("meta = %+v", meta)
	}
	if len(items) != 2 || items[0].Role != llm.RoleUser || items[1].Content != "hi there" {
		t.Fatalf("items = %+v", items)
	}
}

func TestListNewestFirst(t *testing.T) {
	store := New(t.TempDir())
	first, _ := store.Create(Meta{ID: "a", Title: "first"})
	first.Close()
	second, _ := store.Create(Meta{ID: "b", Title: "second"})
	second.Close()

	metas, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("got %d sessions", len(metas))
	}
	// The most recently created session should sort first (CreatedAt desc).
	if !metas[0].CreatedAt.After(metas[1].CreatedAt) && !metas[0].CreatedAt.Equal(metas[1].CreatedAt) {
		t.Fatalf("order wrong: %v then %v", metas[0].CreatedAt, metas[1].CreatedAt)
	}
}

func TestLatest(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.Latest(); err == nil {
		t.Fatal("expected error when no sessions")
	}
	w, _ := store.Create(Meta{ID: "only", Title: "only"})
	w.Close()
	meta, err := store.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "only" {
		t.Fatalf("latest = %q", meta.ID)
	}
}

func TestLoadSkipsCorruptTrailingLine(t *testing.T) {
	store := New(t.TempDir())
	writer, _ := store.Create(Meta{ID: "s1"})
	writer.Append(llm.UserMessage("ok"))
	writer.Close()
	// Append a corrupt line.
	path := filepath.Join(store.Dir(), "s1.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("{not valid json\n")
	file.Close()

	_, items, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (corrupt line skipped)", len(items))
	}
}

func TestToolCallsRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	writer, _ := store.Create(Meta{ID: "s2"})
	writer.Append(llm.AssistantToolCallMessage("", llm.ToolCall{ID: "c1", Name: "shell", Arguments: []byte(`{"command":"ls"}`)}))
	writer.Close()

	_, items, err := store.Load("s2")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].ToolCalls) != 1 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].ToolCalls[0].Name != "shell" || string(items[0].ToolCalls[0].Arguments) != `{"command":"ls"}` {
		t.Fatalf("tool call = %+v", items[0].ToolCalls[0])
	}
}

func TestSetTitleAndDelete(t *testing.T) {
	store := New(t.TempDir())
	w, _ := store.Create(Meta{ID: "s1", Title: "chat"})
	w.Close()

	if err := store.SetTitle("s1", "my title"); err != nil {
		t.Fatal(err)
	}
	meta, _, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "my title" {
		t.Fatalf("title = %q", meta.Title)
	}

	if err := store.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	metas, _ := store.List()
	if len(metas) != 0 {
		t.Fatalf("session not deleted: %v", metas)
	}
}
