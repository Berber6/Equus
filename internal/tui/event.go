package tui

import (
	"errors"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/llm"
)

// errNoSession is returned when the model is built without an agent session.
var errNoSession = errors.New("tui session is nil")

// eventMsg wraps one agent.Event delivered from the running turn.
type eventMsg struct{ event agent.Event }

// turnDoneMsg reports that the current turn finished (or failed).
type turnDoneMsg struct {
	answer string
	err    error
}

// approvalMsg asks the UI to confirm an operation; it carries the reply
// channel the background turn is blocked on.
type approvalMsg struct {
	summary string
	reply   chan agent.ApprovalDecision
}

// sessionLoadedMsg carries a selected session's items back to the loop.
type sessionLoadedMsg struct {
	id    string
	items []llm.ResponseItem
	err   error
}
