package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// tableAlign is a column's horizontal alignment.
type tableAlign uint8

const (
	alignLeft tableAlign = iota
	alignCenter
	alignRight
)

// isTableRow reports whether a trimmed line looks like a table row.
func isTableRow(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

// isTableSeparator reports a markdown table separator row (| --- | :--: |).
func isTableSeparator(trimmed string) bool {
	if !isTableRow(trimmed) {
		return false
	}
	for _, cell := range parseTableRow(trimmed) {
		cell = strings.TrimSpace(cell)
		cell = strings.Trim(cell, ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

// parseTableRow splits a table row into trimmed cells (borders removed).
func parseTableRow(trimmed string) []string {
	trimmed = strings.Trim(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

// parseTableAligns reads per-column alignment from the separator row.
func parseTableAligns(trimmed string) []tableAlign {
	cells := parseTableRow(trimmed)
	aligns := make([]tableAlign, len(cells))
	for i, cell := range cells {
		cell = strings.TrimSpace(cell)
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			aligns[i] = alignCenter
		case right:
			aligns[i] = alignRight
		default:
			aligns[i] = alignLeft
		}
	}
	return aligns
}

// renderTable renders a markdown table with computed column widths, per-column
// alignment, cell wrapping, and a styled header.
func renderTable(header []string, rows [][]string, aligns []tableAlign, p palette, width int) []string {
	columns := len(header)
	for _, row := range rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if columns == 0 {
		return nil
	}

	allRows := append([][]string{header}, rows...)

	// Natural column widths from the (trimmed) cell text.
	natural := make([]int, columns)
	for _, row := range allRows {
		for c := 0; c < columns; c++ {
			cell := ""
			if c < len(row) {
				cell = strings.TrimSpace(row[c])
			}
			if w := xansi.StringWidth(cell); w > natural[c] {
				natural[c] = w
			}
		}
	}
	widths := fitColumnWidths(natural, columns, maxInt(20, width-2))

	// Wrap each cell to its column width.
	wrapped := make([][][]string, len(allRows))
	for r, row := range allRows {
		rowCells := make([][]string, columns)
		for c := 0; c < columns; c++ {
			cell := ""
			if c < len(row) {
				cell = strings.TrimSpace(row[c])
			}
			rowCells[c] = wrapText(cell, widths[c])
		}
		wrapped[r] = rowCells
	}

	out := make([]string, 0, len(wrapped)+1)
	for r, rowCells := range wrapped {
		height := 1
		for c := range rowCells {
			if len(rowCells[c]) > height {
				height = len(rowCells[c])
			}
		}
		for line := 0; line < height; line++ {
			cells := make([]string, columns)
			for c := 0; c < columns; c++ {
				text := ""
				if line < len(rowCells[c]) {
					text = rowCells[c][line]
				}
				cells[c] = alignCell(text, widths[c], aligns[c])
			}
			joined := strings.Join(cells, p.border().Render(" │ "))
			if r == 0 {
				out = append(out, p.strong().Render(joined))
			} else {
				out = append(out, joined)
			}
		}
		if r == 0 {
			ruleWidth := 0
			for _, w := range widths {
				ruleWidth += w
			}
			ruleWidth += 3 * maxInt(0, columns-1) // " │ " separators
			out = append(out, p.border().Render(strings.Repeat("─", ruleWidth)))
		}
	}
	return out
}

// alignCell pads a cell to width per alignment.
func alignCell(text string, width int, align tableAlign) string {
	pad := width - xansi.StringWidth(text)
	if pad <= 0 {
		return xansi.Truncate(text, width, "")
	}
	switch align {
	case alignRight:
		return strings.Repeat(" ", pad) + text
	case alignCenter:
		left := pad / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", pad-left)
	default:
		return text + strings.Repeat(" ", pad)
	}
}

// fitColumnWidths shrinks column widths proportionally to fit the budget.
func fitColumnWidths(natural []int, columns, budget int) []int {
	widths := append([]int(nil), natural...)
	total := 0
	for _, w := range widths {
		total += w
	}
	// Overhead: separators " │ " between columns.
	overhead := 3 * maxInt(0, columns-1)
	for total+overhead > budget && columns > 0 {
		// Shrink the widest column by one until it fits or all are at minimum.
		widest, index := -1, -1
		for i, w := range widths {
			if w > widest {
				widest, index = w, i
			}
		}
		if widest <= 3 {
			break
		}
		widths[index]--
		total--
	}
	return widths
}

// wrapText wraps plain text to at most width display columns on word
// boundaries, breaking over-long words.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := ""
	for _, word := range words {
		for xansi.StringWidth(word) > width {
			// Break a too-long word.
			head := xansi.Truncate(word, width, "")
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, head)
			word = strings.TrimPrefix(word, head)
		}
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if xansi.StringWidth(candidate) > width {
			lines = append(lines, current)
			current = word
		} else {
			current = candidate
		}
	}
	if current != "" || len(lines) == 0 {
		lines = append(lines, current)
	}
	return lines
}
