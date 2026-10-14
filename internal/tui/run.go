package tui

import (
	"context"
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/llm"
)

// RunOptions configures the interactive session.
type RunOptions struct {
	Session *agent.Session
	// History pre-fills the transcript (when resuming a session).
	History []llm.ResponseItem
	Input   io.Reader
	Output  io.Writer
	Version string
	Model   string
	CWD     string
	NoColor bool
	// Markdown controls assistant markdown rendering; SyntaxHighlight enables
	// chroma highlighting of fenced code blocks.
	Markdown        bool
	SyntaxHighlight bool
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
		Session:         options.Session,
		Version:         options.Version,
		Model:           options.Model,
		CWD:             options.CWD,
		NoColor:         options.NoColor,
		IsTTY:           true,
		Markdown:        options.Markdown,
		SyntaxHighlight: options.SyntaxHighlight,
	})
	if err != nil {
		return err
	}
	model.ctxWindow = options.Session.ContextWindow()
	if used, _ := options.Session.ContextUsage(); used > 0 {
		model.ctxUsed = used
	}
	model.appendTranscript(model.logoLines()...)
	model.appendTranscript("")
	if len(options.History) > 0 {
		model.appendTranscript(renderHistory(model, options.History)...)
	}
	// Full-screen (alternate screen) mode: the whole interface is drawn by
	// View() — a self-scrolling transcript, the composer, and a status line.
	// Alternate screen is required because interleaving tea.Println with a
	// growing inline view desyncs Bubble Tea's cursor arithmetic.
	program := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	model.program.program = program
	if options.Session != nil {
		options.Session.SetApprover(func(inner context.Context, request agent.ApprovalRequest) (agent.ApprovalDecision, error) {
			reply := make(chan agent.ApprovalDecision, 1)
			program.Send(approvalMsg{summary: request.Summary, reply: reply})
			select {
			case decision := <-reply:
				return decision, nil
			case <-inner.Done():
				return agent.ApprovalDenied, inner.Err()
			case <-ctx.Done():
				return agent.ApprovalDenied, ctx.Err()
			}
		})
	}
	_, err = program.Run()
	return err
}
