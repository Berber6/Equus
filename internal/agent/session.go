package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Godric-W/Equus/internal/llm"
)

// DefaultBaseInstructions is the system prompt used for fresh sessions.
const DefaultBaseInstructions = "You are Equus, a minimal terminal coding agent written in Go. " +
	"Answer concisely. Use the provided tools when working with files."

// maxToolRoundTrips bounds the tool-call loop per user turn.
const maxToolRoundTrips = 16

// Options configures a Session.
type Options struct {
	BaseInstructions string
	Registry         *toolRegistry
	MaxRoundTrips    int
	// Approver decides whether an operation requiring approval may proceed.
	// A nil approver denies every approval-requiring operation.
	Approver Approver
	// ApprovalMode resolves the policy for a category ("shell", "write"):
	// ApprovalModeAsk / Allow / Deny.
	ApprovalMode func(category string) ApprovalMode
	// AutoApprove, when set, reports whether a specific operation (by category
	// and its summary) should skip the prompt — e.g. a shell command matching
	// an allowed prefix.
	AutoApprove func(category, summary string) bool
	// OnPermanentGrant, when set, is called when the user grants persistent
	// approval for an operation (so it can be recorded).
	OnPermanentGrant func(category, summary string) error
	// Recorder, when set, is notified of every conversation item appended to
	// history (for persisting the session).
	Recorder Recorder
	// Compaction controls automatic context compaction.
	Compaction CompactionOptions
}

// Recorder receives conversation items as they are appended to history.
type Recorder interface {
	Record(item llm.ResponseItem)
}

// Event is a progress notification emitted while the session runs.
type Event struct {
	Kind  EventKind
	Text  string
	Tool  string
	Usage *llm.TokenUsage
}

// EventKind enumerates session event categories.
type EventKind string

const (
	EventReasoningDelta EventKind = "reasoning_delta"
	EventContentDelta   EventKind = "content_delta"
	EventToolCall       EventKind = "tool_call"
	EventToolResult     EventKind = "tool_result"
	EventUsage          EventKind = "usage"
	EventError          EventKind = "error"
	EventApproval       EventKind = "approval"
	EventCompaction     EventKind = "compaction"
)

// Session keeps conversation history and drives one agent turn at a time.
type Session struct {
	client  llm.Client
	options Options

	mu      sync.Mutex
	history []llm.ResponseItem
	// approved caches categories granted "for session" so later identical
	// categories are not re-prompted.
	approved map[approvalKind]bool
	// contextWindow is the model's context size; lastPromptTokens is the most
	// recent request's prompt size (0 until measured).
	contextWindow    int64
	lastPromptTokens int64
	compaction       CompactionOptions
	// usageTotals accumulates token usage across all requests this session.
	usageTotals llm.TokenUsage
}

// NewSession creates a session bound to a model client and tool registry.
func NewSession(client llm.Client, options Options) (*Session, error) {
	if client == nil {
		return nil, errors.New("session client is nil")
	}
	registry := options.Registry
	if registry == nil {
		registry = &toolRegistry{}
	}
	instructions := strings.TrimSpace(options.BaseInstructions)
	if instructions == "" {
		instructions = DefaultBaseInstructions
	}
	roundTrips := options.MaxRoundTrips
	if roundTrips <= 0 {
		roundTrips = maxToolRoundTrips
	}
	compaction := options.Compaction
	if compaction.Threshold <= 0 {
		compaction = defaultCompactionOptions()
	}
	return &Session{
		client: client,
		options: Options{
			BaseInstructions: instructions,
			Registry:         registry,
			MaxRoundTrips:    roundTrips,
			Approver:         options.Approver,
			ApprovalMode:     options.ApprovalMode,
			AutoApprove:      options.AutoApprove,
			OnPermanentGrant: options.OnPermanentGrant,
			Recorder:         options.Recorder,
			Compaction:       compaction,
		},
		approved:      make(map[approvalKind]bool),
		contextWindow: client.Model().ContextWindow,
		compaction:    compaction,
	}, nil
}

// SetApprover installs the approver after construction (the interactive UI
// only has one once its program is running).
func (session *Session) SetApprover(approver Approver) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.options.Approver = approver
}

// SetRecorder installs a recorder after construction.
func (session *Session) SetRecorder(recorder Recorder) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.options.Recorder = recorder
}

// EnableSubagents registers the spawn_agent / wait_agent / close_agent tools
// on the session. root is the working directory subagents read from; notify is
// called when a subagent finishes (id, status, result).
func (session *Session) EnableSubagents(root string, notify func(id, status, result string)) error {
	session.mu.Lock()
	registry := session.options.Registry
	client := session.client
	session.mu.Unlock()
	if registry == nil {
		return errors.New("session has no tool registry")
	}
	manager := newSubagentManager(client, root, func(id, status, result string) {
		if notify != nil {
			notify(id, status, result)
		}
		if status == string(subagentDone) {
			session.appendHistory(llm.UserMessage(fmt.Sprintf("[subagent %s finished]\n%s", id, result)))
		}
	})
	for _, tool := range []Tool{
		SpawnAgentTool{manager: manager},
		WaitAgentTool{manager: manager},
		CloseAgentTool{manager: manager},
	} {
		if err := registry.register(tool); err != nil {
			return err
		}
	}
	return nil
}

// SetBaseInstructions replaces the system prompt after construction (so the
// caller can build it from the session's tool set).
func (session *Session) SetBaseInstructions(instructions string) {
	if strings.TrimSpace(instructions) == "" {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	session.options.BaseInstructions = instructions
}

// Restore replaces the conversation history (used when resuming a session).
func (session *Session) Restore(items []llm.ResponseItem) {
	session.mu.Lock()
	defer session.mu.Unlock()
	cloned := make([]llm.ResponseItem, len(items))
	copy(cloned, items)
	session.history = cloned
}

// Clear drops the in-memory conversation history (the persisted session file,
// if any, is untouched).
func (session *Session) Clear() {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.history = nil
	session.lastPromptTokens = 0
}

// Compact forces a compaction of the current history, ignoring the threshold.
// It reports whether history was compacted.
func (session *Session) Compact(ctx context.Context, sink func(Event)) bool {
	session.mu.Lock()
	keep := session.compaction.KeepRecentTurns
	if keep <= 0 {
		keep = 4
	}
	history := append([]llm.ResponseItem(nil), session.history...)
	session.mu.Unlock()

	old, recent := splitHistory(history, keep)
	if len(old) == 0 {
		return false
	}
	summary, err := session.summarize(ctx, old)
	if err != nil {
		if sink != nil {
			sink(Event{Kind: EventError, Text: "compaction failed: " + err.Error()})
		}
		return false
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return false
	}
	compacted := make([]llm.ResponseItem, 0, len(recent)+1)
	compacted = append(compacted, llm.SystemMessage("Summary of the earlier conversation:\n"+summary))
	compacted = append(compacted, recent...)

	session.mu.Lock()
	session.history = compacted
	session.lastPromptTokens = 0
	session.mu.Unlock()
	if sink != nil {
		sink(Event{Kind: EventCompaction, Text: fmt.Sprintf("compacted %d earlier messages", len(old))})
	}
	return true
}

// recordUsage stores the most recent request's prompt size for compaction and
// accumulates session-wide token totals.
func (session *Session) recordUsage(usage llm.TokenUsage) {
	session.mu.Lock()
	session.lastPromptTokens = usage.InputTokens
	session.usageTotals.InputTokens += usage.InputTokens
	session.usageTotals.OutputTokens += usage.OutputTokens
	session.usageTotals.TotalTokens += usage.TotalTokens
	session.mu.Unlock()
}

// UsageTotals reports the cumulative token usage for this session.
func (session *Session) UsageTotals() llm.TokenUsage {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.usageTotals
}

// ContextUsage reports the last measured prompt size and the model's context
// window (used to display a context percentage).
func (session *Session) ContextUsage() (used, window int64) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.lastPromptTokens, session.contextWindow
}

// ContextWindow reports the model's context window size.
func (session *Session) ContextWindow() int64 {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.contextWindow
}

// ToolNames reports the names of the tools offered to the model.
func (session *Session) ToolNames() []string {
	if session.options.Registry == nil {
		return nil
	}
	return session.options.Registry.names()
}

// History returns a defensive copy of the conversation so far.
func (session *Session) History() []llm.ResponseItem {
	session.mu.Lock()
	defer session.mu.Unlock()
	cloned := make([]llm.ResponseItem, len(session.history))
	copy(cloned, session.history)
	return cloned
}

func (session *Session) appendHistory(items ...llm.ResponseItem) {
	session.mu.Lock()
	session.history = append(session.history, items...)
	recorder := session.options.Recorder
	session.mu.Unlock()
	if recorder != nil {
		for _, item := range items {
			recorder.Record(item)
		}
	}
}

// Ask streams one user turn to completion, emitting progress events to the
// sink and returning the final assistant text.
func (session *Session) Ask(ctx context.Context, prompt string, sink func(Event)) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("prompt is empty")
	}
	if sink == nil {
		sink = func(Event) {}
	}
	// Compact before appending the new turn if the context is nearly full.
	session.maybeCompact(ctx, sink)
	session.appendHistory(llm.UserMessage(prompt))

	var builder strings.Builder
	for round := 0; round < session.options.MaxRoundTrips; round++ {
		message, err := session.sample(ctx, sink)
		if err != nil {
			return builder.String(), err
		}
		session.appendHistory(message)
		builder.Reset()
		builder.WriteString(message.Content)

		if len(message.ToolCalls) == 0 {
			return builder.String(), nil
		}
		if err := session.executeToolCalls(ctx, message.ToolCalls, sink); err != nil {
			return builder.String(), err
		}
	}
	return builder.String(), fmt.Errorf("exceeded %d tool round trips", session.options.MaxRoundTrips)
}

// sample performs one model call, streaming deltas to the sink.
func (session *Session) sample(ctx context.Context, sink func(Event)) (llm.ResponseItem, error) {
	request := llm.Request{
		Model: session.client.Model().Name,
		Prompt: llm.Prompt{
			Input:            session.History(),
			BaseInstructions: session.options.BaseInstructions,
		},
	}
	for _, view := range session.options.Registry.specs() {
		request.Prompt.Tools = append(request.Prompt.Tools, llm.ToolSpec{
			Name:        view.Name,
			Description: view.Description,
			InputSchema: view.InputSchema,
		})
	}

	stream, err := session.client.Stream(ctx, request)
	if err != nil {
		// Fall back to a blocking request only when the failure looks
		// recoverable in another form (e.g. a dropped stream). A definitive
		// rejection from the provider — an HTTP 4xx/5xx status — would fail
		// identically on the blocking path, so retrying it just wastes a
		// request and muddies the error; surface it directly instead.
		if !llm.IsRetryable(err) {
			return llm.ResponseItem{}, err
		}
		response, completeErr := session.client.Complete(ctx, request)
		if completeErr != nil {
			return llm.ResponseItem{}, errors.Join(err, completeErr)
		}
		if response.TokenUsage.TotalTokens > 0 {
			session.recordUsage(response.TokenUsage)
			sink(Event{Kind: EventUsage, Usage: &response.TokenUsage})
		}
		return response.Message, nil
	}
	defer stream.Close()

	var message llm.ResponseItem
	message.Role = llm.RoleAssistant
	var toolArgs strings.Builder
	var activeCall llm.ToolCall
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return message, err
		}
		if chunk.ReasoningDelta != "" {
			sink(Event{Kind: EventReasoningDelta, Text: chunk.ReasoningDelta})
		}
		if chunk.ContentDelta != "" {
			message.Content += chunk.ContentDelta
			sink(Event{Kind: EventContentDelta, Text: chunk.ContentDelta})
		}
		if chunk.TokenUsage != nil {
			usage := *chunk.TokenUsage
			session.recordUsage(usage)
			sink(Event{Kind: EventUsage, Usage: &usage})
		}
		for _, call := range chunk.ToolCalls {
			if call.ID != "" || call.Name != "" {
				if activeCall.ID != "" || activeCall.Name != "" {
					message.ToolCalls = append(message.ToolCalls, finalizeToolCall(activeCall, toolArgs.String()))
					toolArgs.Reset()
				}
				if call.ID != "" {
					activeCall.ID = call.ID
				}
				if call.Name != "" {
					activeCall.Name = call.Name
				}
			}
			toolArgs.WriteString(string(call.Arguments))
		}
		if chunk.Completed() {
			if activeCall.ID != "" || activeCall.Name != "" {
				message.ToolCalls = append(message.ToolCalls, finalizeToolCall(activeCall, toolArgs.String()))
				toolArgs.Reset()
				activeCall = llm.ToolCall{}
			}
		}
	}
	if activeCall.ID != "" || activeCall.Name != "" {
		message.ToolCalls = append(message.ToolCalls, finalizeToolCall(activeCall, toolArgs.String()))
	}
	return message, nil
}

func finalizeToolCall(call llm.ToolCall, arguments string) llm.ToolCall {
	if strings.TrimSpace(arguments) == "" {
		call.Arguments = []byte("{}")
	} else {
		call.Arguments = []byte(arguments)
	}
	return call
}

// executeToolCalls runs each requested tool and appends the tool results to
// the conversation history.
func (session *Session) executeToolCalls(ctx context.Context, calls []llm.ToolCall, sink func(Event)) error {
	for _, call := range calls {
		sink(Event{Kind: EventToolCall, Tool: call.Name, Text: string(call.Arguments)})
		result := session.runTool(ctx, call, sink)
		sink(Event{Kind: EventToolResult, Tool: call.Name, Text: truncateForEvent(result)})
		session.appendHistory(llm.ToolResultMessage(call.ID, result))
	}
	return nil
}

// runTool executes one tool, applying the approval policy first. Denials are
// reported as a tool result so the model can adapt.
func (session *Session) runTool(ctx context.Context, call llm.ToolCall, sink func(Event)) string {
	tool, ok := session.options.Registry.lookup(call.Name)
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", call.Name)
	}
	if decision, denied := session.checkApproval(ctx, tool, call, sink); denied {
		return decision
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return "error: " + err.Error()
	}
	return result
}

// checkApproval resolves the approval policy for a tool. It returns a
// user-facing message and true when the operation must not run.
func (session *Session) checkApproval(ctx context.Context, tool Tool, call llm.ToolCall, sink func(Event)) (string, bool) {
	requestor, ok := tool.(approvalRequestor)
	if !ok {
		return "", false
	}
	category := requestor.ApprovalKind()
	summary, needs := requestor.ApprovalSummary(call.Arguments)
	if category == approvalKindNone {
		return "", false
	}
	mode := ApprovalModeAsk
	if session.options.ApprovalMode != nil {
		mode = session.options.ApprovalMode(string(category))
	}
	switch mode {
	case ApprovalModeAllow:
		return "", false
	case ApprovalModeDeny:
		return "error: " + string(category) + " is denied by policy", true
	}
	// Ask: an allowed prefix (or a session grant) short-circuits the prompt.
	if session.options.AutoApprove != nil && session.options.AutoApprove(string(category), summary) {
		return "", false
	}
	if session.sessionApproved(category) {
		return "", false
	}
	requiresApproval := needs
	if !requiresApproval {
		// Ask mode still prompts for a category we cannot summarize safely.
		requiresApproval = true
	}
	if !requiresApproval {
		return "", false
	}
	sink(Event{Kind: EventApproval, Tool: call.Name, Text: summary})
	if session.options.Approver == nil {
		return "error: approval required but no approver is configured", true
	}
	decision, err := session.options.Approver(ctx, ApprovalRequest{Tool: call.Name, Summary: summary})
	if err != nil {
		return "error: approval failed: " + err.Error(), true
	}
	switch decision {
	case ApprovalOnce:
		return "", false
	case ApprovalSession:
		session.markSessionApproved(category)
		return "", false
	case ApprovalPermanent:
		if session.options.OnPermanentGrant != nil {
			if err := session.options.OnPermanentGrant(string(category), summary); err != nil {
				return "error: could not record permanent approval: " + err.Error(), true
			}
		}
		return "", false
	default:
		return "error: the user denied this operation", true
	}
}

func (session *Session) sessionApproved(category approvalKind) bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.approved[category]
}

func (session *Session) markSessionApproved(category approvalKind) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.approved[category] = true
}

func truncateForEvent(text string) string {
	const limit = 400
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

// ReadLine reads one line of input, handling the quit sentinel.
func ReadLine(reader *bufio.Reader) (string, bool, error) {
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimRight(line, "\r\n"), true, nil
}
