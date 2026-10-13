package tui

import (
	"io"
	"os"
)

// IsTerminal reports whether the reader is an interactive terminal. It is a
// seam so callers can override detection in tests.
var IsTerminal = defaultIsTerminal

func defaultIsTerminal(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// supportsColor reports whether the terminal advertises color support.
func supportsColor() bool {
	term := os.Getenv("TERM")
	return term != "" && term != "dumb" && term != "linux"
}
