package tui

import (
	"fmt"
	"strings"
)

// statusLine renders the footer under the composer: the model, working
// directory, context usage, and current run state, each colored by its
// semantic slot.
func (m Model) statusLine() string {
	segments := make([]string, 0, 4)
	if m.model != "" {
		segments = append(segments, m.palette.statusLineStyle(statusAccentModel).Render(m.model))
	}
	if m.cwd != "" {
		segments = append(segments, m.palette.statusLineStyle(statusAccentPath).Render(m.cwd))
	}
	if percent, ok := m.contextPercent(); ok {
		style := m.palette.statusLineStyle(statusAccentUsage)
		if percent >= 90 {
			style = m.palette.failure()
		} else if percent >= 70 {
			style = m.palette.warning()
		}
		segments = append(segments, style.Render(fmt.Sprintf("ctx %d%%", percent)))
	}
	segments = append(segments, m.palette.statusLineStyle(statusAccentMode).Render(stateLabel(m)))
	separator := m.palette.dim().Render(" · ")
	return "  " + strings.Join(segments, separator)
}

// contextPercent reports how full the context is, when known.
func (m Model) contextPercent() (int, bool) {
	if m.ctxWindow <= 0 || m.ctxUsed <= 0 {
		return 0, false
	}
	percent := int(m.ctxUsed * 100 / m.ctxWindow)
	if percent > 100 {
		percent = 100
	}
	return percent, true
}

// stateLabel reports the current interaction state shown on the status line.
func stateLabel(m Model) string {
	if m.running {
		return "working"
	}
	return "ready"
}
