package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// View renders the whole interface: the scrolling transcript, an optional
// working line, and the input composer at the bottom.
func (m Model) View() string {
	body := m.viewport.View()
	parts := make([]string, 0, 3)
	if body != "" {
		parts = append(parts, body)
	}
	if working := m.workingLine(); working != "" {
		parts = append(parts, working)
	}
	parts = append(parts, m.composerView())
	return strings.Join(parts, "\n")
}

// refreshViewport re-renders the transcript into the scroll viewport and pins
// it to the bottom so new output stays visible.
func (m *Model) refreshViewport() {
	content := strings.Join(m.transcript, "\n")
	if active := m.active.String(); strings.TrimSpace(active) != "" {
		for _, line := range strings.Split(strings.TrimRight(active, "\n"), "\n") {
			content += "\n  " + line
		}
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// composerView renders the input box, highlighted while a turn is running.
func (m Model) composerView() string {
	rendered := m.input.View()
	if m.running {
		return m.palette.dim().Render(rendered)
	}
	return rendered
}

// workingLine shows the running status above the composer.
func (m Model) workingLine() string {
	if !m.running {
		return ""
	}
	return m.palette.dim().Render("· " + m.status + " (esc/ctrl+c to interrupt)")
}

// banner is the header shown once at startup.
func (m Model) banner() string {
	parts := []string{m.palette.accent().Render("equus " + m.version)}
	if m.model != "" {
		parts = append(parts, m.palette.dim().Render(m.model))
	}
	if m.cwd != "" {
		parts = append(parts, m.palette.dim().Render(m.cwd))
	}
	line := strings.Join(parts, m.palette.dim().Render(" · "))
	return xansi.Truncate(line, m.width, "")
}
