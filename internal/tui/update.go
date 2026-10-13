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
		m.refreshViewport()
		return m, nil
	case turnDoneMsg:
		m.finishTurn(message)
		m.refreshViewport()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(message)
	}
	return m, nil
}

// resize reflows the input box, viewport and transcript to the new size.
func (m *Model) resize(width, height int) {
	if width < minWidth {
		width = minWidth
	}
	m.width = width
	m.height = height
	m.input.SetWidth(width - len(inputPrompt))
	m.viewport.Width = width
	m.refreshViewport()
	m.ready = true
}

// handleKey routes a key press to the appropriate handler. Editing keys fall
// through to the textarea, whose raw-mode handling covers backspace, arrows
// and cursor movement — the reason the TUI replaces the old line reader.
func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.input.Reset()
		m.historyPos = -1
		return m, nil
	case "pgup":
		m.viewport.ViewUp()
		return m, nil
	case "pgdown":
		m.viewport.ViewDown()
		return m, nil
	case "up":
		if m.running {
			break
		}
		if m.recallHistory(-1) {
			return m, nil
		}
	case "down":
		if m.running {
			break
		}
		if m.recallHistory(1) {
			return m, nil
		}
	case "enter":
		if m.running {
			return m, nil
		}
		prompt := strings.TrimSpace(m.input.Value())
		if prompt == "" {
			return m, nil
		}
		m.input.Reset()
		m.history = append(m.history, prompt)
		m.historyPos = -1
		m.commit(prompt)
		m.running = true
		m.status = "thinking"
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

// handleEvent folds a streaming agent event into the active buffer.
func (m *Model) handleEvent(event agent.Event) {
	switch event.Kind {
	case agent.EventReasoningDelta:
		m.status = "thinking"
	case agent.EventContentDelta:
		m.status = "responding"
		m.active.WriteString(event.Text)
	case agent.EventToolCall:
		m.flushActive()
		m.transcript = append(m.transcript, m.palette.tool().Render("[tool] "+event.Tool+"("+event.Text+")"))
	case agent.EventToolResult:
		m.transcript = append(m.transcript, m.palette.dim().Render("[tool-result] "+event.Tool+": "+firstLine(event.Text)))
	case agent.EventUsage:
		if event.Usage != nil {
			m.transcript = append(m.transcript, m.palette.dim().Render(usageLine(event.Usage)))
		}
	case agent.EventError:
		m.transcript = append(m.transcript, m.palette.failure().Render("error: "+event.Text))
	}
}

// finishTurn finalizes the turn, committing the answer into the transcript.
func (m *Model) finishTurn(message turnDoneMsg) {
	m.flushActive()
	if message.err != nil && !errors.Is(message.err, context.Canceled) {
		if text := message.err.Error(); text != "" {
			m.transcript = append(m.transcript, m.palette.failure().Render("error: "+text))
		}
	}
	m.running = false
	m.status = "idle"
}

// commit adds the user's line to the transcript before sampling begins.
func (m *Model) commit(prompt string) {
	m.flushActive()
	for _, line := range strings.Split(prompt, "\n") {
		m.transcript = append(m.transcript, m.palette.user().Render("› "+line))
	}
}

// flushActive moves the streaming buffer into the committed transcript.
func (m *Model) flushActive() {
	text := strings.TrimRight(m.active.String(), "\n")
	m.active.Reset()
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		m.transcript = append(m.transcript, "  "+line)
	}
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
