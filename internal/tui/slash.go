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
	slashRename  slashCommand = "rename"
	slashDelete  slashCommand = "delete"
	slashQuit    slashCommand = "quit"
)

// slashCommands lists the built-in commands in display order.
func slashCommands() []slashCommand {
	return []slashCommand{slashHelp, slashClear, slashCompact, slashModel, slashStatus, slashResume, slashRename, slashDelete, slashQuit}
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
		return "resume a saved session (use `equus --resume`)"
	case slashRename:
		return "rename the current session (/rename <title>)"
	case slashDelete:
		return "delete the current session (asks to confirm)"
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

// filterSlashCommands returns commands whose name starts with the typed prefix.
// It activates only while the input looks like "/name" (a leading slash with no
// whitespace yet). An empty result means the popup should be hidden.
func filterSlashCommands(input string) []slashCommand {
	query, ok := slashFilterValue(input)
	if !ok {
		return nil
	}
	var matches []slashCommand
	for _, c := range slashCommands() {
		if strings.HasPrefix(c.Name(), query) {
			matches = append(matches, c)
		}
	}
	return matches
}

// slashFilterValue extracts the command prefix from input. It returns ok=false
// when the input is not a bare command name (no leading slash, or a space was
// typed).
func slashFilterValue(input string) (string, bool) {
	input = strings.TrimLeft(input, " \t\r\n")
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	trimmed := strings.TrimPrefix(input, "/")
	// Any whitespace (including a trailing space after the command) means the
	// user has moved on to arguments; the popup should stand down.
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return "", false
	}
	return strings.ToLower(trimmed), true
}

// completeSlashCommand returns the completed input for a command, with a
// trailing space so arguments can follow.
func completeSlashCommand(c slashCommand) string {
	return "/" + c.Name() + " "
}

// formatSlashCommands renders the command list for /help.
func formatSlashCommands() []string {
	out := make([]string, 0, len(slashCommands()))
	for _, c := range slashCommands() {
		out = append(out, fmt.Sprintf("/%-9s %s", c.Name(), c.Description()))
	}
	return out
}
