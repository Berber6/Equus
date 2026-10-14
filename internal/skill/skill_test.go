package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "greet", "---\nname: greet\ndescription: say hello\n---\nSay hello warmly.\n")
	skill, err := Parse(filepath.Join(root, "skills", "greet", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "greet" || skill.Description != "say hello" || !strings.Contains(skill.Body, "Say hello warmly.") {
		t.Fatalf("skill = %+v", skill)
	}
}

func TestParseRejectsMissingFrontmatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "SKILL.md")
	os.WriteFile(path, []byte("no frontmatter here"), 0o644)
	if _, err := Parse(path); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseRejectsInvalidName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "SKILL.md")
	os.WriteFile(path, []byte("---\nname: Bad Name\n---\nbody"), 0o644)
	if _, err := Parse(path); err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestDiscoverProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeSkill(t, home, "shared", "---\nname: shared\ndescription: from user\n---\nu\n")
	writeSkill(t, filepath.Join(cwd, ".equus"), "shared", "---\nname: shared\ndescription: from project\n---\np\n")
	writeSkill(t, home, "only-user", "---\nname: only-user\ndescription: u\n---\n")

	skills := Discover(home, cwd)
	if len(skills) != 2 {
		t.Fatalf("skills = %d: %+v", len(skills), skills)
	}
	// Sorted by name.
	if skills[0].Name != "only-user" || skills[1].Name != "shared" {
		t.Fatalf("order = %+v", skills)
	}
	if skills[1].Description != "from project" {
		t.Fatalf("project skill should win: %+v", skills[1])
	}
}

func TestDiscoverMissingDirs(t *testing.T) {
	if got := Discover(t.TempDir(), t.TempDir()); len(got) != 0 {
		t.Fatalf("expected no skills, got %+v", got)
	}
}
