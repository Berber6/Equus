// Package tui implements the interactive Equus terminal interface as a
// full-screen (alternate-screen) Bubble Tea program: a self-drawn scrolling
// transcript, a raw-mode input box, and a status line. The alternate screen
// is used because interleaving tea.Println with a growing view desyncs
// Bubble Tea's inline cursor arithmetic.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/llm"
	"github.com/Godric-W/Equus/internal/threadstore"
)

const (
	inputPrompt      = "› "
	inputPlaceholder = "Ask Equus to do anything"
	inputCharLimit   = 20000
	// maxInputRows caps the textarea's content rows; beyond it the textarea
	// scrolls internally. The composer panel is always maxInputRows+2 at most
	// (one padding row above and below the content).
	maxInputRows = 6
	minWidth     = 20
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

	input textarea.Model

	// active accumulates the in-flight assistant stream (a plain string so the
	// value-type model stays safe to copy); it is flushed into transcript at
	// tool boundaries and turn end.
	active string

	running bool
	status  string

	// transcript holds the committed, wrapped conversation lines drawn in the
	// scrolling region above the composer.
	transcript []string

	// scroll is how many lines above the tail the transcript is scrolled
	// (0 = pinned to the bottom).
	scroll int

	// history / historyPos drive prompt recall with the up/down keys.
	history    []string
	historyPos int

	// pending holds the approval request awaiting a keypress, if any.
	pending *approvalPrompt

	// picker is the session-selection overlay, when open.
	picker *sessionPicker
	// deleteConfirm, when true, awaits a y/n for /delete.
	deleteConfirm bool
	// popup is the slash-command completion overlay.
	popup slashPopup
	// loadSessionFn loads a session's items by id (set by Run).
	loadSessionFn func(id string) ([]llm.ResponseItem, error)
	// session ops (set by Run).
	sessionID string
	renameFn  func(id, title string) error
	deleteFn  func(id string) error

	// markdown / syntaxHighlight control assistant output rendering.
	markdown        bool
	syntaxHighlight bool

	// ctxUsed / ctxWindow track the last request's prompt size vs the model's
	// context window, for the status line percentage.
	ctxUsed   int64
	ctxWindow int64

	version string
	model   string
	cwd     string
	branch  string
}

// approvalPrompt is an in-flight approval request shown above the composer.
type approvalPrompt struct {
	summary string
	reply   chan agent.ApprovalDecision
}

// Options configures a new Model.
type Options struct {
	Session         *agent.Session
	Version         string
	Model           string
	CWD             string
	NoColor         bool
	History         []string
	IsTTY           bool
	Markdown        bool
	SyntaxHighlight bool
	// Sessions, when non-nil, opens the session picker with these entries.
	Sessions []threadstore.Meta
	// LoadSession loads a session's items by id.
	LoadSession func(id string) ([]llm.ResponseItem, error)
	// SessionID is the current session id (for rename/delete).
	SessionID string
	RenameFn  func(id, title string) error
	DeleteFn  func(id string) error
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
	input.SetPromptFunc(inputPromptWidth(), func(line int) string {
		if line == 0 {
			return p.accent().Render(inputPrompt)
		}
		return strings.Repeat(" ", inputPromptWidth())
	})
	input.ShowLineNumbers = false
	input.CharLimit = inputCharLimit
	input.MaxHeight = maxInputRows
	// Carry the input background on the textarea's own styles so it fills every
	// line (including padding), rather than wrapping the rendered block — the
	// textarea's internal SGR resets would otherwise clear a block-level fill.
	bg := p.inputBackground()
	input.FocusedStyle.Base = bg
	input.FocusedStyle.Text = bg
	input.FocusedStyle.CursorLine = bg
	input.FocusedStyle.EndOfBuffer = bg
	input.FocusedStyle.Prompt = p.accent()
	input.FocusedStyle.Placeholder = p.dim()
	input.BlurredStyle.Base = bg
	input.BlurredStyle.Text = bg
	input.BlurredStyle.CursorLine = bg
	input.BlurredStyle.EndOfBuffer = bg
	input.BlurredStyle.Prompt = p.dim()
	input.BlurredStyle.Placeholder = p.dim()
	input.SetWidth(80)
	input.SetHeight(1)
	input.Focus()
	_ = input.Cursor.SetMode(cursor.CursorStatic)

	return Model{
		ctx:             ctx,
		cancel:          cancel,
		session:         options.Session,
		program:         &programRef{},
		palette:         p,
		input:           input,
		width:           100,
		height:          30,
		status:          "idle",
		history:         append([]string(nil), options.History...),
		historyPos:      -1,
		version:         options.Version,
		model:           options.Model,
		cwd:             options.CWD,
		markdown:        options.Markdown,
		syntaxHighlight: options.SyntaxHighlight,
		picker:          newPicker(options.Sessions),
		loadSessionFn:   options.LoadSession,
		sessionID:       options.SessionID,
		renameFn:        options.RenameFn,
		deleteFn:        options.DeleteFn,
	}, nil
}

// newPicker builds a picker when sessions were provided, else nil.
func newPicker(sessions []threadstore.Meta) *sessionPicker {
	if sessions == nil {
		return nil
	}
	return &sessionPicker{items: sessions}
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
