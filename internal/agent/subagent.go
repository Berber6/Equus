package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Godric-W/Equus/internal/llm"
)

// maxSubagents bounds concurrently running subagents per session.
const maxSubagents = 8

// subagentSystemPrompt is the base instruction for a spawned subagent.
const subagentSystemPrompt = "You are a subagent spawned by Equus to complete one focused task. " +
	"You have read-only tools (read_file, list_files, glob, grep). " +
	"Investigate as needed and then reply with a concise result for the parent agent. " +
	"Do not ask questions; make reasonable assumptions and report what you found."

// subagentStatus enumerates a subagent's lifecycle state.
type subagentStatus string

const (
	subagentRunning subagentStatus = "running"
	subagentDone    subagentStatus = "done"
	subagentFailed  subagentStatus = "failed"
)

// subagent is one spawned task.
type subagent struct {
	id     string
	prompt string
	status subagentStatus
	result string
	err    error
	done   chan struct{}
	cancel context.CancelFunc
}

// subagentManager runs and tracks subagents for a session.
type subagentManager struct {
	client   llm.Client
	root     string
	notify   func(id, status, result string)
	mu       sync.Mutex
	agents   map[string]*subagent
	sequence int
}

func newSubagentManager(client llm.Client, root string, notify func(id, status, result string)) *subagentManager {
	return &subagentManager{client: client, root: root, notify: notify, agents: map[string]*subagent{}}
}

// spawn starts a subagent in the background and returns its id.
func (m *subagentManager) spawn(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("spawn_agent: prompt is empty")
	}
	m.mu.Lock()
	if len(m.agents) >= maxSubagents {
		m.mu.Unlock()
		return "", fmt.Errorf("spawn_agent: at most %d subagents may run at once", maxSubagents)
	}
	m.sequence++
	id := fmt.Sprintf("agent-%d", m.sequence)
	agent := &subagent{id: id, prompt: prompt, status: subagentRunning, done: make(chan struct{})}
	m.agents[id] = agent
	m.mu.Unlock()

	go m.run(id, agent, ctx)
	return id, nil
}

// run executes one subagent to completion and records its result.
func (m *subagentManager) run(id string, agent *subagent, parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	agent.cancel = cancel
	m.mu.Unlock()
	defer cancel()

	result, err := runSubagent(ctx, m.client, m.root, agent.prompt)
	m.mu.Lock()
	if err != nil {
		agent.status = subagentFailed
		agent.err = err
	} else {
		agent.status = subagentDone
		agent.result = truncateSubagentResult(result)
	}
	status, text := agent.status, agent.result
	m.mu.Unlock()
	close(agent.done)

	if m.notify != nil {
		m.notify(id, string(status), text)
	}
}

// wait blocks until the subagent finishes or the timeout elapses.
func (m *subagentManager) wait(ctx context.Context, id string, timeout time.Duration) (subagentStatus, string, error) {
	m.mu.Lock()
	agent, ok := m.agents[id]
	m.mu.Unlock()
	if !ok {
		return "", "", fmt.Errorf("wait_agent: unknown subagent %q", id)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-agent.done:
	case <-timer.C:
		return subagentRunning, "", nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if agent.status == subagentFailed && agent.err != nil {
		return agent.status, "", agent.err
	}
	return agent.status, agent.result, nil
}

// close cancels a running subagent and removes it.
func (m *subagentManager) close(id string) error {
	m.mu.Lock()
	agent, ok := m.agents[id]
	if ok {
		delete(m.agents, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("close_agent: unknown subagent %q", id)
	}
	if agent.cancel != nil {
		agent.cancel()
	}
	return nil
}

// list reports the ids of all tracked subagents (for status/diagnostics).
func (m *subagentManager) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// runSubagent builds a read-only subagent session and runs one prompt.
func runSubagent(ctx context.Context, client llm.Client, root, prompt string) (string, error) {
	registry, err := NewReadOnlyRegistry(root)
	if err != nil {
		return "", err
	}
	session, err := NewSession(client, Options{Registry: registry, BaseInstructions: subagentSystemPrompt})
	if err != nil {
		return "", err
	}
	return session.Ask(ctx, prompt, nil)
}

// truncateSubagentResult caps how much of a subagent result is returned.
func truncateSubagentResult(text string) string {
	const limit = 8 << 10
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n... (truncated)"
}

// --- tools ---

// SpawnAgentTool starts a background subagent.
type SpawnAgentTool struct{ manager *subagentManager }

func (tool SpawnAgentTool) Name() string { return "spawn_agent" }

func (tool SpawnAgentTool) Description() string {
	return "Spawn a subagent to work on a focused task in the background (read-only tools, up to 8 at once). " +
		"It returns an agent id; you are notified automatically when it finishes."
}

func (tool SpawnAgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The task for the subagent to complete."}
		},
		"required": ["prompt"]
	}`)
}

func (tool SpawnAgentTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("spawn_agent: decode arguments: %w", err)
	}
	id, err := tool.manager.spawn(ctx, input.Prompt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("spawned subagent %s (running in the background; you'll be notified when it finishes)", id), nil
}

// WaitAgentTool blocks until a subagent finishes.
type WaitAgentTool struct{ manager *subagentManager }

func (tool WaitAgentTool) Name() string { return "wait_agent" }

func (tool WaitAgentTool) Description() string {
	return "Wait for a previously spawned subagent to finish and return its result. " +
		"If it is still running after the timeout, returns its current status."
}

func (tool WaitAgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "The subagent id returned by spawn_agent."},
			"timeout_seconds": {"type": "integer", "description": "Optional wait timeout in seconds (default 30, max 300)."}
		},
		"required": ["id"]
	}`)
}

func (tool WaitAgentTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		ID             string `json:"id"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("wait_agent: decode arguments: %w", err)
	}
	timeout := 30 * time.Second
	if input.TimeoutSeconds > 0 {
		timeout = time.Duration(input.TimeoutSeconds) * time.Second
		if timeout > 300*time.Second {
			timeout = 300 * time.Second
		}
	}
	status, result, err := tool.manager.wait(ctx, strings.TrimSpace(input.ID), timeout)
	if err != nil {
		return "", fmt.Errorf("wait_agent: %w", err)
	}
	switch status {
	case subagentRunning:
		return fmt.Sprintf("subagent %s is still running", input.ID), nil
	case subagentFailed:
		return fmt.Sprintf("subagent %s failed", input.ID), nil
	default:
		return fmt.Sprintf("subagent %s finished:\n%s", input.ID, result), nil
	}
}

// CloseAgentTool cancels a subagent.
type CloseAgentTool struct{ manager *subagentManager }

func (tool CloseAgentTool) Name() string { return "close_agent" }

func (tool CloseAgentTool) Description() string {
	return "Stop tracking a subagent and release its slot (cancels it if still running)."
}

func (tool CloseAgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "The subagent id to close."}
		},
		"required": ["id"]
	}`)
}

func (tool CloseAgentTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("close_agent: decode arguments: %w", err)
	}
	if err := tool.manager.close(strings.TrimSpace(input.ID)); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed subagent %s", input.ID), nil
}
