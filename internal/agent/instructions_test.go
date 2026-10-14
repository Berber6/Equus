package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Godric-W/Equus/internal/skill"
)

func TestBuildBaseInstructionsIncludesEnvironment(t *testing.T) {
	got := BuildBaseInstructions(InstructionsInput{
		CWD:      "/work/proj",
		Platform: "linux/amd64",
		Tools:    []string{"read_file", "apply_patch", "shell"},
	})
	for _, want := range []string{"/work/proj", "linux/amd64", "apply_patch", "shell", "glob", "apply_patch"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions missing %q:\n%s", want, got)
		}
	}
}

func TestBuildBaseInstructionsOverrideWins(t *testing.T) {
	got := BuildBaseInstructions(InstructionsInput{
		CWD:      "/work",
		Override: "CUSTOM PROMPT",
	})
	if got != "CUSTOM PROMPT" {
		t.Fatalf("override not honored: %q", got)
	}
}

func TestBuildBaseInstructionsIncludesProjectDocs(t *testing.T) {
	got := BuildBaseInstructions(InstructionsInput{
		CWD:         "/work",
		ProjectDocs: []string{"Use tabs, not spaces."},
	})
	if !strings.Contains(got, "Use tabs, not spaces.") || !strings.Contains(got, "Project instructions") {
		t.Fatalf("project docs not injected:\n%s", got)
	}
}

func TestDiscoverAgentsDocsParentToChild(t *testing.T) {
	root := t.TempDir()
	// Mark the root as a project.
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "sub")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), "root guidance")
	writeFile(t, filepath.Join(child, "AGENTS.md"), "child guidance")

	docs := DiscoverAgentsDocs(child)
	if len(docs) != 2 {
		t.Fatalf("docs = %d, want 2: %v", len(docs), docs)
	}
	if !strings.Contains(docs[0], "root guidance") || !strings.Contains(docs[1], "child guidance") {
		t.Fatalf("order wrong (want parent first): %v", docs)
	}
}

func TestDiscoverAgentsDocsNone(t *testing.T) {
	if docs := DiscoverAgentsDocs(t.TempDir()); len(docs) != 0 {
		t.Fatalf("expected no docs, got %v", docs)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildBaseInstructionsIncludesSkills(t *testing.T) {
	got := BuildBaseInstructions(InstructionsInput{
		CWD:    "/w",
		Skills: []skill.Skill{{Name: "deploy", Description: "how to deploy", Path: "/w/skills/deploy/SKILL.md"}},
	})
	if !strings.Contains(got, "Available skills") || !strings.Contains(got, "deploy: how to deploy") {
		t.Fatalf("skills not injected:\n%s", got)
	}
}
