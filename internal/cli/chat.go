package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/llm/openai"
	"github.com/spf13/cobra"
)

func newChatCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive chat session with the model",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runChat(command.Context(), stdin, stdout, stderr)
		},
	}
	return command
}

func runChat(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	session, err := buildSession()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(stdin)
	fmt.Fprintln(stdout, "Equus interactive chat. Type your message; \\quit or Ctrl-D to exit.")
	for {
		fmt.Fprint(stdout, "\n> ")
		line, ok, err := agent.ReadLine(reader)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stdout)
			return nil
		}
		prompt := strings.TrimSpace(line)
		switch prompt {
		case "":
			continue
		case "\\quit", "\\q", "exit", "quit":
			return nil
		}

		fmt.Fprintln(stdout, "")
		answer, err := session.Ask(ctx, prompt, chatEventSink(stdout))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(stderr, "error: %v\n", err)
			continue
		}
		fmt.Fprintf(stdout, "\n[equus] %s\n", answer)
	}
}

// chatEventSink renders streaming session events to the terminal.
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

// buildSession composes the model client and agent session from config.
func buildSession() (*agent.Session, error) {
	configured, err := loadConfig()
	if err != nil {
		return nil, err
	}
	provider, _, err := configured.ResolveProvider()
	if err != nil {
		return nil, err
	}
	if provider.WireAPI != config.WireAPIChatCompletions {
		return nil, fmt.Errorf("unsupported wire_api %q (only %q is implemented)", provider.WireAPI, config.WireAPIChatCompletions)
	}
	client, err := openai.NewClient(openai.Options{
		ProviderName: configured.ModelProvider,
		APIKey:       provider.APIKey,
		BaseURL:      provider.BaseURL,
		Model:        configured.Model,
		Timeout:      provider.Timeout,
	})
	if err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	registry, err := agent.NewDefaultRegistry(workingDir)
	if err != nil {
		return nil, err
	}
	return agent.NewSession(client, agent.Options{Registry: registry})
}
