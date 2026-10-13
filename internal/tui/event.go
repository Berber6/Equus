package tui

import (
	"errors"

	"github.com/Godric-W/Equus/internal/agent"
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
