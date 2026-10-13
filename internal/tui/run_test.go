package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/llm"
)

func TestIsTerminalRejectsNonFile(t *testing.T) {
	if IsTerminal(nil) {
		t.Fatal("nil reader must not be a terminal")
	}
	file, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if IsTerminal(file) {
		t.Fatal("regular file must not be a terminal")
	}
}

func TestRunRejectsNonTerminal(t *testing.T) {
	session := newTestSession(t, "answer")
	reader, writer := io.Pipe()
	defer writer.Close()
	err := Run(context.Background(), RunOptions{Session: session, Input: reader, Output: io.Discard})
	if !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("err = %v, want %v", err, ErrNotTerminal)
	}
}

func TestRunRejectsNilSession(t *testing.T) {
	if err := Run(context.Background(), RunOptions{}); err == nil {
		t.Fatal("expected error for nil session")
	}
}

func TestUpdateCommitAndQuit(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi"), Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	model = updated.(Model)
	if got := model.input.Value(); got != "hello" {
		t.Fatalf("input = %q", got)
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.running {
		t.Fatal("expected turn to be running after enter")
	}
	if model.input.Value() != "" {
		t.Fatalf("input not cleared: %q", model.input.Value())
	}
	if command == nil {
		t.Fatal("expected a command to echo the user line and start the turn")
	}
	if !strings.Contains(model.userLine("hello"), "hello") {
		t.Fatal("user line rendering lost the prompt text")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(Model)
	if model.input.Value() != "" {
		t.Fatal("ctrl+d with empty input must not alter input")
	}
}

func TestBackspaceDeletesCharacter(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if got := model.input.Value(); got != "ab" {
		t.Fatalf("after backspace input = %q, want %q", got, "ab")
	}
}

// TestUpdateHandlesContentDeltaViaCopy exercises the value-copy path Bubble
// Tea takes: Update receives the model by value and a streaming delta must not
// corrupt shared state (regression for the strings.Builder copy panic).
func TestUpdateHandlesContentDeltaViaCopy(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := model.Update(eventMsg{event: agent.Event{Kind: agent.EventContentDelta, Text: "你好"}})
	model = updated.(Model)
	updated, _ = model.Update(eventMsg{event: agent.Event{Kind: agent.EventContentDelta, Text: "世界"}})
	model = updated.(Model)
	if model.active != "你好世界" {
		t.Fatalf("active = %q, want %q", model.active, "你好世界")
	}
}

func TestFinishTurnFlushesAnswer(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.running = true
	model.handleEvent(agent.Event{Kind: agent.EventContentDelta, Text: "你好"})
	if model.active != "你好" {
		t.Fatalf("active = %q, want %q", model.active, "你好")
	}
	command := model.finishTurn(turnDoneMsg{answer: "你好"})
	if model.running {
		t.Fatal("running should be false after turn done")
	}
	if command == nil {
		t.Fatal("expected a print command for the committed answer")
	}
	if model.active != "" {
		t.Fatalf("active buffer not reset: %q", model.active)
	}
	if !strings.Contains(model.assistantLine("你好"), "你好") {
		t.Fatal("assistant line rendering lost the answer")
	}
}

// newTestSession builds a real agent.Session backed by a scripted client.
func newTestSession(t *testing.T, answer string) *agent.Session {
	t.Helper()
	registry, err := agent.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := agent.NewSession(&fakeClient{answer: answer}, agent.Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// fakeClient is a scripted llm.Client returning a fixed answer.
type fakeClient struct{ answer string }

func (client *fakeClient) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	return llm.Response{
		Message:      llm.AssistantMessage(client.answer),
		FinishReason: llm.FinishReasonStop,
	}, nil
}

func (client *fakeClient) Stream(ctx context.Context, request llm.Request) (llm.Stream, error) {
	response, err := client.Complete(ctx, request)
	if err != nil {
		return nil, err
	}
	return &fakeStream{chunks: []llm.StreamChunk{{
		ContentDelta: response.Message.Content,
		FinishReason: response.FinishReason,
	}}}, nil
}

func (client *fakeClient) Model() llm.ModelInfo {
	return llm.ModelInfo{Provider: "fake", Name: "fake-model"}
}

type fakeStream struct {
	chunks []llm.StreamChunk
	index  int
}

func (stream *fakeStream) Recv() (llm.StreamChunk, error) {
	if stream.index >= len(stream.chunks) {
		return llm.StreamChunk{}, io.EOF
	}
	chunk := stream.chunks[stream.index]
	stream.index++
	return chunk, nil
}

func (stream *fakeStream) Close() error { return nil }
