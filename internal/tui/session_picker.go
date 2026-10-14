package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/threadstore"
)

// sessionPicker is an overlay that lets the user choose a saved session.
type sessionPicker struct {
	items    []threadstore.Meta
	selected int
}

// renderSessionPicker draws the picker as an overlay occupying the transcript
// region, leaving the composer/status in place.
func (m Model) renderSessionPicker(rows int) []string {
	p := m.picker
	lines := []string{
		"",
		m.palette.accent().Render("Select a session") + m.palette.dim().Render("   (↑/↓ move · enter select · esc cancel)"),
		"",
	}
	if len(p.items) == 0 {
		lines = append(lines, m.palette.dim().Render("  (no saved sessions)"))
	}
	for index, meta := range p.items {
		marker := "  "
		style := m.palette.plain()
		if index == p.selected {
			marker = "› "
			style = m.palette.accent()
		}
		title := strings.TrimSpace(meta.Title)
		if title == "" {
			title = "(untitled)"
		}
		line := fmt.Sprintf("%s%s  %s  %s", marker, meta.CreatedAt.Format("2006-01-02 15:04"), title, m.palette.dim().Render(meta.CWD))
		lines = append(lines, style.Render(line))
	}
	// Fit to the region height, top-aligned.
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

// handlePickerKey handles a key while the picker is open. It returns the model,
// a command, and whether the key was consumed.
func (m Model) handlePickerKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.picker == nil {
		return m, nil, false
	}
	switch key.String() {
	case "up", "k":
		if m.picker.selected > 0 {
			m.picker.selected--
		}
		return m, nil, true
	case "down", "j":
		if m.picker.selected < len(m.picker.items)-1 {
			m.picker.selected++
		}
		return m, nil, true
	case "esc", "ctrl+c":
		m.picker = nil
		return m, nil, true
	case "enter":
		if len(m.picker.items) == 0 {
			m.picker = nil
			return m, nil, true
		}
		meta := m.picker.items[m.picker.selected]
		m.picker = nil
		return m, m.loadSession(meta.ID), true
	}
	return m, nil, true
}

// applyLoadedSession installs the selected session's history and rebuilds the
// transcript.
func (m *Model) applyLoadedSession(message sessionLoadedMsg) {
	if message.err != nil {
		m.appendTranscript(m.errorEntry("resume failed: " + message.err.Error())...)
		return
	}
	m.session.Restore(message.items)
	m.sessionID = message.id
	m.transcript = nil
	m.active = ""
	m.scroll = 0
	m.appendTranscript(m.logoLines()...)
	m.appendTranscript("")
	if lines := renderHistory(*m, message.items); len(lines) > 0 {
		m.appendTranscript(lines...)
	}
}

// resolveDeleteConfirm handles a y/n key for a pending /delete.
func (m Model) resolveDeleteConfirm(key tea.KeyMsg) Model {
	m.deleteConfirm = false
	switch key.String() {
	case "y", "Y":
		if m.deleteFn != nil && m.sessionID != "" {
			if err := m.deleteFn(m.sessionID); err != nil {
				m.appendTranscript(m.errorEntry("delete failed: " + err.Error())...)
				return m
			}
			m.appendTranscript(m.palette.dim().Render("  deleted session " + m.sessionID))
			m.sessionID = ""
		}
	default:
		m.appendTranscript(m.palette.dim().Render("  delete cancelled"))
	}
	return m
}

// loadSession requests the selected session's items via the configured loader.
func (m Model) loadSession(id string) tea.Cmd {
	if m.loadSessionFn == nil {
		return nil
	}
	loader := m.loadSessionFn
	return func() tea.Msg {
		items, err := loader(id)
		return sessionLoadedMsg{id: id, items: items, err: err}
	}
}
