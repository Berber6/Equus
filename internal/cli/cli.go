// Package cli wires the Equus command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Godric-W/Equus/internal/config"
	"github.com/spf13/cobra"
)

// Run executes the CLI with the given arguments and streams.
func Run(ctx context.Context, arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newRootCommand(stdin, stdout, stderr)
	root.SetArgs(arguments)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "equus:", err)
		}
		return 1
	}
	return 0
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "equus",
		Short:         "Equus - a minimal terminal coding agent",
		Long:          "Equus is a minimal Go coding agent that talks to an OpenAI-compatible LLM endpoint and can read, list and write files via tools. Running `equus` with no subcommand starts the interactive TUI.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	configureChatFlags(root, stdin, stdout, stderr)
	root.AddCommand(newExecCommand(stdout, stderr))
	root.AddCommand(newSessionsCommand(stdout, stderr))
	root.AddCommand(newVersionCommand(stdout))
	root.AddCommand(newConfigCommand(stdout, stderr))
	return root
}

// sessionsDir is where persisted sessions live.
func sessionsDir() string {
	home := equusHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "sessions")
}

// equusHome resolves the configuration home directory: EQUUS_HOME when set,
// otherwise ~/.equus.
func equusHome() string {
	if home := strings.TrimSpace(os.Getenv(config.EnvHome)); home != "" {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(userHome, ".equus")
}

// loadConfig loads and validates the effective configuration.
func loadConfig() (config.Config, error) {
	loader := config.NewLoader(equusHome())
	configured, err := loader.Load()
	if err != nil {
		return config.Config{}, err
	}
	if err := config.Validate(configured); err != nil {
		return config.Config{}, fmt.Errorf("%s: %w", loader.ConfigPath(), err)
	}
	return configured, nil
}
