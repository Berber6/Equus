// Package skill discovers and parses lightweight skills: SKILL.md files with
// YAML frontmatter (name, description) and a Markdown body. Skills are
// advertised in the system prompt; the model reads their body on demand.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxSkillBytes caps how much of a single SKILL.md is read.
const maxSkillBytes = 64 << 10

// Skill is one discovered skill.
type Skill struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// Parse reads and parses a SKILL.md file.
func Parse(path string) (Skill, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Skill{}, err
	}
	if info.Size() > maxSkillBytes {
		return Skill{}, fmt.Errorf("SKILL.md exceeds %d bytes", maxSkillBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	frontmatter, body, err := splitFrontmatter(string(content))
	if err != nil {
		return Skill{}, err
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return Skill{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	name := strings.TrimSpace(meta.Name)
	if !validName(name) {
		return Skill{}, fmt.Errorf("invalid skill name %q (use lowercase letters, digits, and hyphens)", name)
	}
	return Skill{
		Name:        name,
		Description: strings.TrimSpace(meta.Description),
		Body:        strings.TrimSpace(body),
		Path:        path,
	}, nil
}

// splitFrontmatter separates the YAML frontmatter from the body. The file must
// begin with a "---" line.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", "", fmt.Errorf("SKILL.md must start with '---' frontmatter")
	}
	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("SKILL.md frontmatter is not terminated with '---'")
	}
	frontmatter = rest[:end]
	body = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	return frontmatter, body, nil
}

// validName reports whether name is lowercase alphanumeric with hyphens.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// loadDir parses every "<dir>/<name>/SKILL.md" under a skills directory.
func loadDir(dir string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		if skill, err := Parse(path); err == nil {
			skills = append(skills, skill)
		}
	}
	return skills
}
