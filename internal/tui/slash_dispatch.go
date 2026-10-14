package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/agent"
)

// dispatchSlashCommand runs a local slash command and returns any command to
// the Bubble Tea runtime (e.g. quit).
func (m *Model) dispatchSlashCommand(invocation slashInvocation) tea.Cmd {
	switch invocation.Command {
	case slashHelp:
		m.appendTranscript("")
		m.appendTranscript(m.palette.accent().Render("Commands"))
		for _, line := range formatSlashCommands() {
			m.appendTranscript("  " + line)
		}
		return nil
	case slashClear:
		m.clearConversation()
		return nil
	case slashCompact:
		return m.compactNow()
	case slashModel:
		m.appendTranscript("")
		m.appendTranscript("  " + m.palette.dim().Render("model: ") + m.model)
		return nil
	case slashStatus:
		m.appendTranscript("")
		for _, line := range m.statusLines() {
			m.appendTranscript("  " + line)
		}
		return nil
	case slashResume:
		m.appendTranscript("")
		m.appendTranscript(m.palette.dim().Render("  resume is not interactive yet — run `equus chat --resume` or `--continue`"))
		return nil
	case slashQuit:
		return tea.Quit
	default:
		m.appendTranscript("")
		m.appendTranscript(m.errorEntry(fmt.Sprintf("unknown command /%s (try /help)", invocation.Command))...)
		return nil
	}
}

// clearConversation resets the transcript and the session history, then re-adds
// the startup logo.
func (m *Model) clearConversation() {
	m.transcript = nil
	m.active = ""
	m.scroll = 0
	m.session.Clear()
	m.appendTranscript(m.logoLines()...)
	m.appendTranscript("")
}

// compactNow forces a compaction and reports the result in the transcript.
func (m Model) compactNow() tea.Cmd {
	session := m.session
	ctx := m.ctx
	program := m.program
	before := len(session.History())
	return func() tea.Msg {
		compacted := session.Compact(ctx, func(event agent.Event) {
			program.send(eventMsg{event: event})
		})
		if !compacted {
			program.send(eventMsg{event: agent.Event{Kind: agent.EventError, Text: fmt.Sprintf("nothing to compact (%d messages)", before)}})
		}
		return nil
	}
}

// statusLines renders the /status output.
func (m Model) statusLines() []string {
	lines := []string{
		m.palette.dim().Render("model:     ") + m.model,
		m.palette.dim().Render("directory: ") + m.cwd,
		m.palette.dim().Render("version:   ") + m.version,
	}
	if percent, ok := m.contextPercent(); ok {
		lines = append(lines, m.palette.dim().Render("context:   ")+fmt.Sprintf("%d%% used", percent))
	}
	if names := m.session.ToolNames(); len(names) > 0 {
		lines = append(lines, m.palette.dim().Render("tools:     ")+strings.Join(names, ", "))
	}
	return lines
}
