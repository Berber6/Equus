package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// View renders the managed bottom region only: an optional working line and
// the input composer. The conversation itself is emitted through tea.Println
// into the terminal scrollback, not drawn here.
func (m Model) View() string {
	composer := m.composerView()
	if working := m.workingLine(); working != "" {
		return working + "\n" + composer
	}
	return composer
}

// composerView renders the input box, dimmed while a turn is running.
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
	return strings.Join(parts, m.palette.dim().Render(" · "))
}

// userLine renders a submitted prompt for the scrollback.
func (m Model) userLine(text string) string {
	return m.palette.user().Render("› " + text)
}

// assistantLine renders the committed assistant answer for the scrollback.
func (m Model) assistantLine(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for index, line := range lines {
		lines[index] = "  " + line
	}
	return strings.Join(lines, "\n")
}

// inputPromptWidth is the display width of the input prompt glyph.
func inputPromptWidth() int {
	return xansi.StringWidth(inputPrompt)
}
