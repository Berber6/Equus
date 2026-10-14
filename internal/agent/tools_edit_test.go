package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchGlobRecursive(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "internal/agent/tool.go", true},
		{"**/*.go", "README.md", false},
		{"internal/**/*.go", "internal/agent/tool.go", true},
		{"internal/**/*.go", "cmd/main.go", false},
		{"internal/**/*.go", "internal/tool.go", true},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
	}
	for _, tc := range cases {
		if got := matchGlob(tc.pattern, tc.name); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestApplyPatchToolUpdatesFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "line1\nline2\nline3\n")
	tool := ApplyPatchTool{Root: root}
	patchText := "*** Begin Patch\n*** Update File: a.txt\n@@\n-line2\n+CHANGED\n*** End Patch\n"
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]string{"patch": patchText}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Fatalf("result = %q", result)
	}
	if got := read(t, root, "a.txt"); got != "line1\nCHANGED\nline3\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyPatchToolRejectsEscape(t *testing.T) {
	root := t.TempDir()
	tool := ApplyPatchTool{Root: root}
	patchText := "*** Begin Patch\n*** Add File: ../escape.txt\n+bad\n*** End Patch\n"
	if _, err := tool.Execute(context.Background(), mustJSON(t, map[string]string{"patch": patchText})); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestGlobToolFindsGoFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main")
	write(t, root, "internal/agent/tool.go", "package agent")
	write(t, root, "README.md", "# hi")
	write(t, root, ".git/config", "should be skipped")

	tool := GlobTool{Root: root}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]string{"pattern": "**/*.go"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "main.go") || !strings.Contains(result, "internal/agent/tool.go") {
		t.Fatalf("result = %q", result)
	}
	if strings.Contains(result, "README.md") || strings.Contains(result, ".git") {
		t.Fatalf("unexpected match: %q", result)
	}
}

func TestGlobToolNoMatches(t *testing.T) {
	tool := GlobTool{Root: t.TempDir()}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]string{"pattern": "**/*.nonexistent"}))
	if err != nil {
		t.Fatal(err)
	}
	if result != "(no matches)" {
		t.Fatalf("result = %q", result)
	}
}

func TestGrepToolMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "package main\n\nfunc main() {}\n")
	write(t, root, "b.txt", "nothing here\n")

	tool := GrepTool{Root: root}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"pattern": "func main", "glob": "*.go"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "a.go:3: func main() {}") {
		t.Fatalf("result = %q", result)
	}
	if strings.Contains(result, "b.txt") {
		t.Fatalf("glob filter failed: %q", result)
	}
}

func TestGrepToolCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "Hello World\n")
	tool := GrepTool{Root: root}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"pattern": "hello", "case_insensitive": true}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello World") {
		t.Fatalf("result = %q", result)
	}
}

func TestGrepToolInvalidRegexp(t *testing.T) {
	tool := GrepTool{Root: t.TempDir()}
	if _, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"pattern": "("})); err == nil {
		t.Fatal("expected invalid regexp error")
	}
}

func TestGrepToolSkipsBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0x00, 0x01, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, root, "text.txt", "needle\n")
	tool := GrepTool{Root: root}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"pattern": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "bin.dat") {
		t.Fatalf("binary file searched: %q", result)
	}
}

func TestNewDefaultRegistryHasEditTools(t *testing.T) {
	registry, err := NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"apply_patch", "glob", "grep"} {
		if _, ok := registry.lookup(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
