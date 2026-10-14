package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/llm"
)

// Update implements tea.Model.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case eventMsg:
		m.handleEvent(message.event)
		return m, nil
	case approvalMsg:
		m.pending = &approvalPrompt{summary: message.summary, reply: message.reply}
		return m, nil
	case turnDoneMsg:
		m.finishTurn(message)
		return m, nil
	case sessionLoadedMsg:
		m.applyLoadedSession(message)
		return m, nil
	case tea.MouseMsg:
		m.handleMouse(message)
		return m, nil
	case tea.KeyMsg:
		updated, command := m.handleKey(message)
		if model, ok := updated.(Model); ok {
			model.updateInputLayout()
			return model, command
		}
		return updated, command
	}
	return m, nil
}

// handleMouse scrolls the transcript on wheel events.
func (m *Model) handleMouse(message tea.MouseMsg) {
	switch message.Button {
	case tea.MouseButtonWheelUp:
		m.scrollBy(3)
	case tea.MouseButtonWheelDown:
		m.scrollBy(-3)
	}
}

// replyApproval resolves a pending approval and clears it.
func (m *Model) replyApproval(decision agent.ApprovalDecision) {
	if m.pending == nil {
		return
	}
	select {
	case m.pending.reply <- decision:
	default:
	}
	m.pending = nil
}

// scrollBy adjusts the transcript scroll offset, clamped to the content.
func (m *Model) scrollBy(delta int) {
	m.scroll += delta
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// resize reflows the input box to the new size.
func (m *Model) resize(width, height int) {
	if width < minWidth {
		width = minWidth
	}
	m.width = width
	m.height = height
	m.input.SetWidth(width - inputPromptWidth())
	m.updateInputLayout()
}

// updateInputLayout keeps the textarea's own height stable (the live textarea
// only needs to be tall enough to hold the cursor; the composer frames the
// visible window itself, so per-keystroke resizing — which would leave the
// textarea's viewport YOffset stale and hide earlier lines — is avoided).
func (m *Model) updateInputLayout() {
	m.input.SetHeight(maxInputRows)
}

// handleKey routes a key press to the appropriate handler. Editing keys fall
// through to the textarea, whose raw-mode handling covers backspace, arrows
// and cursor movement — the reason the TUI replaces the old line reader.
func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the session picker is open, it captures all keys.
	if m.picker != nil {
		model, command, _ := m.handlePickerKey(key)
		return model, command
	}
	// While an approval is pending, y/a/p/n decide it and nothing else is
	// routed.
	if m.pending != nil {
		switch key.String() {
		case "y", "Y":
			m.replyApproval(agent.ApprovalOnce)
		case "a", "A":
			m.replyApproval(agent.ApprovalSession)
		case "p", "P":
			m.replyApproval(agent.ApprovalPermanent)
		case "n", "N", "esc", "ctrl+c":
			m.replyApproval(agent.ApprovalDenied)
		}
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		if m.running {
			m.cancel()
			m.status = "cancelling"
			return m, nil
		}
		m.input.Reset()
		m.historyPos = -1
		return m, nil
	case "ctrl+d":
		if strings.TrimSpace(m.input.Value()) == "" {
			return m, tea.Quit
		}
	case "esc":
		if m.running {
			m.cancel()
			m.status = "cancelling"
			return m, nil
		}
		m.input.Reset()
		m.historyPos = -1
		return m, nil
	case "pgup":
		m.scrollBy(m.height / 2)
		return m, nil
	case "pgdown":
		m.scrollBy(-(m.height / 2))
		return m, nil
	case "up":
		if m.running {
			break
		}
		// Move within the input's lines first; only recall history when the
		// cursor is already on the first line.
		if m.input.Line() > 0 {
			break
		}
		if m.recallHistory(-1) {
			return m, nil
		}
	case "down":
		if m.running {
			break
		}
		if m.input.Line() < strings.Count(m.input.Value(), "\n") {
			break
		}
		if m.recallHistory(1) {
			return m, nil
		}
	case "ctrl+j", "alt+enter":
		// Insert a newline instead of submitting. Bubble Tea v1 cannot
		// distinguish shift+enter from enter (no Kitty keyboard protocol), so
		// these are the reliable multi-line keys: ctrl+j works everywhere,
		// alt+enter where the terminal reports it.
		if m.running {
			return m, nil
		}
		var command tea.Cmd
		m.input, command = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m, command
	case "enter":
		if m.running {
			return m, nil
		}
		prompt := strings.TrimSpace(m.input.Value())
		if prompt == "" {
			return m, nil
		}
		m.input.Reset()
		m.historyPos = -1
		// A leading slash dispatches a local command instead of a model turn.
		parsed, err := parseInput(prompt)
		if err != nil {
			m.appendTranscript("")
			m.appendTranscript(m.errorEntry(err.Error())...)
			return m, nil
		}
		if parsed.Command != nil {
			m.history = append(m.history, prompt)
			return m, m.dispatchSlashCommand(*parsed.Command)
		}
		m.history = append(m.history, prompt)
		m.active = ""
		m.running = true
		m.scroll = 0
		m.status = "thinking"
		// Echo the user's line into the transcript (preceded by a blank line to
		// separate turns), then start the turn.
		m.appendTranscript("")
		m.appendTranscript(m.userEntry(prompt)...)
		return m, m.runTurn(prompt)
	}

	var command tea.Cmd
	m.input, command = m.input.Update(key)
	return m, command
}

// runTurn drives one agent turn in a goroutine, forwarding events back into
// the Bubble Tea loop via tea.Program.Send.
func (m Model) runTurn(prompt string) tea.Cmd {
	ctx := m.ctx
	session := m.session
	program := m.program
	return func() tea.Msg {
		answer, err := session.Ask(ctx, prompt, func(event agent.Event) {
			program.send(eventMsg{event: event})
		})
		return turnDoneMsg{answer: answer, err: err}
	}
}

// handleEvent folds a streaming agent event into the model, committing any
// lines that belong in the transcript.
func (m *Model) handleEvent(event agent.Event) {
	switch event.Kind {
	case agent.EventReasoningDelta:
		m.status = "thinking"
	case agent.EventContentDelta:
		m.status = "responding"
		m.active += event.Text
	case agent.EventToolCall:
		m.flushActive()
		m.status = "tool: " + event.Tool
		m.appendTranscript(m.toolCallEntry(event.Tool, event.Text)...)
	case agent.EventToolResult:
		m.appendTranscript(m.toolResultEntry(event.Tool, event.Text)...)
	case agent.EventUsage:
		if event.Usage != nil {
			m.ctxUsed = event.Usage.InputTokens
			m.appendTranscript(m.usageEntry(usageLine(event.Usage))...)
		}
	case agent.EventError:
		m.appendTranscript(m.errorEntry(event.Text)...)
	case agent.EventApproval:
		// The prompt itself is drawn from m.pending (set via approvalMsg).
		m.status = "awaiting approval"
	case agent.EventCompaction:
		m.appendTranscript(m.palette.dim().Render("· " + event.Text))
	}
}

// finishTurn finalizes the turn, committing the answer into the transcript.
func (m *Model) finishTurn(message turnDoneMsg) {
	m.flushActive()
	if message.err != nil && !errors.Is(message.err, context.Canceled) {
		if text := message.err.Error(); text != "" {
			m.appendTranscript(m.errorEntry(text)...)
		}
	}
	m.running = false
	m.status = "idle"
}

// flushActive commits the accumulated assistant stream into the transcript and
// resets the buffer.
func (m *Model) flushActive() {
	text := strings.TrimRight(m.active, "\n")
	m.active = ""
	if strings.TrimSpace(text) == "" {
		return
	}
	m.appendTranscript(m.assistantEntry(text)...)
}

// recallHistory walks the submitted-prompt history. It reports whether the
// input box was updated.
func (m *Model) recallHistory(direction int) bool {
	if len(m.history) == 0 {
		return false
	}
	if m.historyPos < 0 {
		if direction > 0 {
			return false
		}
		m.historyPos = len(m.history)
	}
	m.historyPos += direction
	if m.historyPos < 0 {
		m.historyPos = 0
	}
	if m.historyPos >= len(m.history) {
		m.historyPos = len(m.history)
		m.input.Reset()
		return true
	}
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
	return true
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

func usageLine(usage *llm.TokenUsage) string {
	return "in=" + strconv.FormatInt(usage.InputTokens, 10) + " out=" + strconv.FormatInt(usage.OutputTokens, 10)
}
