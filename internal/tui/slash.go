package tui

import (
	"fmt"
	"strings"
)

// slashCommand is an interactive command entered with a leading slash; it is
// handled locally and never sent to the model.
type slashCommand string

const (
	slashHelp    slashCommand = "help"
	slashClear   slashCommand = "clear"
	slashCompact slashCommand = "compact"
	slashModel   slashCommand = "model"
	slashStatus  slashCommand = "status"
	slashResume  slashCommand = "resume"
	slashQuit    slashCommand = "quit"
)

// slashCommands lists the built-in commands in display order.
func slashCommands() []slashCommand {
	return []slashCommand{slashHelp, slashClear, slashCompact, slashModel, slashStatus, slashResume, slashQuit}
}

func (c slashCommand) Name() string { return string(c) }

func (c slashCommand) Description() string {
	switch c {
	case slashHelp:
		return "show available commands"
	case slashClear:
		return "clear the conversation (keeps the saved session)"
	case slashCompact:
		return "summarize the conversation to free context"
	case slashModel:
		return "show the current model"
	case slashStatus:
		return "show session status"
	case slashResume:
		return "resume a saved session (use `equus chat --resume`)"
	case slashQuit:
		return "exit Equus"
	default:
		return ""
	}
}

// findSlashCommand resolves a name (with or without a leading slash).
func findSlashCommand(name string) (slashCommand, bool) {
	name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
	if name == "exit" {
		return slashQuit, true
	}
	for _, c := range slashCommands() {
		if c.Name() == name {
			return c, true
		}
	}
	return "", false
}

// slashInvocation is a parsed command with its argument string.
type slashInvocation struct {
	Command slashCommand
	Args    string
}

// inputResult distinguishes ordinary prompt text from a slash command.
type inputResult struct {
	Text    string
	Command *slashInvocation
}

// parseInput parses an input line into either prompt text or a slash command.
func parseInput(value string) (inputResult, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "/") {
		return inputResult{Text: value}, nil
	}
	invocation, err := parseSlashInvocation(value)
	if err != nil {
		return inputResult{}, err
	}
	return inputResult{Command: &invocation}, nil
}

func parseSlashInvocation(value string) (slashInvocation, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return slashInvocation{}, fmt.Errorf("invalid command %q", value)
	}
	command, ok := findSlashCommand(fields[0])
	if !ok {
		return slashInvocation{}, fmt.Errorf("unknown command %q (try /help)", fields[0])
	}
	args := strings.TrimSpace(strings.TrimPrefix(value, fields[0]))
	return slashInvocation{Command: command, Args: args}, nil
}

// formatSlashCommands renders the command list for /help.
func formatSlashCommands() []string {
	out := make([]string, 0, len(slashCommands()))
	for _, c := range slashCommands() {
		out = append(out, fmt.Sprintf("/%-9s %s", c.Name(), c.Description()))
	}
	return out
}
