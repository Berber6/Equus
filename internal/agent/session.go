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
)

// Session keeps conversation history and drives one agent turn at a time.
type Session struct {
	client  llm.Client
	options Options

	mu      sync.Mutex
	history []llm.ResponseItem
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
	return &Session{
		client: client,
		options: Options{
			BaseInstructions: instructions,
			Registry:         registry,
			MaxRoundTrips:    roundTrips,
		},
	}, nil
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
	defer session.mu.Unlock()
	session.history = append(session.history, items...)
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
		// Fall back to a blocking request when streaming is unavailable.
		response, completeErr := session.client.Complete(ctx, request)
		if completeErr != nil {
			return llm.ResponseItem{}, errors.Join(err, completeErr)
		}
		if response.TokenUsage.TotalTokens > 0 {
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
		tool, ok := session.options.Registry.lookup(call.Name)
		var result string
		var err error
		switch {
		case !ok:
			result = fmt.Sprintf("error: unknown tool %q", call.Name)
		default:
			result, err = tool.Execute(ctx, call.Arguments)
			if err != nil {
				result = "error: " + err.Error()
			}
		}
		sink(Event{Kind: EventToolResult, Tool: call.Name, Text: truncateForEvent(result)})
		session.appendHistory(llm.ToolResultMessage(call.ID, result))
	}
	return nil
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
