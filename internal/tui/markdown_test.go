package tui

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func mdPalette() palette { return palette{level: colorLevelTrueColor, dark: true} }

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRight(xansi.Strip(l), " ")
	}
	return out
}

func TestRenderHeading(t *testing.T) {
	lines := renderMarkdown("# Title\n\n## Sub", mdPalette(), 40, false)
	joined := strings.Join(plainLines(lines), "\n")
	if !strings.Contains(joined, "Title") || !strings.Contains(joined, "Sub") {
		t.Fatalf("headings missing: %q", joined)
	}
	// The hash markers must be stripped.
	if strings.Contains(joined, "# Title") {
		t.Fatalf("# marker not stripped: %q", joined)
	}
}

func TestRenderBulletAndOrdered(t *testing.T) {
	lines := renderMarkdown("- first\n- second\n1. one\n2. two", mdPalette(), 40, false)
	plain := plainLines(lines)
	if !strings.Contains(plain[0], "• first") {
		t.Fatalf("bullet = %q", plain[0])
	}
	if !strings.Contains(plain[2], "1. one") {
		t.Fatalf("ordered = %q", plain[2])
	}
}

func TestRenderInlineCodeAndBold(t *testing.T) {
	lines := renderMarkdown("use `code` and **bold** here", mdPalette(), 40, false)
	got := strings.Join(plainLines(lines), "\n")
	if !strings.Contains(got, "use code and bold here") {
		t.Fatalf("inline markers not stripped: %q", got)
	}
}

func TestRenderCodeBlockUnknownLanguage(t *testing.T) {
	lines := renderMarkdown("```\nplain text\n```", mdPalette(), 40, false)
	got := strings.Join(plainLines(lines), "\n")
	if !strings.Contains(got, "plain text") {
		t.Fatalf("code content missing: %q", got)
	}
	// No panic with an unknown/empty language.
}

func TestRenderCodeBlockHighlighted(t *testing.T) {
	src := "```go\nfunc main() {}\n```"
	lines := renderMarkdown(src, mdPalette(), 60, true)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "\x1b[") {
		t.Fatalf("expected ANSI highlighting: %q", joined)
	}
	if !strings.Contains(xansi.Strip(joined), "func main") {
		t.Fatalf("code text missing: %q", xansi.Strip(joined))
	}
}

func TestRenderQuotePrefix(t *testing.T) {
	lines := renderMarkdown("> quoted line", mdPalette(), 40, false)
	if !strings.Contains(xansi.Strip(lines[0]), "│ quoted line") {
		t.Fatalf("quote = %q", xansi.Strip(lines[0]))
	}
}

func TestRenderTableAlignment(t *testing.T) {
	src := "| Name | Qty |\n|:-----|----:|\n| apple | 3 |\n| fig | 12 |"
	lines := renderMarkdown(src, mdPalette(), 60, false)
	plain := plainLines(lines)
	// Header + separator + 2 rows = 4 lines.
	if len(plain) < 4 {
		t.Fatalf("table lines = %d: %v", len(plain), plain)
	}
	if !strings.Contains(plain[0], "Name") || !strings.Contains(plain[0], "Qty") {
		t.Fatalf("header = %q", plain[0])
	}
	// Right-aligned Qty column: "3" should be padded to the right edge.
	if !strings.HasSuffix(plain[2], "3") {
		t.Fatalf("expected right-aligned value: %q", plain[2])
	}
}

func TestRenderMarkdownNoColorStrips(t *testing.T) {
	noColor := palette{noColor: true, level: colorLevelNone, dark: true}
	lines := renderMarkdown("# Hi\n\n`x` and **y**", noColor, 40, true)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "\x1b[") {
		t.Fatalf("no-color output should not contain SGR: %q", joined)
	}
}
