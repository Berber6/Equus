package tui

import "testing"

func countTrue(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

func TestScrollbarHiddenWhenFits(t *testing.T) {
	if got := scrollbar(5, 5, 0); got != nil {
		t.Fatalf("expected nil when content fits, got %v", got)
	}
	if got := scrollbar(3, 10, 0); got != nil {
		t.Fatalf("expected nil when content smaller, got %v", got)
	}
	if got := scrollbar(10, 0, 0); got != nil {
		t.Fatalf("expected nil for zero rows, got %v", got)
	}
}

func TestScrollbarThumbAtBottomWhenPinned(t *testing.T) {
	bar := scrollbar(100, 10, 0) // offset 0 = pinned to bottom
	if len(bar) != 10 {
		t.Fatalf("len = %d", len(bar))
	}
	// Thumb should occupy the bottom rows.
	if !bar[len(bar)-1] {
		t.Fatalf("thumb should be at the bottom: %v", bar)
	}
	if bar[0] {
		t.Fatalf("thumb should not be at the top: %v", bar)
	}
}

func TestScrollbarThumbAtTopWhenScrolledUp(t *testing.T) {
	bar := scrollbar(100, 10, 90) // offset = maxOffset = 100-10
	if !bar[0] {
		t.Fatalf("thumb should be at the top: %v", bar)
	}
	if bar[len(bar)-1] {
		t.Fatalf("thumb should not be at the bottom: %v", bar)
	}
}

func TestScrollbarThumbAtLeastOneCell(t *testing.T) {
	bar := scrollbar(10_000, 10, 0)
	if countTrue(bar) < 1 {
		t.Fatalf("thumb must be at least 1 cell: %v", bar)
	}
}
