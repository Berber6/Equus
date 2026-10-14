package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Godric-W/Equus/internal/llm"
)

// fakeClient drives the session loop with scripted responses.
type fakeClient struct {
	responses     []llm.Response
	index         int
	requests      []llm.Request
	streamErr     error
	completeCalls int
}

func (client *fakeClient) Complete(_ context.Context, request llm.Request) (llm.Response, error) {
	client.completeCalls++
	client.requests = append(client.requests, request)
	if client.index >= len(client.responses) {
		return llm.Response{}, errors.New("no more scripted responses")
	}
	response := client.responses[client.index]
	client.index++
	return response, nil
}

func (client *fakeClient) Stream(ctx context.Context, request llm.Request) (llm.Stream, error) {
	if client.streamErr != nil {
		return nil, client.streamErr
	}
	response, err := client.Complete(ctx, request)
	if err != nil {
		return nil, err
	}
	return &fakeStream{chunks: []llm.StreamChunk{{
		ContentDelta: response.Message.Content,
		ToolCalls:    response.Message.ToolCalls,
		FinishReason: response.FinishReason,
	}}}, nil
}

func (client *fakeClient) Model() llm.ModelInfo {
	return llm.ModelInfo{Provider: "fake", Name: "fake-model"}
}

func (client *fakeClient) Capabilities() llm.Capabilities {
	return llm.Capabilities{SupportsStreaming: true, SupportsToolCalls: true}
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

func TestAskReturnsFinalAnswer(t *testing.T) {
	client := &fakeClient{responses: []llm.Response{
		{Message: llm.AssistantMessage("答案"), FinishReason: llm.FinishReasonStop},
	}}
	session, err := NewSession(client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := session.Ask(context.Background(), "问题", func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "答案" {
		t.Fatalf("answer = %q", answer)
	}
	if len(session.History()) != 2 {
		t.Fatalf("history length = %d", len(session.History()))
	}
}

func TestAskExecutesToolRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("42"), 0o644); err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"note.txt"}`)}
	client := &fakeClient{responses: []llm.Response{
		{Message: llm.AssistantToolCallMessage("", call), FinishReason: llm.FinishReasonToolCalls},
		{Message: llm.AssistantMessage("文件内容是 42"), FinishReason: llm.FinishReasonStop},
	}}
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(client, Options{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	answer, err := session.Ask(context.Background(), "note.txt 里是什么数字？", func(event Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if answer != "文件内容是 42" {
		t.Fatalf("answer = %q", answer)
	}
	var sawResult bool
	for _, event := range events {
		if event.Kind == EventToolResult && event.Tool == "read_file" && event.Text == "42" {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatalf("tool result event missing: %+v", events)
	}
	// history: user, assistant(tool_call), tool, assistant(final)
	history := session.History()
	if len(history) != 4 || history[2].Role != llm.RoleTool || history[2].Content != "42" {
		t.Fatalf("history = %+v", history)
	}
}

// nonRetryableError mimics a provider rejection: the stream call fails in a
// way that must not be retried on the blocking path.
type nonRetryableError struct{}

func (nonRetryableError) Error() string   { return "provider rejected request" }
func (nonRetryableError) Retryable() bool { return false }

func TestAskDoesNotFallBackOnTerminalStreamError(t *testing.T) {
	client := &fakeClient{streamErr: nonRetryableError{}}
	session, err := NewSession(client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.Ask(context.Background(), "hi", func(Event) {})
	if err == nil {
		t.Fatal("expected error to surface")
	}
	// The blocking fallback must not have been attempted.
	if client.completeCalls != 0 {
		t.Fatalf("Complete called %d times, want 0", client.completeCalls)
	}
}

func TestAskRejectsEmptyPrompt(t *testing.T) {
	session, err := NewSession(&fakeClient{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Ask(context.Background(), "   ", func(Event) {}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestWriteFileToolStaysInRoot(t *testing.T) {
	root := t.TempDir()
	tool := WriteFileTool{Root: root}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"../escape.txt","content":"x"}`)); err == nil {
		t.Fatal("expected path escape error")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"sub/dir/a.txt","content":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "sub/dir/a.txt") {
		t.Fatalf("result = %q", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "sub", "dir", "a.txt"))
	if err != nil || string(content) != "hello" {
		t.Fatalf("file content = %q err = %v", content, err)
	}
}

func TestMaxRoundTripsGuard(t *testing.T) {
	call := llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}
	// Every response asks for another tool call: the loop must stop.
	responses := make([]llm.Response, 32)
	for index := range responses {
		responses[index] = llm.Response{Message: llm.AssistantToolCallMessage("", call), FinishReason: llm.FinishReasonToolCalls}
	}
	client := &fakeClient{responses: responses}
	registry, err := NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(client, Options{Registry: registry, MaxRoundTrips: 3})
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.Ask(context.Background(), "loop", func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "round trips") {
		t.Fatalf("err = %v", err)
	}
}

func TestRestoreReplacesHistory(t *testing.T) {
	client := &fakeClient{responses: []llm.Response{{Message: llm.AssistantMessage("回答"), FinishReason: llm.FinishReasonStop}}}
	session, err := NewSession(client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	session.Restore([]llm.ResponseItem{llm.UserMessage("小明"), llm.AssistantMessage("你好小明")})
	if got := len(session.History()); got != 2 {
		t.Fatalf("history len = %d", got)
	}
	// A new turn builds on the restored history.
	if _, err := session.Ask(context.Background(), "我叫什么", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	history := session.History()
	if len(history) != 4 || history[0].Content != "小明" {
		t.Fatalf("history = %+v", history)
	}
}

func TestRecorderNotified(t *testing.T) {
	recorder := &recordingRecorder{}
	client := &fakeClient{responses: []llm.Response{{Message: llm.AssistantMessage("ok"), FinishReason: llm.FinishReasonStop}}}
	session, err := NewSession(client, Options{Recorder: recorder})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Ask(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	// user item + assistant item
	if len(recorder.items) != 2 {
		t.Fatalf("recorded %d items, want 2", len(recorder.items))
	}
}

type recordingRecorder struct{ items []llm.ResponseItem }

func (r *recordingRecorder) Record(item llm.ResponseItem) { r.items = append(r.items, item) }

func TestUsageTotalsAccumulate(t *testing.T) {
	session, err := NewSession(&fakeClient{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	session.recordUsage(llm.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15})
	session.recordUsage(llm.TokenUsage{InputTokens: 20, OutputTokens: 8, TotalTokens: 28})
	got := session.UsageTotals()
	if got.InputTokens != 30 || got.OutputTokens != 13 || got.TotalTokens != 43 {
		t.Fatalf("totals = %+v", got)
	}
}
