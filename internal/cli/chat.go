package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/Godric-W/Equus/internal/agent"
	"github.com/Godric-W/Equus/internal/approvals"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/llm"
	"github.com/Godric-W/Equus/internal/llm/anthropic"
	"github.com/Godric-W/Equus/internal/llm/openai"
	"github.com/Godric-W/Equus/internal/skill"
	"github.com/Godric-W/Equus/internal/threadstore"
	"github.com/Godric-W/Equus/internal/tui"
	"github.com/spf13/cobra"
)

// resumePicker is the sentinel value of --resume when no id is given, meaning
// "open the interactive session picker".
const resumePicker = "\x00pick"

// configureChatFlags makes the command itself the interactive chat entry point:
// running `equus` with no subcommand starts the TUI.
func configureChatFlags(command *cobra.Command, stdin io.Reader, stdout, stderr io.Writer) {
	var (
		continueLatest bool
		resumeID       string
		noSave         bool
	)
	command.RunE = func(command *cobra.Command, _ []string) error {
		resume := resumeID
		if command.Flags().Changed("resume") && resumeID == "" {
			resume = resumePicker
		}
		return runChat(command.Context(), chatOptions{
			continueLatest: continueLatest,
			resumeID:       resume,
			noSave:         noSave,
		}, stdin, stdout, stderr)
	}
	command.Flags().BoolVarP(&continueLatest, "continue", "c", false, "continue the most recent session")
	command.Flags().StringVar(&resumeID, "resume", "", "resume a session by id; omit the id to pick interactively")
	command.Flags().Lookup("resume").NoOptDefVal = resumePicker
	command.Flags().BoolVar(&noSave, "no-save", false, "do not save this session")
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

	// Connect configured MCP servers and register their tools. A server that
	// fails to connect is skipped (a warning goes to stderr) rather than
	// aborting the CLI.
	mcpSessions := connectMCPServers(ctx, configured, session, stderr)
	defer closeMCPServers(mcpSessions)

	store := threadstore.New(sessionsDir())
	var history []llm.ResponseItem
	var pickerSessions []threadstore.Meta
	var sessionID string

	// The picker needs no writer until a session is chosen.
	if options.resumeID == resumePicker {
		metas, listErr := store.List()
		if listErr != nil {
			return listErr
		}
		pickerSessions = metas
		if pickerSessions == nil {
			pickerSessions = []threadstore.Meta{}
		}
	} else {
		var writer *threadstore.Writer
		var restored *restoredSession
		writer, restored, err = openSession(store, options, workingDir, model)
		if err != nil {
			return err
		}
		if restored != nil {
			history = restored.Items
			sessionID = restored.Meta.ID
			session.Restore(history)
		}
		if writer != nil {
			defer writer.Close()
			session.SetRecorder(threadstore.Recorder{Writer: writer})
			sessionID = writer.ID()
		}
	}

	return tui.Run(ctx, tui.RunOptions{
		Session:         session,
		History:         history,
		Input:           stdin,
		Output:          stdout,
		Version:         Version,
		Model:           model,
		CWD:             workingDir,
		Markdown:        configured.Markdown.Enabled,
		SyntaxHighlight: configured.Markdown.SyntaxHighlight,
		Sessions:        pickerSessions,
		SessionID:       sessionID,
		LoadSession: func(id string) ([]llm.ResponseItem, error) {
			_, items, loadErr := store.Load(id)
			return items, loadErr
		},
		RenameFn: store.SetTitle,
		DeleteFn: store.Delete,
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

// newModelClient builds the transport client selected by wire_api.
func newModelClient(configured config.Config, provider config.ModelProviderInfo) (llm.Client, error) {
	switch provider.WireAPI {
	case config.WireAPIChatCompletions, config.WireAPIResponses, "":
		return openai.NewClient(openai.Options{
			ProviderName: configured.ModelProvider,
			APIKey:       provider.APIKey,
			BaseURL:      provider.BaseURL,
			Model:        configured.Model,
			Timeout:      provider.Timeout,
			WireAPI:      string(provider.WireAPI),
		})
	case config.WireAPIAnthropicMessages:
		return anthropic.NewClient(anthropic.Options{
			ProviderName: configured.ModelProvider,
			APIKey:       provider.APIKey,
			BaseURL:      provider.BaseURL,
			Model:        configured.Model,
			Timeout:      provider.Timeout,
			MaxTokens:    provider.MaxTokens,
		})
	default:
		return nil, fmt.Errorf("unsupported wire_api %q", provider.WireAPI)
	}
}

// effectiveShellAllowPrefixes merges the configured prefixes with those the
// user granted permanently (stored under the Equus home directory).
func effectiveShellAllowPrefixes(approval config.ApprovalConfig, home string) []string {
	merged := append([]string(nil), approval.ShellAllowPrefixes...)
	state, err := approvals.New(home).Load()
	if err == nil {
		merged = append(merged, state.ShellAllowPrefixes...)
	}
	return merged
}

// prefixAllowed reports whether command matches one of the allow prefixes on a
// word boundary.
func prefixAllowed(prefixes []string, command string) bool {
	command = strings.TrimSpace(command)
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if command == prefix || strings.HasPrefix(command, prefix+" ") {
			return true
		}
	}
	return false
}

// buildSession composes the model client and agent session from config,
// returning the session plus the model name.
func buildSession(configured config.Config, workingDir string) (*agent.Session, string, error) {
	provider, _, err := configured.ResolveProvider()
	if err != nil {
		return nil, "", err
	}
	client, err := newModelClient(configured, provider)
	if err != nil {
		return nil, "", err
	}
	registry, err := agent.NewDefaultRegistry(workingDir)
	if err != nil {
		return nil, "", err
	}
	approval := configured.Approval
	compaction := configured.Compaction
	allowlist := effectiveShellAllowPrefixes(approval, equusHome())
	session, err := agent.NewSession(client, agent.Options{
		Registry: registry,
		ApprovalMode: func(category string) agent.ApprovalMode {
			return agent.ApprovalMode(approval.EffectiveMode(category))
		},
		AutoApprove: func(category, summary string) bool {
			if category != "shell" {
				return false
			}
			return prefixAllowed(allowlist, summary)
		},
		OnPermanentGrant: func(category, summary string) error {
			// Only shell commands are recorded as exact prefixes.
			if category == "shell" {
				return approvals.New(equusHome()).AddShellPrefix(summary)
			}
			return nil
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
		Skills:      skill.Discover(equusHome(), workingDir),
		Override:    configured.BaseInstructions,
	}))
	if err != nil {
		return nil, "", err
	}
	return session, configured.Model, nil
}
