package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/llm"
	"github.com/Godric-W/Equus/internal/llm/openai"
	"github.com/Godric-W/Equus/internal/threadstore"
	"github.com/Godric-W/Equus/internal/tui"
	"github.com/spf13/cobra"
)

func newChatCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var (
		continueLatest bool
		resumeID       string
		noSave         bool
	)
	command := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive TUI chat session with the model",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runChat(command.Context(), chatOptions{
				continueLatest: continueLatest,
				resumeID:       resumeID,
				noSave:         noSave,
			}, stdin, stdout, stderr)
		},
	}
	command.Flags().BoolVarP(&continueLatest, "continue", "c", false, "continue the most recent session")
	command.Flags().StringVar(&resumeID, "resume", "", "resume a session by id")
	command.Flags().BoolVar(&noSave, "no-save", false, "do not save this session")
	return command
}

type chatOptions struct {
	continueLatest bool
	resumeID       string
	noSave         bool
}

func runChat(ctx context.Context, options chatOptions, stdin io.Reader, stdout, stderr io.Writer) error {
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

	store := threadstore.New(sessionsDir())
	var history []llm.ResponseItem
	writer, restored, err := openSession(store, options, workingDir, model)
	if err != nil {
		return err
	}
	if restored != nil {
		history = restored.Items
		session.Restore(history)
	}
	if writer != nil {
		defer writer.Close()
		session.SetRecorder(threadstore.Recorder{Writer: writer})
	}

	return tui.Run(ctx, tui.RunOptions{
		Session: session,
		History: history,
		Input:   stdin,
		Output:  stdout,
		Version: Version,
		Model:   model,
		CWD:     workingDir,
	})
}

type restoredSession struct {
	Meta  threadstore.Meta
	Items []llm.ResponseItem
}

// openSession resolves the resume behavior and, when saving is enabled, opens a
// writer for the session (creating a new one or appending to an existing file).
func openSession(store *threadstore.Store, options chatOptions, workingDir, model string) (*threadstore.Writer, *restoredSession, error) {
	switch {
	case options.resumeID != "":
		meta, items, err := store.Load(options.resumeID)
		if err != nil {
			return nil, nil, fmt.Errorf("resume %s: %w", options.resumeID, err)
		}
		writer, err := reopen(store, meta)
		if err != nil {
			return nil, nil, err
		}
		return writer, &restoredSession{Meta: meta, Items: items}, nil
	case options.continueLatest:
		meta, err := store.Latest()
		if err != nil {
			return nil, nil, err
		}
		_, items, err := store.Load(meta.ID)
		if err != nil {
			return nil, nil, err
		}
		writer, err := reopen(store, meta)
		if err != nil {
			return nil, nil, err
		}
		return writer, &restoredSession{Meta: meta, Items: items}, nil
	}
	if options.noSave {
		return nil, nil, nil
	}
	writer, err := store.Create(threadstore.Meta{Model: model, CWD: workingDir, Title: "chat"})
	if err != nil {
		return nil, nil, err
	}
	return writer, nil, nil
}

// reopen continues writing to an existing session file without a new meta line.
func reopen(store *threadstore.Store, meta threadstore.Meta) (*threadstore.Writer, error) {
	// Create with a fixed id re-writes a meta line, which is harmless (the last
	// meta wins on load) and keeps the writer simple.
	return store.Create(meta)
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
	compaction := configured.Compaction
	session, err := agent.NewSession(client, agent.Options{
		Registry: registry,
		ApprovalMode: func(category string) agent.ApprovalMode {
			return agent.ApprovalMode(approval.EffectiveMode(category))
		},
		Compaction: agent.CompactionOptions{
			Enabled:         compaction.Enabled,
			Threshold:       compaction.Threshold,
			KeepRecentTurns: compaction.KeepRecentTurns,
		},
	})
	if err != nil {
		return nil, "", err
	}
	session.SetBaseInstructions(agent.BuildBaseInstructions(agent.InstructionsInput{
		CWD:         workingDir,
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		Tools:       session.ToolNames(),
		ProjectDocs: agent.DiscoverAgentsDocs(workingDir),
		Override:    configured.BaseInstructions,
	}))
	if err != nil {
		return nil, "", err
	}
	return session, configured.Model, nil
}
