package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// The startup wordmark, drawn as terminal-safe Braille glyphs. Wide is used on
// roomy terminals; compact is the fallback.
const wideLogoArt = `
⠑⠢⡀ ⠄ ⢀⣀⡀⢀⣀⡀ ⠄ ⠄ ⢀⣀⡀ ⠢⠢⡀
⠆ ⠇ ⠇ ⠇   ⠇ ⠇ ⠇ ⠇ ⠇   ⠆⠂⠲
⠇ ⠇ ⠇ ⠇   ⠇ ⠇⠄⠇ ⠇   ⠇⠙⠆
⠘⠤⠂ ⠇ ⠘⠤⠂⠤⠤⠂⠤⠂⠘⠤⠂ ⠇ ⠙⠤⠂⢠`

const compactLogoArt = `
⠑⠢⡀ ⢀⣀⡀ ⠄ ⠄ ⢀⣀⡀ ⠢⠢⡀
⠆ ⠇ ⠇   ⠇ ⠇ ⠇ ⠇ ⠇   ⠆⠂⠲
⠘⠤⠂ ⠘⠤⠂ ⠤⠂⠘⠤ ⠇ ⠙⠤⠂⢠`

const (
	wideLogoGap    = 6
	compactLogoGap = 3
)

var (
	wideLogo             = logoWordmark(wideLogoArt)
	compactLogo          = logoWordmark(compactLogoArt)
	wideLogoMinimumWidth = logoWidth(wideLogo)
)

// logoWordmark trims the leading/trailing blank lines of the raw art.
func logoWordmark(art string) string {
	return strings.Trim(art, "\n")
}

// startLogo selects the wordmark for the given width.
func startLogo(width int) string {
	if width >= wideLogoMinimumWidth {
		return wideLogo
	}
	return compactLogo
}

func logoWidth(value string) int {
	width := 0
	for _, line := range strings.Split(value, "\n") {
		if w := lipgloss.Width(line); w > width {
			width = w
		}
	}
	return width
}

// logoLines renders the startup header: the wordmark plus a bordered info box
// (name/version, model, working directory), for insertion into the transcript.
func (m Model) logoLines() []string {
	width := maxInt(20, m.width)
	art := startLogo(width)
	lines := strings.Split(art, "\n")

	boxWidth := minInt(width, 60)
	if boxWidth < 24 {
		boxWidth = minInt(width, 24)
	}
	inner := maxInt(8, boxWidth-2)
	border := m.palette.dim()

	header := m.palette.accent().Render(">_ ") + m.palette.bold().Render("Equus")
	if m.version != "" {
		header += m.palette.dim().Render(" (" + m.version + ")")
	}
	lines = append(lines, "")
	lines = append(lines, border.Render("╭"+strings.Repeat("─", inner)+"╮"))
	lines = append(lines, boxLine("│ ", header, inner))
	lines = append(lines, boxLine("│ ", "", inner))
	if m.model != "" {
		lines = append(lines, boxLine("│ ", m.palette.dim().Render("model:     ")+m.model, inner))
	}
	if m.cwd != "" {
		lines = append(lines, boxLine("│ ", m.palette.dim().Render("directory: ")+m.cwd, inner))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return lines
}

// boxLine renders one info-box row padded to the inner width with a right
// border.
func boxLine(prefix, content string, inner int) string {
	used := lipgloss.Width(prefix) + lipgloss.Width(content)
	if used > inner {
		content = truncateWidth(content, maxInt(0, inner-lipgloss.Width(prefix)))
		used = lipgloss.Width(prefix) + lipgloss.Width(content)
	}
	return prefix + content + strings.Repeat(" ", maxInt(0, inner-used)) + " │"
}

// truncateWidth truncates a styled string to a display width.
func truncateWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return xansi.Truncate(text, width, "")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
