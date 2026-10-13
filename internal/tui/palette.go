package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// colorLevel describes how much color the terminal can render.
type colorLevel uint8

const (
	colorLevelNone colorLevel = iota
	colorLevelANSI16
	colorLevelANSI256
	colorLevelTrueColor
)

// palette holds the styles used to render the interface. Syntax highlighting
// and theme lookups are omitted because Equus renders plain text.
type palette struct {
	level   colorLevel
	dark    bool
	noColor bool
}

func detectPalette(noColor bool) palette {
	if noColor || !supportsColor() {
		return palette{level: colorLevelNone, noColor: true, dark: true}
	}
	return palette{level: detectColorLevel(), dark: detectDarkBackground()}
}

// detectColorLevel inspects COLORTERM/TERM to guess the color depth.
func detectColorLevel() colorLevel {
	colorTerm := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		return colorLevelTrueColor
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	switch {
	case strings.Contains(term, "256color"):
		return colorLevelANSI256
	case term == "" || term == "dumb":
		return colorLevelNone
	default:
		return colorLevelANSI16
	}
}

// detectDarkBackground respects COLORFGBG when present, defaulting to dark.
func detectDarkBackground() bool {
	value := strings.TrimSpace(os.Getenv("COLORFGBG"))
	if value == "" {
		return true
	}
	parts := strings.Split(value, ";")
	background := strings.TrimSpace(parts[len(parts)-1])
	switch background {
	case "7", "15":
		return false
	default:
		return true
	}
}

// colorProfile maps the detected level to a termenv profile. Applying it
// explicitly stops lipgloss from probing the terminal (OSC 11 / cursor
// queries) on first render, which otherwise blocks on terminals that do not
// answer.
func (p palette) colorProfile() termenv.Profile {
	switch p.level {
	case colorLevelTrueColor:
		return termenv.TrueColor
	case colorLevelANSI256:
		return termenv.ANSI256
	case colorLevelANSI16:
		return termenv.ANSI
	default:
		return termenv.Ascii
	}
}

func (p palette) plain() lipgloss.Style { return lipgloss.NewStyle() }

func (p palette) strong() lipgloss.Style { return p.plain().Bold(true) }

func (p palette) dim() lipgloss.Style {
	if p.noColor || p.level == colorLevelNone {
		return p.plain().Faint(true)
	}
	return p.plain().Faint(true)
}

func (p palette) accent() lipgloss.Style {
	if p.noColor || p.level == colorLevelNone {
		return p.strong()
	}
	return p.plain().Foreground(lipgloss.Color("6")).Bold(true)
}

func (p palette) success() lipgloss.Style {
	return p.statusStyle("2")
}

func (p palette) failure() lipgloss.Style {
	return p.statusStyle("1").Bold(true)
}

func (p palette) warning() lipgloss.Style {
	return p.statusStyle("3")
}

func (p palette) user() lipgloss.Style {
	if p.noColor || p.level == colorLevelNone {
		return p.plain()
	}
	return p.plain().Foreground(lipgloss.Color("4"))
}

func (p palette) tool() lipgloss.Style {
	if p.noColor || p.level == colorLevelNone {
		return p.plain()
	}
	return p.plain().Foreground(lipgloss.Color("6"))
}

func (p palette) statusStyle(ansi string) lipgloss.Style {
	if p.noColor || p.level == colorLevelNone {
		return p.plain()
	}
	return p.plain().Foreground(lipgloss.Color(ansi))
}
