package approvals

import (
	"path/filepath"
	"testing"
)

func TestAddAndLoadPrefixes(t *testing.T) {
	store := New(t.TempDir())
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ShellAllowPrefixes) != 0 {
		t.Fatalf("expected empty state, got %v", state)
	}

	if err := store.AddShellPrefix("git status"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddShellPrefix("git status"); err != nil { // duplicate
		t.Fatal(err)
	}
	if err := store.AddShellPrefix("go test"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ShellAllowPrefixes) != 2 {
		t.Fatalf("prefixes = %v", reloaded.ShellAllowPrefixes)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "nope"))
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ShellAllowPrefixes) != 0 {
		t.Fatalf("state = %v", state)
	}
}
