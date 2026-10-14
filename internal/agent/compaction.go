package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// compactionPrompt instructs the model to summarize earlier conversation so a
// resumed agent can continue (adapted from the reference agent's checkpoint
// prompt).
const compactionPrompt = "You are performing a CONTEXT CHECKPOINT COMPACTION. " +
	"Create a concise handoff summary of the conversation so far for another LLM that will resume the task. " +
	"Include: current progress and key decisions, important context/constraints/user preferences, " +
	"what remains to be done, and any critical data or examples needed to continue. " +
	"Output only the summary text."

// CompactionOptions controls automatic context compaction.
type CompactionOptions struct {
	Enabled         bool
	Threshold       float64 // fraction of context window that triggers compaction
	KeepRecentTurns int     // recent user turns kept verbatim
}

// defaultCompactionOptions mirrors the config defaults.
func defaultCompactionOptions() CompactionOptions {
	return CompactionOptions{Enabled: true, Threshold: 0.8, KeepRecentTurns: 4}
}

// maybeCompact summarizes older history when the last request approached the
// context window. It never fails the turn: on error it leaves history intact
// and reports the problem through the sink.
func (session *Session) maybeCompact(ctx context.Context, sink func(Event)) {
	options := session.compaction
	if !options.Enabled {
		return
	}
	window := session.contextWindow
	if window <= 0 {
		return
	}
	threshold := options.Threshold
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8
	}
	session.mu.Lock()
	used := session.lastPromptTokens
	history := append([]llm.ResponseItem(nil), session.history...)
	session.mu.Unlock()
	if int64(float64(window)*threshold) == 0 || used < int64(float64(window)*threshold) {
		return
	}

	keep := options.KeepRecentTurns
	if keep <= 0 {
		keep = 4
	}
	old, recent := splitHistory(history, keep)
	if len(old) == 0 {
		return
	}
	summary, err := session.summarize(ctx, old)
	if err != nil {
		sink(Event{Kind: EventError, Text: "compaction skipped: " + err.Error()})
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	compacted := make([]llm.ResponseItem, 0, len(recent)+1)
	compacted = append(compacted, llm.SystemMessage("Summary of the earlier conversation:\n"+summary))
	compacted = append(compacted, recent...)

	session.mu.Lock()
	session.history = compacted
	session.lastPromptTokens = 0 // force a fresh measurement next request
	session.mu.Unlock()
	sink(Event{Kind: EventCompaction, Text: fmt.Sprintf("compacted %d earlier messages", len(old))})
}

// summarize asks the model to summarize the given history items.
func (session *Session) summarize(ctx context.Context, items []llm.ResponseItem) (string, error) {
	prompt := llm.Prompt{
		Input:            append([]llm.ResponseItem(nil), items...),
		BaseInstructions: compactionPrompt,
	}
	response, err := session.client.Complete(ctx, llm.Request{Model: session.client.Model().Name, Prompt: prompt})
	if err != nil {
		return "", err
	}
	return response.Message.Content, nil
}

// splitHistory returns the older items (everything before the last `keep` user
// turns) and the recent items (the last `keep` user turns onward). Items after
// the cut are kept verbatim.
func splitHistory(history []llm.ResponseItem, keep int) (old, recent []llm.ResponseItem) {
	if keep <= 0 || len(history) == 0 {
		return nil, append([]llm.ResponseItem(nil), history...)
	}
	// Find the index of the user turn that starts the kept tail: the `keep`-th
	// user turn counting from the end.
	turnsSeen := 0
	cut := 0
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Role != llm.RoleUser {
			continue
		}
		turnsSeen++
		if turnsSeen == keep {
			cut = index
			break
		}
	}
	if cut == 0 || cut >= len(history) {
		return nil, append([]llm.ResponseItem(nil), history...)
	}
	old = append([]llm.ResponseItem(nil), history[:cut]...)
	recent = append([]llm.ResponseItem(nil), history[cut:]...)
	return old, recent
}
