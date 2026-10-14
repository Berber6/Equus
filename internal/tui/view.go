package tui

import (
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/textarea"
	xansi "github.com/charmbracelet/x/ansi"
)

// viewFrame counts renders. Bubble Tea skips lines identical to the previous
// frame (emitting a bare "\n" for them); when the frame content shifts — e.g.
// the composer growing for a multi-line input — that index-based skip drops
// rows. Tagging every line with an alternating zero-width marker changes the
// byte content each frame and forces a correct full repaint.
var viewFrame atomic.Uint64

func frameTag() string {
	if viewFrame.Add(1)%2 == 0 {
		return ""
	}
	return "​" // zero-width space: differs from nothing, width 0
}

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
	popup := m.popupView()

	fixedRows := len(composer) + len(popup)
	if working != "" {
		fixedRows++
	}
	fixedRows++ // status

	historyRows := m.height - fixedRows
	if historyRows < 0 {
		historyRows = 0
	}
	history := m.transcriptWindow(historyRows)
	if m.picker != nil {
		history = m.renderSessionPicker(historyRows)
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, history...)
	if working != "" {
		lines = append(lines, working)
	}
	lines = append(lines, popup...)
	lines = append(lines, composer...)
	lines = append(lines, status)
	tag := frameTag()
	for index, line := range lines {
		lines[index] = padToWidth(line, maxInt(1, m.width-1)) + tag
	}
	return strings.Join(lines, "\n")
}

// transcriptWindow returns the visible slice of the transcript for the given
// number of rows, honoring the scroll offset (lines above the tail). It also
// appends the in-flight assistant stream buffer at the tail.
func (m Model) transcriptWindow(rows int) []string {
	all := append([]string{}, m.transcript...)
	if active := strings.TrimSpace(m.active); active != "" {
		all = append(all, m.assistantEntry(m.active)...)
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

// approvalView renders the pending approval prompt in place of the composer.
func (m Model) approvalView() []string {
	rows := []string{m.backgroundRow("")}
	for _, line := range m.wrapLines(m.palette.bold().Render("Approve ") + m.palette.tool().Render(m.pending.summary) + "?") {
		rows = append(rows, m.backgroundRow(line))
	}
	rows = append(rows, m.backgroundRow(m.palette.dim().Render("[y] once   [a] session   [p] always   [n] deny")))
	rows = append(rows, m.backgroundRow(""))
	return rows
}

// composerView renders the input panel as a dynamic-height, full-width
// background band: one padding row, the visible content rows, and one padding
// row. Content beyond maxInputRows scrolls inside the panel.
func (m Model) composerView() []string {
	rows := []string{m.backgroundRow("")} // top padding
	for _, line := range m.visibleInputLines() {
		rows = append(rows, m.backgroundRow(line))
	}
	rows = append(rows, m.backgroundRow("")) // bottom padding
	return rows
}

// backgroundRow pads one physical line to just under the terminal width and
// paints the whole row with the input background. The content is stripped of
// its own SGR resets first so the fill covers the entire row (including the
// placeholder state and trailing columns).
func (m Model) backgroundRow(content string) string {
	plain := strings.TrimRight(xansi.Strip(content), " ")
	target := maxInt(1, m.width-1)
	if pad := target - xansi.StringWidth(plain); pad > 0 {
		plain += strings.Repeat(" ", pad)
	} else {
		plain = xansi.Truncate(plain, target, "")
	}
	return m.palette.inputBackground().Render(plain)
}

// backgroundFill is a full-width blank row in the input background color.
func (m Model) backgroundFill() string {
	return m.palette.inputBackground().Render(strings.Repeat(" ", maxInt(1, m.width)))
}

// visibleInputLines renders the textarea and returns exactly the rows to show,
// scrolling the window to keep the cursor visible when the content exceeds
// maxInputRows.
//
// It renders a *copy* at a tall, unclamped height. Rendering the live textarea
// (or a short copy) leaves its internal viewport scrolled to the cursor, which
// drops every line above the cursor — the bug where only the last line of a
// multi-line input appeared.
func (m Model) visibleInputLines() []string {
	probe := m.input
	probe.MaxHeight = 0 // no clamp: let SetHeight take effect as-is
	probe.SetHeight(maxInputRows)
	lines := strings.Split(strings.TrimRight(probe.View(), "\n"), "\n")
	if len(lines) < 1 {
		lines = []string{""}
	}
	// Trim trailing blank rows so the panel only shows the rows in use, but
	// never below the cursor row (a trailing newline keeps an empty cursor
	// line visible).
	cursorRow := textareaCursorRow(m.input, m.widthForWrap())
	minRows := cursorRow + 1
	for len(lines) > minRows && strings.TrimSpace(xansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= maxInputRows {
		return lines
	}
	start := cursorRow - maxInputRows + 1
	if start < 0 {
		start = 0
	}
	if maxStart := len(lines) - maxInputRows; start > maxStart {
		start = maxStart
	}
	return lines[start : start+maxInputRows]
}

// textareaCursorRow estimates the visual row of the cursor within the rendered
// input, by counting the wrapped rows of all lines before the cursor line and
// adding the cursor's row offset within its line.
func textareaCursorRow(input textarea.Model, wrapWidth int) int {
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	logical := strings.Split(input.Value(), "\n")
	cursorLine := input.Line()
	if cursorLine < 0 {
		cursorLine = 0
	}
	if cursorLine >= len(logical) {
		cursorLine = len(logical) - 1
	}
	row := 0
	for index, line := range logical {
		if index == cursorLine {
			return row + input.LineInfo().RowOffset
		}
		width := xansi.StringWidth(line)
		row += maxInt(1, (width+wrapWidth-1)/wrapWidth)
	}
	return row
}

// widthForWrap returns the input's usable wrap width.
func (m Model) widthForWrap() int {
	return m.width - inputPromptWidth()
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
