package tui

import (
	"strings"
)

// View draws the whole alternate screen: the scrolling transcript, an optional
// working line, the input composer panel, and the status line. The total
// height always equals the terminal height, and every line is padded to the
// terminal width.
func (m Model) View() string {
	if m.height <= 0 {
		return ""
	}
	composer := m.composerView() // panelRows lines
	if m.pending != nil {
		composer = m.approvalView()
	}
	working := m.workingLine()
	status := m.statusLine()

	fixedRows := len(composer)
	if working != "" {
		fixedRows++
	}
	fixedRows++ // status

	historyRows := m.height - fixedRows
	if historyRows < 0 {
		historyRows = 0
	}
	history := m.transcriptWindow(historyRows)

	lines := make([]string, 0, m.height)
	lines = append(lines, history...)
	if working != "" {
		lines = append(lines, working)
	}
	lines = append(lines, composer...)
	lines = append(lines, status)
	for index, line := range lines {
		lines[index] = padToWidth(line, m.width)
	}
	return strings.Join(lines, "\n")
}

// transcriptWindow returns the visible slice of the transcript for the given
// number of rows, honoring the scroll offset (lines above the tail). It also
// appends the in-flight assistant stream buffer at the tail.
func (m Model) transcriptWindow(rows int) []string {
	all := append([]string{}, m.transcript...)
	if active := strings.TrimSpace(m.active); active != "" {
		all = append(all, m.wrapLines(m.assistantLine(m.active))...)
	}
	if rows <= 0 {
		return nil
	}
	offset := m.scroll
	if offset < 0 {
		offset = 0
	}
	if maxOffset := len(all) - rows; offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := len(all) - offset
	start := end - rows
	if start < 0 {
		start = 0
	}
	window := append([]string{}, all[start:end]...)
	// Top-align when the transcript is shorter than the window.
	for len(window) < rows {
		window = append([]string{""}, window...)
	}
	return window
}

// approvalView renders the pending approval prompt in place of the composer,
// occupying the same panelRows rows so the layout stays fixed.
func (m Model) approvalView() []string {
	rows := make([]string, 0, panelRows)
	rows = append(rows, m.backgroundFill())
	summary := m.wrapLines(m.palette.bold().Render("Approve ") + m.palette.tool().Render(m.pending.summary) + "?")
	rows = append(rows, summary...)
	rows = append(rows, m.palette.dim().Render("[y] once   [a] allow for session   [n] deny"))
	rows = append(rows, m.backgroundFill())
	// Fit to exactly panelRows.
	for len(rows) < panelRows {
		rows = append(rows, m.backgroundFill())
	}
	if len(rows) > panelRows {
		rows = rows[:panelRows]
	}
	return rows
}

// composerView renders the input panel as a fixed panelRows rows with a
// full-width background band: one padding row, the textarea content, and one
// padding row. The textarea already carries the background on its own styles.
func (m Model) composerView() []string {
	contentRows := maxInt(1, panelRows-2)
	rendered := strings.Split(strings.TrimRight(m.input.View(), "\n"), "\n")
	for len(rendered) < contentRows {
		rendered = append(rendered, "")
	}
	if len(rendered) > contentRows {
		rendered = rendered[len(rendered)-contentRows:]
	}

	rows := make([]string, 0, panelRows)
	rows = append(rows, m.backgroundFill())
	for _, line := range rendered {
		rows = append(rows, padToWidth(line, m.width))
	}
	rows = append(rows, m.backgroundFill())
	return rows
}

// backgroundFill is a full-width blank row in the input background color.
func (m Model) backgroundFill() string {
	return m.palette.inputBackground().Render(strings.Repeat(" ", maxInt(1, m.width)))
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
