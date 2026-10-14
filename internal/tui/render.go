package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/Godric-W/Equus/internal/llm"
)

// userEntry renders a submitted prompt: a "› " marker on a subtle background
// fill spanning the full width, mirroring the reference agent's user message.
func (m Model) userEntry(text string) []string {
	style := m.palette.user().Width(maxInt(1, m.width))
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	for index, line := range lines {
		marker := "  "
		if index == 0 {
			marker = "› "
		}
		rendered = append(rendered, style.Render(marker+line))
	}
	return rendered
}

// assistantEntry renders a committed assistant answer: as markdown when
// enabled, otherwise as plain text. The first line carries a "• " marker;
// subsequent lines are indented to align under it.
func (m Model) assistantEntry(text string) []string {
	var body []string
	if m.markdown {
		body = renderMarkdown(text, m.palette, m.width, m.syntaxHighlight)
	} else {
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			body = append(body, m.wrapLines(line)...)
		}
	}
	if len(body) == 0 {
		return nil
	}
	out := make([]string, 0, len(body))
	for index, line := range body {
		prefix := "  "
		if index == 0 {
			prefix = m.palette.dim().Render("• ")
		}
		out = append(out, prefix+line)
	}
	return out
}

// toolCallEntry renders a tool invocation: "• Ran <name> <detail>".
func (m Model) toolCallEntry(name, detail string) []string {
	head := m.palette.dim().Render("• ") + m.palette.strong().Render("Ran ") + m.palette.tool().Render(name)
	if strings.TrimSpace(detail) != "" {
		head += " " + detail
	}
	return m.wrapLines(head)
}

// toolResultEntry renders a tool result under its call: "  └ <summary>".
func (m Model) toolResultEntry(name, result string) []string {
	summary := firstLine(result)
	if strings.TrimSpace(summary) == "" {
		summary = "(no output)"
	}
	line := m.palette.dim().Render("  └ ") + m.palette.dim().Render(name+": "+summary)
	return m.wrapLines(line)
}

// errorEntry renders an error in the failure style.
func (m Model) errorEntry(text string) []string {
	return m.wrapLines(m.palette.dim().Render("• ") + m.palette.failure().Render(text))
}

// usageEntry renders a token usage line.
func (m Model) usageEntry(text string) []string {
	return []string{m.palette.dim().Render(text)}
}

// wrapLines hard-wraps a (possibly multi-line, possibly styled) block to the
// terminal width so it fits the transcript region.
func (m Model) wrapLines(block string) []string {
	width := maxInt(1, m.width)
	out := make([]string, 0, 4)
	for _, line := range strings.Split(block, "\n") {
		if xansi.StringWidth(line) <= width {
			out = append(out, line)
			continue
		}
		out = append(out, strings.Split(xansi.Hardwrap(line, width, true), "\n")...)
	}
	return out
}

// appendTranscript adds rendered lines to the transcript, committing nothing if
// the block is empty.
func (m *Model) appendTranscript(lines ...string) {
	if len(lines) == 0 {
		return
	}
	m.transcript = append(m.transcript, lines...)
}

// renderHistory rebuilds transcript lines from restored conversation items.
func renderHistory(m Model, items []llm.ResponseItem) []string {
	var lines []string
	callNames := map[string]string{} // tool_call id -> tool name
	for _, item := range items {
		switch item.Role {
		case llm.RoleUser:
			lines = append(lines, "")
			lines = append(lines, m.userEntry(item.Content)...)
		case llm.RoleAssistant:
			if strings.TrimSpace(item.Content) != "" {
				lines = append(lines, m.assistantEntry(item.Content)...)
			}
			for _, call := range item.ToolCalls {
				callNames[call.ID] = call.Name
				lines = append(lines, m.toolCallEntry(call.Name, string(call.Arguments))...)
			}
		case llm.RoleTool:
			name := callNames[item.ToolCallID]
			if name == "" {
				name = "tool"
			}
			lines = append(lines, m.toolResultEntry(name, item.Content)...)
		}
	}
	return lines
}

// padToWidth pads a rendered line with spaces (or truncates it) so it fills
// exactly width display columns.
func padToWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	line = xansi.Truncate(line, width, "")
	padding := width - xansi.StringWidth(line)
	if padding <= 0 {
		return line
	}
	return line + strings.Repeat(" ", padding)
}

// inputPromptWidth is the display width of the input prompt glyph.
func inputPromptWidth() int {
	return xansi.StringWidth(inputPrompt)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
