package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectGitBranch(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature-x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := detectGitBranch(sub); got != "feature-x" {
		t.Fatalf("branch = %q", got)
	}
}

func TestDetectGitBranchDetached(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	os.Mkdir(gitDir, 0o755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("a1b2c3d4e5f60718293a4b5c6d7e8f9012345678\n"), 0o644)
	if got := detectGitBranch(root); got != "a1b2c3d" {
		t.Fatalf("detached branch = %q", got)
	}
}

func TestDetectGitBranchGitFile(t *testing.T) {
	root := t.TempDir()
	// A worktree-style .git file pointing at a real gitdir elsewhere.
	realGit := filepath.Join(t.TempDir(), "worktrees", "wt")
	if err := os.MkdirAll(realGit, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(realGit, "HEAD"), []byte("ref: refs/heads/wt-branch\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: "+realGit+"\n"), 0o644)
	if got := detectGitBranch(root); got != "wt-branch" {
		t.Fatalf("worktree branch = %q", got)
	}
}

func TestDetectGitBranchNone(t *testing.T) {
	if got := detectGitBranch(t.TempDir()); got != "" {
		t.Fatalf("expected no branch, got %q", got)
	}
}

func TestStatusLineShowsBranch(t *testing.T) {
	model, err := NewModel(context.Background(), Options{Session: newTestSession(t, "hi"), Model: "m", CWD: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	model.branch = "main"
	if got := stripANSI(model.statusLine()); !strings.Contains(got, "⎇ main") {
		t.Fatalf("status line missing branch: %q", got)
	}
	model.branch = ""
	if got := stripANSI(model.statusLine()); strings.Contains(got, "⎇") {
		t.Fatalf("branch marker should be absent when unset: %q", got)
	}
}
