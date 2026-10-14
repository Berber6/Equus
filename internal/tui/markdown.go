package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// maxHighlightBytes caps how much code is sent to the syntax highlighter;
// larger blocks are emitted verbatim to avoid stalls.
const maxHighlightBytes = 16 << 10

// renderMarkdown renders a subset of markdown (headings, lists, quotes, fenced
// code with syntax highlighting, tables, and inline code/emphasis) into styled
// terminal lines. It is a deliberately small, line-oriented renderer.
func renderMarkdown(source string, p palette, width int, highlight bool) []string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Fenced code block.
		if fence, lang, ok := codeFence(trimmed); ok {
			i++
			block := []string{}
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
				block = append(block, lines[i])
				i++
			}
			out = append(out, renderCodeBlock(block, lang, p, highlight)...)
			continue
		}
		// Table: a header row followed by a separator row.
		if isTableRow(trimmed) && i+1 < len(lines) && isTableSeparator(strings.TrimSpace(lines[i+1])) {
			header := parseTableRow(trimmed)
			aligns := parseTableAligns(strings.TrimSpace(lines[i+1]))
			i += 2
			var rows [][]string
			for i < len(lines) && isTableRow(strings.TrimSpace(lines[i])) {
				rows = append(rows, parseTableRow(strings.TrimSpace(lines[i])))
				i++
			}
			i-- // the loop advanced past the last table row
			out = append(out, renderTable(header, rows, aligns, p, width)...)
			continue
		}
		switch {
		case trimmed == "":
			out = append(out, "")
		case headingLevel(trimmed) > 0:
			level := headingLevel(trimmed)
			text := strings.TrimSpace(trimmed[level:])
			out = append(out, renderInline(headingStyleFor(p, level), text, p))
		case strings.HasPrefix(trimmed, "> "):
			out = append(out, renderQuote(strings.TrimSpace(trimmed[2:]), p, width)...)
		case isBullet(trimmed):
			out = append(out, renderBullet(trimmed, p))
		case isOrdered(trimmed):
			out = append(out, renderOrdered(trimmed, p))
		case isHorizontalRule(trimmed):
			out = append(out, p.dim().Render(strings.Repeat("─", maxInt(1, width-2))))
		default:
			out = append(out, renderInline(p.plain(), line, p))
		}
	}
	return out
}

// codeFence detects a ``` or ~~~ fence, returning the fence string and language.
func codeFence(trimmed string) (fence, lang string, ok bool) {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, f) {
			return f, strings.TrimSpace(trimmed[len(f):]), true
		}
	}
	return "", "", false
}

// renderCodeBlock emits a fenced code block, syntax-highlighted when possible.
func renderCodeBlock(block []string, lang string, p palette, highlight bool) []string {
	code := strings.Join(block, "\n")
	body := code
	if highlight && p.rich() && len(code) <= maxHighlightBytes {
		if highlighted := highlightCode(code, lang, p); highlighted != "" {
			body = highlighted
		}
	}
	rendered := strings.Split(strings.TrimRight(body, "\n"), "\n")
	out := make([]string, 0, len(rendered)+2)
	out = append(out, p.dim().Render("  ┌─"+codeLangLabel(lang)))
	for _, line := range rendered {
		out = append(out, "  "+p.dim().Render("│ ")+line)
	}
	out = append(out, p.dim().Render("  └─"))
	return out
}

func codeLangLabel(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return ""
	}
	return " " + lang
}

// highlightCode returns the syntax-highlighted code, or "" when highlighting is
// unavailable.
func highlightCode(code, lang string, p palette) string {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		return ""
	}
	lexer = chroma.Coalesce(lexer)
	formatter := formatters.Get(p.chromaFormatter())
	if formatter == nil {
		return ""
	}
	style := markdownChromaStyle(p)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return ""
	}
	var builder strings.Builder
	if err := formatter.Format(&builder, style, iterator); err != nil {
		return ""
	}
	return strings.TrimRight(builder.String(), "\n")
}

// chromaFormatter maps the palette's color depth to a chroma formatter name.
func (p palette) chromaFormatter() string {
	switch p.level {
	case colorLevelTrueColor:
		return "terminal16m"
	case colorLevelANSI256:
		return "terminal256"
	case colorLevelANSI16:
		return "terminal16"
	default:
		return ""
	}
}

// markdownChromaStyle picks a built-in chroma style by terminal background.
func markdownChromaStyle(p palette) *chroma.Style {
	name := "github"
	if p.dark {
		name = "dracula"
	}
	if style := styles.Get(name); style != nil {
		return style
	}
	return styles.Fallback
}

// headingStyle returns the style for a markdown heading level.
func (m Model) headingStyle(level int) lipgloss.Style {
	return headingStyleFor(m.palette, level)
}

// headingStyleFor maps a heading level to a style.
func headingStyleFor(p palette, level int) lipgloss.Style {
	switch level {
	case 1:
		return p.accent().Underline(true)
	case 2:
		return p.strong()
	case 3:
		return p.strong().Italic(true)
	default:
		return p.plain().Italic(true)
	}
}

// isBullet reports an unordered list marker.
func isBullet(trimmed string) bool {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	return false
}

func renderBullet(trimmed string, p palette) string {
	content := strings.TrimSpace(trimmed[2:])
	return p.accent().Render("• ") + renderInline(p.plain(), content, p)
}

// isOrdered reports an ordered list marker ("1. ").
func isOrdered(trimmed string) bool {
	dot := strings.IndexByte(trimmed, '.')
	if dot <= 0 || dot+1 >= len(trimmed) || trimmed[dot+1] != ' ' {
		return false
	}
	for _, r := range trimmed[:dot] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func renderOrdered(trimmed string, p palette) string {
	dot := strings.IndexByte(trimmed, '.')
	marker := trimmed[:dot+1]
	content := strings.TrimSpace(trimmed[dot+1:])
	return p.accent().Render(marker+" ") + renderInline(p.plain(), content, p)
}

func isHorizontalRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != '*' && r != '_' {
			return false
		}
	}
	return true
}

func headingLevel(trimmed string) int {
	count := 0
	for count < len(trimmed) && trimmed[count] == '#' {
		count++
	}
	if count == 0 || count > 6 || count >= len(trimmed) || trimmed[count] != ' ' {
		return 0
	}
	return count
}

// renderQuote renders a block-quote line with a bar prefix.
func renderQuote(content string, p palette, width int) []string {
	wrapped := wrapPlain(content, maxInt(4, width-4))
	out := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		out = append(out, p.dim().Render("│ ")+p.success().Render(line))
	}
	return out
}

func wrapPlain(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	return wrapText(text, width)
}

// renderInline applies inline markdown (code, bold, italic) to a line.
func renderInline(base lipgloss.Style, text string, p palette) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "`"):
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				out.WriteString(p.tool().Render(text[i+1 : i+1+end]))
				i += end + 2
				continue
			}
		case strings.HasPrefix(text[i:], "**"):
			if end := strings.Index(text[i+2:], "**"); end >= 0 {
				out.WriteString(base.Bold(true).Render(text[i+2 : i+2+end]))
				i += end + 4
				continue
			}
		case strings.HasPrefix(text[i:], "*"):
			if end := strings.IndexByte(text[i+1:], '*'); end > 0 {
				out.WriteString(base.Italic(true).Render(text[i+1 : i+1+end]))
				i += end + 2
				continue
			}
		}
		next := strings.IndexAny(text[i:], "`*")
		if next < 0 {
			out.WriteString(base.Render(text[i:]))
			break
		}
		out.WriteString(base.Render(text[i : i+next]))
		i += next
	}
	return out.String()
}
