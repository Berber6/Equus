package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/llm/openai"
	"github.com/Godric-W/Equus/internal/tui"
	"github.com/spf13/cobra"
)

func newChatCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive TUI chat session with the model",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runChat(command.Context(), stdin, stdout, stderr)
		},
	}
	return command
}

func runChat(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	configured, err := loadConfig()
	if err != nil {
		return err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	// The TUI installs its own interactive approver once its program runs.
	session, model, err := buildSession(configured, workingDir)
	if err != nil {
		return err
	}
	return tui.Run(ctx, tui.RunOptions{
		Session: session,
		Input:   stdin,
		Output:  stdout,
		Version: Version,
		Model:   model,
		CWD:     workingDir,
	})
}

// buildSession composes the model client and agent session from config,
// returning the session plus the model name.
func buildSession(configured config.Config, workingDir string) (*agent.Session, string, error) {
	provider, _, err := configured.ResolveProvider()
	if err != nil {
		return nil, "", err
	}
	if provider.WireAPI != config.WireAPIChatCompletions {
		return nil, "", fmt.Errorf("unsupported wire_api %q (only %q is implemented)", provider.WireAPI, config.WireAPIChatCompletions)
	}
	client, err := openai.NewClient(openai.Options{
		ProviderName: configured.ModelProvider,
		APIKey:       provider.APIKey,
		BaseURL:      provider.BaseURL,
		Model:        configured.Model,
		Timeout:      provider.Timeout,
	})
	if err != nil {
		return nil, "", err
	}
	registry, err := agent.NewDefaultRegistry(workingDir)
	if err != nil {
		return nil, "", err
	}
	approval := configured.Approval
	session, err := agent.NewSession(client, agent.Options{
		Registry: registry,
		ApprovalMode: func(category string) agent.ApprovalMode {
			return agent.ApprovalMode(approval.EffectiveMode(category))
		},
	})
	if err != nil {
		return nil, "", err
	}
	return session, configured.Model, nil
}
