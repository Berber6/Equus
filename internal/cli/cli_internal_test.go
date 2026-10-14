package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Godric-W/Equus/internal/approvals"
	"github.com/Godric-W/Equus/internal/config"
	"github.com/Godric-W/Equus/internal/threadstore"
)

func TestPrefixAllowedWordBoundary(t *testing.T) {
	prefixes := []string{"git status", "go test"}
	cases := []struct {
		command string
		want    bool
	}{
		{"git status", true},
		{"git status --short", true},
		{"  git diff ", false},
		{"git statusx", false},
		{"go test ./...", true},
	}
	for _, tc := range cases {
		if got := prefixAllowed(prefixes, tc.command); got != tc.want {
			t.Errorf("prefixAllowed(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestEffectiveShellAllowPrefixesMergesState(t *testing.T) {
	home := t.TempDir()
	if err := approvals.New(home).AddShellPrefix("custom-cmd"); err != nil {
		t.Fatal(err)
	}
	approval := config.ApprovalConfig{ShellAllowPrefixes: []string{"git status"}}
	merged := effectiveShellAllowPrefixes(approval, home)
	joined := strings.Join(merged, ",")
	if !strings.Contains(joined, "git status") || !strings.Contains(joined, "custom-cmd") {
		t.Fatalf("merged = %v", merged)
	}
}

func TestEquusHomeEnvOverride(t *testing.T) {
	t.Setenv(config.EnvHome, "/custom/equus")
	if got := equusHome(); got != "/custom/equus" {
		t.Fatalf("equusHome = %q", got)
	}
}

func TestOpenSessionNoSave(t *testing.T) {
	store := threadstore.New(t.TempDir())
	writer, restored, err := openSession(store, chatOptions{noSave: true}, "/w", "m")
	if err != nil {
		t.Fatal(err)
	}
	if writer != nil || restored != nil {
		t.Fatalf("expected no writer/restore, got %v %v", writer, restored)
	}
}

func TestOpenSessionCreatesAndResumes(t *testing.T) {
	store := threadstore.New(t.TempDir())
	writer, restored, err := openSession(store, chatOptions{}, "/w", "m")
	if err != nil {
		t.Fatal(err)
	}
	if writer == nil || restored != nil {
		t.Fatalf("expected a new writer, got %v %v", writer, restored)
	}
	id := writer.ID()
	writer.Close()

	w2, restored2, err := openSession(store, chatOptions{resumeID: id}, "/w", "m")
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	if restored2 == nil || restored2.Meta.ID != id {
		t.Fatalf("resume failed: %+v", restored2)
	}
}

func TestOpenSessionBadResume(t *testing.T) {
	store := threadstore.New(t.TempDir())
	if _, _, err := openSession(store, chatOptions{resumeID: "nope"}, "/w", "m"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestNewModelClientSelectsWire(t *testing.T) {
	base := config.Config{Model: "m", ModelProvider: "openai"}
	for _, wire := range []config.WireAPI{config.WireAPIChatCompletions, config.WireAPIResponses, config.WireAPIAnthropicMessages} {
		provider := config.ModelProviderInfo{WireAPI: wire, APIKey: "k", BaseURL: "http://x/v1"}
		client, err := newModelClient(base, provider)
		if err != nil {
			t.Fatalf("wire %q: %v", wire, err)
		}
		if client == nil {
			t.Fatalf("wire %q: nil client", wire)
		}
	}
	if _, err := newModelClient(base, config.ModelProviderInfo{WireAPI: "bogus", APIKey: "k", BaseURL: "http://x"}); err == nil {
		t.Fatal("expected error for unknown wire_api")
	}
}

func TestVersionCommandSmoke(t *testing.T) {
	var out bytes.Buffer
	code := Run(context.Background(), []string{"version"}, strings.NewReader(""), &out, &out)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "equus ") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestConfigShowCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	keyFile := filepath.Join(home, "api_key.txt")
	if err := os.WriteFile(keyFile, []byte("sk-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := "model: m\nmodel_providers:\n  openai:\n    wire_api: chat_completions\n    api_key_file: api_key.txt\n    base_url: http://x/v1\n"
	if err := os.WriteFile(filepath.Join(home, config.ConfigFileName), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Run(context.Background(), []string{"config", "show"}, strings.NewReader(""), &out, &out)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %q", code, out.String())
	}
	// The key must be redacted and the file path must not leak the secret.
	if strings.Contains(out.String(), "sk-secret") {
		t.Fatalf("secret leaked: %q", out.String())
	}
}
