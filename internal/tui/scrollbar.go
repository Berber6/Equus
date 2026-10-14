package tui

// scrollbar computes a 1-column vertical scrollbar for the transcript region.
// It returns one entry per visible row: true where the thumb sits. A nil result
// means the scrollbar should not be drawn (everything fits, or no rows).
//
// total is the number of transcript lines, rows the viewport height, and offset
// how many lines above the tail are scrolled (0 = pinned to the bottom).
func scrollbar(total, rows, offset int) []bool {
	if rows <= 0 || total <= rows {
		return nil
	}
	thumb := rows * rows / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > rows {
		thumb = rows
	}
	maxOffset := total - rows
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	// offset counts up from the bottom; convert to a distance from the top.
	topPosition := maxOffset - offset
	start := 0
	if maxOffset > 0 {
		start = topPosition * (rows - thumb) / maxOffset
	}
	result := make([]bool, rows)
	for i := start; i < start+thumb && i < rows; i++ {
		result[i] = true
	}
	return result
}
