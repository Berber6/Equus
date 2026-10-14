package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

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
	if joined := strings.Join(model.userEntry("hello"), "\n"); !strings.Contains(joined, "hello") {
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

// The composer uses a fixed content height. Resizing the textarea on every
// keystroke leaves its internal viewport YOffset stale, which scrolls the first
// line out of view (so only the last line is visible) — the bug this guards.
func TestInputHeightIsFixed(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	for _, r := range "one" {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	if got := model.input.Height(); got != maxInputRows {
		t.Fatalf("height = %d, want %d", got, maxInputRows)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	model = updated.(Model)
	if got := model.input.Height(); got != maxInputRows {
		t.Fatalf("height after newline = %d, want %d", got, maxInputRows)
	}
}

func TestViewLinesPaddedToWidth(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(40, 24)
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if width := xansi.StringWidth(line); width != 40 {
			t.Fatalf("line width = %d, want 40: %q", width, line)
		}
	}
}

func TestRenderPrefixes(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	if got := stripANSI(strings.Join(model.userEntry("hello"), "\n")); !strings.Contains(got, "› hello") {
		t.Fatalf("userEntry = %q", got)
	}
	if got := stripANSI(model.assistantLine("answer")); !strings.Contains(got, "• answer") {
		t.Fatalf("assistantLine = %q", got)
	}
	if got := stripANSI(strings.Join(model.toolCallEntry("grep", "pattern"), "\n")); !strings.Contains(got, "Ran grep") {
		t.Fatalf("toolCallEntry = %q", got)
	}
	if got := stripANSI(strings.Join(model.toolResultEntry("grep", "a.go:1: x"), "\n")); !strings.Contains(got, "└ grep: a.go:1: x") {
		t.Fatalf("toolResultEntry = %q", got)
	}
}

func TestStatusLineShowsModelAndCWD(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi"), Model: "glm-5.3", CWD: "/tmp/work"})
	if err != nil {
		t.Fatal(err)
	}
	model.resize(80, 24)
	got := stripANSI(model.statusLine())
	if !strings.Contains(got, "glm-5.3") || !strings.Contains(got, "/tmp/work") || !strings.Contains(got, "ready") {
		t.Fatalf("statusLine = %q", got)
	}
	model.running = true
	if got := stripANSI(model.statusLine()); !strings.Contains(got, "working") {
		t.Fatalf("statusLine while running = %q", got)
	}
}

// stripANSI removes SGR escape sequences for assertion-friendly comparison.
func stripANSI(s string) string {
	return xansi.Strip(s)
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
	model.finishTurn(turnDoneMsg{answer: "你好"})
	if model.running {
		t.Fatal("running should be false after turn done")
	}
	if model.active != "" {
		t.Fatalf("active buffer not reset: %q", model.active)
	}
	if joined := strings.Join(model.transcript, "\n"); !strings.Contains(joined, "你好") {
		t.Fatalf("answer not committed to transcript: %v", model.transcript)
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
