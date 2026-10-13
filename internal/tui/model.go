// Package tui implements the interactive Equus terminal interface, modeled on
// the Amadeus TUI but trimmed to Equus's capabilities: a scrolling transcript,
// a raw-mode input box, and streaming agent events.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/agent"
)

const (
	inputPrompt      = "› "
	inputPlaceholder = "Ask Equus to do anything"
	inputCharLimit   = 20000
	maxInputRows     = 5
	minWidth         = 20
)

// Model is the Bubble Tea model backing the interactive chat session.
type Model struct {
	ctx     context.Context
	cancel  context.CancelFunc
	session *agent.Session
	palette palette

	// program is a shared handle set by Run so the background turn can post
	// events back into the Bubble Tea loop. It is a pointer holder because
	// Bubble Tea copies the model by value.
	program *programRef

	width  int
	height int

	input    textarea.Model
	viewport viewport.Model

	// transcript holds the committed, already-rendered lines shown above the
	// composer. The streaming turn appends to a separate active buffer.
	transcript []string
	active     strings.Builder

	running bool
	status  string

	history    []string
	historyPos int

	version string
	model   string
	cwd     string

	ready bool
}

// Options configures a new Model.
type Options struct {
	Session *agent.Session
	Version string
	Model   string
	CWD     string
	NoColor bool
	History []string
	IsTTY   bool
}

// NewModel builds the interactive model. It returns an error when the session
// is missing.
func NewModel(ctx context.Context, options Options) (Model, error) {
	if options.Session == nil {
		return Model{}, errNoSession
	}
	ctx, cancel := context.WithCancel(ctx)
	p := detectPalette(options.NoColor)

	input := textarea.New()
	input.Placeholder = inputPlaceholder
	input.SetPromptFunc(len(inputPrompt), func(line int) string {
		if line == 0 {
			return p.accent().Render(inputPrompt)
		}
		return strings.Repeat(" ", len(inputPrompt))
	})
	input.ShowLineNumbers = false
	input.CharLimit = inputCharLimit
	input.MaxHeight = maxInputRows
	input.FocusedStyle.Prompt = p.accent()
	input.FocusedStyle.Placeholder = p.dim()
	input.BlurredStyle.Placeholder = p.dim()
	input.FocusedStyle.CursorLine = p.plain()
	input.SetWidth(80)
	input.SetHeight(1)
	input.Focus()
	_ = input.Cursor.SetMode(cursor.CursorStatic)

	viewer := viewport.New(80, 20)
	viewer.MouseWheelEnabled = true

	return Model{
		ctx:        ctx,
		cancel:     cancel,
		session:    options.Session,
		program:    &programRef{},
		palette:    p,
		input:      input,
		viewport:   viewer,
		width:      100,
		height:     30,
		status:     "idle",
		history:    append([]string(nil), options.History...),
		historyPos: -1,
		version:    options.Version,
		model:      options.Model,
		cwd:        options.CWD,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.HideCursor, textarea.Blink)
}

// programRef holds the running program so the model (copied by value) can
// still reach it from background goroutines.
type programRef struct {
	program *tea.Program
}

func (ref *programRef) send(message tea.Msg) {
	if ref == nil || ref.program == nil {
		return
	}
	ref.program.Send(message)
}
