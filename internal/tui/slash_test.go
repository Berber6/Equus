package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/llm"
)

func TestParseInputPlainText(t *testing.T) {
	result, err := parseInput("  hello world ")
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != nil || result.Text != "hello world" {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseInputSlashCommand(t *testing.T) {
	result, err := parseInput("/help")
	if err != nil {
		t.Fatal(err)
	}
	if result.Command == nil || result.Command.Command != slashHelp {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseInputUnknownCommand(t *testing.T) {
	if _, err := parseInput("/nope"); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestQuitAlias(t *testing.T) {
	if cmd, ok := findSlashCommand("/exit"); !ok || cmd != slashQuit {
		t.Fatalf("exit alias = %v %v", cmd, ok)
	}
}

func TestFormatSlashCommands(t *testing.T) {
	lines := formatSlashCommands()
	if len(lines) != len(slashCommands()) {
		t.Fatalf("lines = %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"/help", "/clear", "/compact", "/model", "/status", "/resume", "/quit"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %q", want, joined)
		}
	}
}

func TestDispatchHelpListsCommands(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	model.dispatchSlashCommand(slashInvocation{Command: slashHelp})
	if !strings.Contains(strings.Join(model.transcript, "\n"), "/help") {
		t.Fatalf("help output missing: %v", model.transcript)
	}
}

func TestDispatchClearResetsConversation(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	model.appendTranscript("some line")
	if len(model.session.History()) != 0 {
		// seed history via Ask is heavy; use Restore to simulate prior turns
	}
	model.session.Restore([]llm.ResponseItem{llm.UserMessage("x")})
	model.dispatchSlashCommand(slashInvocation{Command: slashClear})
	if len(model.session.History()) != 0 {
		t.Fatalf("history not cleared: %d", len(model.session.History()))
	}
	joined := strings.Join(model.transcript, "\n")
	if strings.Contains(joined, "some line") {
		t.Fatalf("old transcript not cleared: %q", joined)
	}
}

func TestDispatchQuitReturnsQuitCommand(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	command := model.dispatchSlashCommand(slashInvocation{Command: slashQuit})
	if command == nil {
		t.Fatal("expected a quit command")
	}
	if msg := command(); msg != tea.Quit() {
		t.Fatalf("expected tea.Quit, got %v", msg)
	}
}

func TestEnterDispatchesCommand(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	model.input.SetValue("/help")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.running {
		t.Fatal("/help must not start a model turn")
	}
	if !strings.Contains(strings.Join(model.transcript, "\n"), "/help") {
		t.Fatalf("help not shown: %v", model.transcript)
	}
}
