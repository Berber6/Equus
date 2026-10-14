package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Shell execution limits.
const (
	defaultShellTimeout = 30 * time.Second
	maxShellTimeout     = 300 * time.Second
	maxShellOutputBytes = 16 << 10
)

// ShellTool runs a shell command in the working directory. It always requires
// approval (a command can do arbitrary work).
type ShellTool struct{ Root string }

func (tool ShellTool) Name() string { return "shell" }

func (tool ShellTool) Description() string {
	return "Run a shell command in the working directory and return its combined stdout/stderr and exit code. " +
		"Use it to build, test and inspect the project. Prefer the dedicated file tools for reading and editing."
}

func (tool ShellTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The shell command to run."},
			"timeout_seconds": {"type": "integer", "description": "Optional timeout in seconds (default 30, max 300)."}
		},
		"required": ["command"]
	}`)
}

// ApprovalKind reports the policy category for this tool.
func (tool ShellTool) ApprovalKind() approvalKind { return approvalKindShell }

// ApprovalSummary returns the command to display, always requiring approval.
func (tool ShellTool) ApprovalSummary(arguments []byte) (string, bool) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", false
	}
	return strings.TrimSpace(input.Command), strings.TrimSpace(input.Command) != ""
}

func (tool ShellTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("shell: decode arguments: %w", err)
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return "", fmt.Errorf("shell: command is empty")
	}
	timeout := defaultShellTimeout
	if input.TimeoutSeconds > 0 {
		timeout = time.Duration(input.TimeoutSeconds) * time.Second
		if timeout > maxShellTimeout {
			timeout = maxShellTimeout
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell, args := shellCommand(command)
	cmd := exec.CommandContext(runCtx, shell, args...)
	cmd.Dir = tool.Root
	stdout, err := cmd.CombinedOutput()

	output := truncateOutput(string(stdout), maxShellOutputBytes)
	status := 0
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		return fmt.Sprintf("command timed out after %s\n%s", timeout, output), nil
	case err != nil:
		if exitErr, ok := err.(*exec.ExitError); ok {
			status = exitErr.ExitCode()
		} else {
			return fmt.Sprintf("failed to run command: %v\n%s", err, output), nil
		}
	}
	return fmt.Sprintf("exit code: %d\n%s", status, output), nil
}

// shellCommand picks the shell and argv for the platform.
func shellCommand(command string) (string, []string) {
	return "/bin/bash", []string{"-lc", command}
}

// truncateOutput caps the byte length, keeping the head and noting the drop.
func truncateOutput(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + fmt.Sprintf("\n... (output truncated, %d bytes total)", len(text))
}
