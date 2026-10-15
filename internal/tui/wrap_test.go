package tui

import (
	"strings"
	"testing"
	"time"
)

// A word wider than the wrap width (e.g. a double-width CJK rune with width 1)
// must not hang the wrapper.
func TestWrapTextOverWideWordTerminates(t *testing.T) {
	done := make(chan []string, 1)
	go func() {
		done <- wrapText("中文字很长的字符串", 1)
	}()
	select {
	case lines := <-done:
		for _, line := range lines {
			if line == "" {
				t.Fatalf("empty line produced: %v", lines)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wrapText hung on an over-wide word (infinite loop)")
	}
}

func TestWrapTextNormal(t *testing.T) {
	lines := wrapText("the quick brown fox", 10)
	for _, line := range lines {
		if len(line) > 10 {
			t.Fatalf("line too wide: %q", line)
		}
	}
	if !strings.Contains(strings.Join(lines, " "), "quick") {
		t.Fatalf("content lost: %v", lines)
	}
}
