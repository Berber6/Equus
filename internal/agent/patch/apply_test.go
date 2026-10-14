package patch

import (
	"os"
	"path/filepath"
	"testing"
)

func mustApply(t *testing.T, root, text string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hunks, err := Parse(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Apply(root, hunks); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestApplyUpdateSingleLine(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "*** Begin Patch\n*** Update File: a.txt\n@@\n-line2\n+CHANGED\n*** End Patch\n",
		map[string]string{"a.txt": "line1\nline2\nline3\n"})
	if got := readFile(t, root, "a.txt"); got != "line1\nCHANGED\nline3\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyUpdateMultipleChunks(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n*** Update File: multi.txt\n" +
		"@@\n-line2\n+changed2\n" +
		"@@\n-line4\n+changed4\n" +
		"*** End Patch\n"
	mustApply(t, root, patch, map[string]string{"multi.txt": "line1\nline2\nline3\nline4\n"})
	if got := readFile(t, root, "multi.txt"); got != "line1\nchanged2\nline3\nchanged4\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyUpdateWithContextAnchor(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n*** Update File: a.txt\n@@ def g():\n-    pass\n+    return 2\n*** End Patch\n"
	mustApply(t, root, patch, map[string]string{"a.txt": "def f():\n    pass\n\ndef g():\n    pass\n"})
	if got := readFile(t, root, "a.txt"); got != "def f():\n    pass\n\ndef g():\n    return 2\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyUpdateFuzzyTrailingWhitespace(t *testing.T) {
	root := t.TempDir()
	// File lines carry trailing spaces; the patch omits them.
	patch := "*** Begin Patch\n*** Update File: a.txt\n@@\n-foo\n-bar\n+ok\n*** End Patch\n"
	mustApply(t, root, patch, map[string]string{"a.txt": "foo   \nbar\t\t\n"})
	if got := readFile(t, root, "a.txt"); got != "ok\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyAddFile(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "*** Begin Patch\n*** Add File: sub/new.txt\n+hello\n+world\n*** End Patch\n", nil)
	if got := readFile(t, root, "sub/new.txt"); got != "hello\nworld\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyDeleteFile(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "*** Begin Patch\n*** Delete File: gone.txt\n*** End Patch\n",
		map[string]string{"gone.txt": "bye\n"})
	if _, err := os.Stat(filepath.Join(root, "gone.txt")); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestApplyMoveFile(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-a\n+b\n*** End Patch\n"
	mustApply(t, root, patch, map[string]string{"old.txt": "a\n"})
	if _, err := os.Stat(filepath.Join(root, "old.txt")); !os.IsNotExist(err) {
		t.Fatal("old file should be gone")
	}
	if got := readFile(t, root, "new.txt"); got != "b\n" {
		t.Fatalf("moved content = %q", got)
	}
}

func TestApplyUpdatesMultipleFiles(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n" +
		"*** Update File: a.txt\n@@\n-a\n+A\n" +
		"*** Update File: b.txt\n@@\n-b\n+B\n" +
		"*** End Patch\n"
	mustApply(t, root, patch, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	if got := readFile(t, root, "a.txt"); got != "A\n" {
		t.Fatalf("a = %q", got)
	}
	if got := readFile(t, root, "b.txt"); got != "B\n" {
		t.Fatalf("b = %q", got)
	}
}

func TestApplyFailsWhenContextNotFound(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n*** Update File: a.txt\n@@\n-nope\n+x\n*** End Patch\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, hunks); err == nil {
		t.Fatal("expected apply error for missing context")
	}
}

func TestApplyPartialSuccessKeepsEarlierChanges(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n" +
		"*** Update File: a.txt\n@@\n-a\n+A\n" +
		"*** Update File: b.txt\n@@\n-missing\n+X\n" +
		"*** End Patch\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, hunks); err == nil {
		t.Fatal("expected error")
	}
	// The first hunk must have been applied and not rolled back.
	if got := readFile(t, root, "a.txt"); got != "A\n" {
		t.Fatalf("a.txt = %q, want A (earlier change preserved)", got)
	}
}

func TestApplyRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n*** Update File: ../escape.txt\n@@\n-a\n+b\n*** End Patch\n"
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, hunks); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestSeekSequenceStages(t *testing.T) {
	lines := []string{"foo   ", "bar\t\t"}
	if got := seekSequence(lines, []string{"foo", "bar"}, 0, false); got != 0 {
		t.Fatalf("rstrip match = %d, want 0", got)
	}
	if got := seekSequence([]string{"a", "b", "c"}, []string{"b", "c"}, 0, false); got != 1 {
		t.Fatalf("exact match = %d, want 1", got)
	}
	if got := seekSequence([]string{"just one"}, []string{"too", "many"}, 0, false); got != -1 {
		t.Fatalf("oversized pattern = %d, want -1", got)
	}
}
