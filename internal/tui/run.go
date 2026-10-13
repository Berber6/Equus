package tui

import (
	"context"
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Godric-W/Equus/internal/agent"
)

// RunOptions configures the interactive session.
type RunOptions struct {
	Session *agent.Session
	Input   io.Reader
	Output  io.Writer
	Version string
	Model   string
	CWD     string
	NoColor bool
}

// ErrNotTerminal is returned when the input stream is not an interactive
// terminal, which the TUI requires.
var ErrNotTerminal = errors.New("interactive Equus requires a terminal")

// Run starts the interactive TUI and blocks until the user quits.
func Run(ctx context.Context, options RunOptions) error {
	if options.Session == nil {
		return errNoSession
	}
	input := options.Input
	if input == nil {
		input = os.Stdin
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}
	if !IsTerminal(input) {
		return ErrNotTerminal
	}
	// Fix the color profile up front so lipgloss does not probe the terminal
	// (OSC 11 background / cursor queries), which blocks on terminals that do
	// not answer the query.
	lipgloss.SetColorProfile(detectPalette(options.NoColor).colorProfile())
	model, err := NewModel(ctx, Options{
		Session: options.Session,
		Version: options.Version,
		Model:   options.Model,
		CWD:     options.CWD,
		NoColor: options.NoColor,
		IsTTY:   true,
	})
	if err != nil {
		return err
	}
	// Inline mode (no alternate screen): the transcript is written to the
	// terminal's normal scrollback via tea.Println, so exiting leaves the
	// conversation visible and the terminal's own scrollback and mouse wheel
	// work as usual. Only the composer is a managed bottom region.
	program := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	model.program.program = program
	_, err = program.Run()
	return err
}
