package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

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
	session, err := buildSession()
	if err != nil {
		return err
	}
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
