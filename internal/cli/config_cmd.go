package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Godric-W/Equus/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCommand(stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Inspect Equus configuration",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(newConfigCheckCommand(stdout, stderr))
	command.AddCommand(newConfigShowCommand(stdout))
	return command
}

func newConfigCheckCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			home := equusHome()
			loader := config.NewLoader(home)
			configured, err := loader.Load()
			if err != nil {
				return err
			}
			if err := config.Validate(configured); err != nil {
				return fmt.Errorf("%s: %w", loader.ConfigPath(), err)
			}
			fmt.Fprintf(stdout, "configuration OK (%s)\n", loader.ConfigPath())
			return nil
		},
	}
}

func newConfigShowCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the effective redacted configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			home := equusHome()
			loader := config.NewLoader(home)
			configured, err := loader.Load()
			if err != nil {
				return err
			}
			if err := config.Validate(configured); err != nil {
				return fmt.Errorf("%s: %w", loader.ConfigPath(), err)
			}
			encoder := yaml.NewEncoder(stdout)
			encoder.SetIndent(2)
			defer encoder.Close()
			return encoder.Encode(config.Redact(configured))
		},
	}
}

// EnsureHome creates the Equus home directory with a starter config when it
// does not exist yet. It is invoked by chat/exec before the first run.
func EnsureHome() error {
	home := equusHome()
	if home == "" {
		return nil
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	path := filepath.Join(home, config.ConfigFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	starter := fmt.Sprintf(`# Equus starter configuration written on first run.
# Keep credentials out of this file when possible and inject the key via
# EQUUS_API_KEY instead.

model: GLM-5.3
model_provider: openai

model_providers:
  openai:
    wire_api: chat_completions
    api_key: ${%s}
    base_url: http://127.0.0.1:3456/v1
    timeout: 120s
`, config.EnvAPIKey)
	return os.WriteFile(path, []byte(starter), 0o644)
}
