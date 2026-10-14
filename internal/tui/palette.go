package tui

import (
	"fmt"
	"math"
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

// terminalRGB is a 24-bit color.
type terminalRGB struct{ Red, Green, Blue uint8 }

// palette holds the styles used to render the interface. Colors are derived
// from the terminal's color depth and light/dark background so the interface
// reads well on any terminal.
type palette struct {
	level      colorLevel
	dark       bool
	noColor    bool
	foreground terminalRGB
	background terminalRGB
}

// statusLineAccent names the semantic slots of the status line.
type statusLineAccent uint8

const (
	statusAccentModel statusLineAccent = iota
	statusAccentPath
	statusAccentBranch
	statusAccentUsage
	statusAccentMode
)

func detectPalette(noColor bool) palette {
	if noColor || !supportsColor() {
		return palette{
			level: colorLevelNone, noColor: true, dark: true,
			foreground: terminalRGB{220, 220, 220}, background: terminalRGB{20, 20, 20},
		}
	}
	level := detectColorLevel()
	dark := detectDarkBackground()
	foreground := terminalRGB{30, 30, 30}
	background := terminalRGB{245, 245, 245}
	if dark {
		foreground = terminalRGB{225, 225, 225}
		background = terminalRGB{18, 18, 18}
	}
	return palette{level: level, dark: dark, foreground: foreground, background: background}
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
// explicitly stops lipgloss from probing the terminal (OSC 11 background /
// cursor queries) on first render, which otherwise blocks on terminals that do
// not answer the query.
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

func (p palette) rich() bool { return !p.noColor && p.level != colorLevelNone }

func (p palette) plain() lipgloss.Style  { return lipgloss.NewStyle() }
func (p palette) strong() lipgloss.Style { return p.plain().Bold(true) }
func (p palette) bold() lipgloss.Style   { return p.strong() }

// muted / dim render secondary text.
func (p palette) muted() lipgloss.Style {
	style := p.plain().Faint(true)
	if p.level == colorLevelTrueColor || p.level == colorLevelANSI256 {
		style = style.Foreground(p.bestColor(blendRGB(p.background, p.foreground, 0.55)))
	}
	return style
}

func (p palette) dim() lipgloss.Style { return p.muted() }

// accent highlights the primary/emphasis color.
func (p palette) accent() lipgloss.Style {
	if !p.rich() {
		return p.strong()
	}
	if p.dark || p.level == colorLevelANSI16 {
		return p.plain().Foreground(lipgloss.Color("6")).Bold(true)
	}
	return p.plain().Foreground(p.bestColor(p.accentRGB())).Bold(true)
}

func (p palette) success() lipgloss.Style {
	return p.statusColor(terminalRGB{72, 187, 120}, "2", "2").Bold(true)
}

func (p palette) failure() lipgloss.Style {
	return p.statusColor(terminalRGB{235, 107, 107}, "1", "1").Bold(true)
}

func (p palette) warning() lipgloss.Style {
	return p.statusColor(terminalRGB{224, 174, 72}, "3", "3")
}

// tool styles tool names in tool-call lines.
func (p palette) tool() lipgloss.Style {
	if !p.rich() {
		return p.plain()
	}
	return p.plain().Foreground(lipgloss.Color("6"))
}

// user styles a submitted prompt; on truecolor it carries a subtle background.
func (p palette) user() lipgloss.Style {
	if !p.rich() || p.level != colorLevelTrueColor {
		return p.plain()
	}
	alpha := 0.04
	if p.dark {
		alpha = 0.12
	}
	return p.plain().Background(p.bestColor(blendRGB(p.background, p.foreground, alpha)))
}

// inputBackground fills the composer with a tint so the input box stands out.
// It returns a solid background at every color level (truecolor/256 blend the
// terminal background; ANSI 16 uses grey) so the fill is always visible.
func (p palette) inputBackground() lipgloss.Style {
	if !p.rich() {
		return p.plain()
	}
	switch p.level {
	case colorLevelANSI16:
		if p.dark {
			return p.plain().Background(lipgloss.Color("8"))
		}
		return p.plain().Background(lipgloss.Color("7"))
	default:
		alpha := 0.04
		if p.dark {
			alpha = 0.12
		}
		return p.plain().Background(p.bestColor(blendRGB(p.background, p.foreground, alpha)))
	}
}

// separator / border derive faint rules from the background/foreground mix.
func (p palette) separator() lipgloss.Style {
	if p.level == colorLevelTrueColor || p.level == colorLevelANSI256 {
		return p.plain().Foreground(p.bestColor(blendRGB(p.background, p.foreground, 0.20)))
	}
	return p.muted()
}

func (p palette) border() lipgloss.Style {
	if p.level == colorLevelTrueColor || p.level == colorLevelANSI256 {
		return p.plain().Foreground(p.bestColor(blendRGB(p.background, p.foreground, 0.32)))
	}
	return p.muted()
}

// statusLineStyle colors a status line segment by its semantic slot.
func (p palette) statusLineStyle(accent statusLineAccent) lipgloss.Style {
	if !p.rich() {
		return p.muted()
	}
	ansi, rgb := statusLineFallback(accent)
	if p.level == colorLevelANSI16 {
		return p.plain().Foreground(lipgloss.Color(ansi))
	}
	if themed, ok := statusLineThemeRGB(accent, p.dark); ok {
		rgb = themed
	}
	return p.plain().Foreground(p.bestColor(softenStatusLineRGB(rgb)))
}

func statusLineThemeRGB(accent statusLineAccent, dark bool) (terminalRGB, bool) {
	// Built-in approximations of a catppuccin-like theme (chroma-free).
	if dark {
		switch accent {
		case statusAccentPath:
			return terminalRGB{166, 227, 161}, true // green
		case statusAccentBranch:
			return terminalRGB{203, 166, 247}, true // mauve
		case statusAccentUsage:
			return terminalRGB{148, 226, 213}, true // teal
		case statusAccentMode:
			return terminalRGB{245, 194, 231}, true // pink
		default:
			return terminalRGB{137, 220, 235}, true // sky
		}
	}
	switch accent {
	case statusAccentPath:
		return terminalRGB{64, 160, 43}, true
	case statusAccentBranch:
		return terminalRGB{136, 57, 239}, true
	case statusAccentUsage:
		return terminalRGB{23, 146, 153}, true
	case statusAccentMode:
		return terminalRGB{234, 118, 203}, true
	default:
		return terminalRGB{4, 165, 229}, true
	}
}

func statusLineFallback(accent statusLineAccent) (string, terminalRGB) {
	switch accent {
	case statusAccentPath, statusAccentUsage:
		return "2", terminalRGB{0, 205, 0}
	case statusAccentBranch, statusAccentMode:
		return "5", terminalRGB{205, 0, 205}
	default:
		return "6", terminalRGB{0, 205, 205}
	}
}

func softenStatusLineRGB(color terminalRGB) terminalRGB {
	luma := (77*uint16(color.Red) + 150*uint16(color.Green) + 29*uint16(color.Blue)) / 256
	soften := func(channel uint8) uint8 {
		return uint8((uint16(channel)*85 + luma*15 + 50) / 100)
	}
	return terminalRGB{Red: soften(color.Red), Green: soften(color.Green), Blue: soften(color.Blue)}
}

func (p palette) accentRGB() terminalRGB {
	if p.dark {
		return terminalRGB{0, 205, 205}
	}
	return terminalRGB{0, 95, 135}
}

func (p palette) statusColor(rgb terminalRGB, darkFallback, lightFallback string) lipgloss.Style {
	if !p.rich() {
		return p.plain()
	}
	if p.level == colorLevelANSI16 {
		if p.dark {
			return p.plain().Foreground(lipgloss.Color(darkFallback))
		}
		return p.plain().Foreground(lipgloss.Color(lightFallback))
	}
	return p.plain().Foreground(p.bestColor(rgb))
}

func (p palette) bestColor(rgb terminalRGB) lipgloss.TerminalColor {
	if !p.rich() || p.level == colorLevelANSI16 {
		return lipgloss.NoColor{}
	}
	if p.level == colorLevelANSI256 {
		return lipgloss.Color(fmt.Sprintf("%d", nearestANSI256(rgb)))
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", rgb.Red, rgb.Green, rgb.Blue))
}

func nearestANSI256(target terminalRGB) uint8 {
	bestIndex := uint8(16)
	bestDistance := math.MaxFloat64
	for index := 16; index <= 255; index++ {
		candidate := ansi256RGB(uint8(index))
		red := float64(int(target.Red) - int(candidate.Red))
		green := float64(int(target.Green) - int(candidate.Green))
		blue := float64(int(target.Blue) - int(candidate.Blue))
		distance := red*red*0.299 + green*green*0.587 + blue*blue*0.114
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = uint8(index)
		}
	}
	return bestIndex
}

func ansi256RGB(index uint8) terminalRGB {
	if index >= 232 {
		value := uint8(8 + 10*(int(index)-232))
		return terminalRGB{value, value, value}
	}
	cube := int(index) - 16
	levels := [...]uint8{0, 95, 135, 175, 215, 255}
	return terminalRGB{levels[cube/36], levels[(cube/6)%6], levels[cube%6]}
}

func blendRGB(background, foreground terminalRGB, alpha float64) terminalRGB {
	alpha = math.Max(0, math.Min(1, alpha))
	blend := func(base, overlay uint8) uint8 {
		return uint8(float64(base)*(1-alpha) + float64(overlay)*alpha)
	}
	return terminalRGB{blend(background.Red, foreground.Red), blend(background.Green, foreground.Green), blend(background.Blue, foreground.Blue)}
}
