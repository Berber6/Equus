package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/spf13/cobra"
)

func newExecCommand(stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "exec PROMPT",
		Short: "Run one prompt through the agent and print the answer",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			prompt := strings.Join(arguments, " ")
			return runExec(command.Context(), prompt, stdout, stderr)
		},
	}
	return command
}

func runExec(ctx context.Context, prompt string, stdout, stderr io.Writer) error {
	configured, err := loadConfig()
	if err != nil {
		return err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	session, _, err := buildSession(configured, workingDir)
	if err != nil {
		return err
	}
	// Non-interactive: approvals have nowhere to go, so deny (the session
	// reports the denial to the model, which can adapt).
	session.SetApprover(func(context.Context, agent.ApprovalRequest) (agent.ApprovalDecision, error) {
		return agent.ApprovalDenied, nil
	})
	answer, err := session.Ask(ctx, prompt, chatEventSink(stdout))
	if err != nil && answer == "" {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	fmt.Fprintln(stdout, answer)
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(stderr, "warning: turn ended early: %v\n", err)
	}
	return nil
}

// chatEventSink renders streaming session events to the terminal for the
// non-interactive exec command.
func chatEventSink(stdout io.Writer) func(agent.Event) {
	var reasoningShown bool
	return func(event agent.Event) {
		switch event.Kind {
		case agent.EventReasoningDelta:
			if !reasoningShown {
				fmt.Fprintln(stdout, "\x1b[2m[thinking]\x1b[0m")
				reasoningShown = true
			}
			fmt.Fprint(stdout, event.Text)
		case agent.EventToolCall:
			fmt.Fprintf(stdout, "\x1b[36m[tool] %s(%s)\x1b[0m\n", event.Tool, event.Text)
		case agent.EventToolResult:
			fmt.Fprintf(stdout, "\x1b[36m[tool-result] %s: %s\x1b[0m\n", event.Tool, event.Text)
		case agent.EventUsage:
			if event.Usage != nil {
				fmt.Fprintf(stdout, "\x1b[2m[tokens] in=%d out=%d\x1b[0m\n", event.Usage.InputTokens, event.Usage.OutputTokens)
			}
		}
	}
}
