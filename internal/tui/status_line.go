package tui

import "strings"

// statusLine renders the footer under the composer: the model, working
// directory, and current run state, each colored by its semantic slot.
func (m Model) statusLine() string {
	segments := make([]string, 0, 3)
	if m.model != "" {
		segments = append(segments, m.palette.statusLineStyle(statusAccentModel).Render(m.model))
	}
	if m.cwd != "" {
		segments = append(segments, m.palette.statusLineStyle(statusAccentPath).Render(m.cwd))
	}
	segments = append(segments, m.palette.statusLineStyle(statusAccentMode).Render(stateLabel(m)))
	separator := m.palette.dim().Render(" · ")
	return "  " + strings.Join(segments, separator)
}

// stateLabel reports the current interaction state shown on the status line.
func stateLabel(m Model) string {
	if m.running {
		return "working"
	}
	return "ready"
}
