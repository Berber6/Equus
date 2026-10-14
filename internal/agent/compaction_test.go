package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/Godric-W/Equus/internal/llm"
)

func TestSplitHistoryKeepsRecentTurns(t *testing.T) {
	history := []llm.ResponseItem{
		llm.UserMessage("u1"), llm.AssistantMessage("a1"),
		llm.UserMessage("u2"), llm.AssistantMessage("a2"),
		llm.UserMessage("u3"), llm.AssistantMessage("a3"),
	}
	old, recent := splitHistory(history, 2)
	// Keep the last 2 user turns: u2 onward.
	if len(old) != 2 || old[0].Content != "u1" {
		t.Fatalf("old = %+v", old)
	}
	if len(recent) != 4 || recent[0].Content != "u2" {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestSplitHistoryKeepsAllWhenTooShort(t *testing.T) {
	history := []llm.ResponseItem{llm.UserMessage("u1"), llm.AssistantMessage("a1")}
	old, recent := splitHistory(history, 4)
	if len(old) != 0 || len(recent) != 2 {
		t.Fatalf("old=%d recent=%d", len(old), len(recent))
	}
}

func TestMaybeCompactBelowThresholdNoop(t *testing.T) {
	client := &compactionClient{summary: "SUMMARY", window: 1000}
	session, err := NewSession(client, Options{
		Compaction: CompactionOptions{Enabled: true, Threshold: 0.8, KeepRecentTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Restore([]llm.ResponseItem{
		llm.UserMessage("u1"), llm.AssistantMessage("a1"),
		llm.UserMessage("u2"), llm.AssistantMessage("a2"),
	})
	session.recordUsage(llm.TokenUsage{InputTokens: 100}) // 10% < 80%
	var events []Event
	session.maybeCompact(context.Background(), func(e Event) { events = append(events, e) })
	if len(session.History()) != 4 {
		t.Fatalf("history compacted unexpectedly: %d", len(session.History()))
	}
	if len(events) != 0 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestMaybeCompactReplacesOlderHistory(t *testing.T) {
	client := &compactionClient{summary: "the user discussed topic X", window: 1000}
	session, err := NewSession(client, Options{
		Compaction: CompactionOptions{Enabled: true, Threshold: 0.8, KeepRecentTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Restore([]llm.ResponseItem{
		llm.UserMessage("old question"), llm.AssistantMessage("old answer"),
		llm.UserMessage("recent question"), llm.AssistantMessage("recent answer"),
	})
	session.recordUsage(llm.TokenUsage{InputTokens: 900}) // 90% > 80%
	var events []Event
	session.maybeCompact(context.Background(), func(e Event) { events = append(events, e) })

	history := session.History()
	// summary message + the kept recent turn
	if len(history) != 3 {
		t.Fatalf("history len = %d: %+v", len(history), history)
	}
	if history[0].Role != llm.RoleSystem || history[0].Content == "" {
		t.Fatalf("first item should be a summary: %+v", history[0])
	}
	if history[1].Content != "recent question" {
		t.Fatalf("recent turn not preserved: %+v", history[1])
	}
	var sawCompaction bool
	for _, e := range events {
		if e.Kind == EventCompaction {
			sawCompaction = true
		}
	}
	if !sawCompaction {
		t.Fatal("expected a compaction event")
	}
}

func TestMaybeCompactFailureKeepsHistory(t *testing.T) {
	client := &compactionClient{err: errors.New("model down"), window: 1000}
	session, err := NewSession(client, Options{
		Compaction: CompactionOptions{Enabled: true, Threshold: 0.8, KeepRecentTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := []llm.ResponseItem{
		llm.UserMessage("old"), llm.AssistantMessage("old a"),
		llm.UserMessage("recent"), llm.AssistantMessage("recent a"),
	}
	session.Restore(original)
	session.recordUsage(llm.TokenUsage{InputTokens: 950})
	var events []Event
	session.maybeCompact(context.Background(), func(e Event) { events = append(events, e) })

	if len(session.History()) != len(original) {
		t.Fatalf("history changed on failure: %+v", session.History())
	}
	var sawError bool
	for _, e := range events {
		if e.Kind == EventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("expected an error event on summarize failure")
	}
}

func TestMaybeCompactDisabled(t *testing.T) {
	client := &compactionClient{summary: "S", window: 1000}
	session, err := NewSession(client, Options{
		Compaction: CompactionOptions{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Restore([]llm.ResponseItem{llm.UserMessage("a"), llm.AssistantMessage("b"), llm.UserMessage("c"), llm.AssistantMessage("d")})
	session.recordUsage(llm.TokenUsage{InputTokens: 999})
	session.maybeCompact(context.Background(), func(Event) {})
	if len(session.History()) != 4 {
		t.Fatalf("disabled compaction still ran: %d", len(session.History()))
	}
}

// compactionClient returns a fixed summary from Complete.
type compactionClient struct {
	summary string
	err     error
	window  int64
}

func (c *compactionClient) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	if c.err != nil {
		return llm.Response{}, c.err
	}
	return llm.Response{Message: llm.AssistantMessage(c.summary), FinishReason: llm.FinishReasonStop}, nil
}

func (c *compactionClient) Stream(ctx context.Context, request llm.Request) (llm.Stream, error) {
	response, err := c.Complete(ctx, request)
	if err != nil {
		return nil, err
	}
	return &fakeStream{chunks: []llm.StreamChunk{{ContentDelta: response.Message.Content, FinishReason: response.FinishReason}}}, nil
}

func (c *compactionClient) Model() llm.ModelInfo {
	return llm.ModelInfo{Provider: "fake", Name: "fake", ContextWindow: c.window}
}
