package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Godric-W/Equus/internal/skill"
)

// maxAgentsDocBytes caps how much of a single AGENTS.md is injected.
const maxAgentsDocBytes = 64 << 10

// InstructionsInput describes the environment used to build the system prompt.
type InstructionsInput struct {
	// CWD is the working directory shown to the model.
	CWD string
	// Platform is the OS/arch (e.g. "linux/amd64").
	Platform string
	// Tools lists the available tool names.
	Tools []string
	// ProjectDocs are AGENTS.md contents (parent first).
	ProjectDocs []string
	// Skills lists discovered skills advertised to the model.
	Skills []skill.Skill
	// Override, when non-empty, replaces the whole default prompt.
	Override string
}

// BuildBaseInstructions renders the system prompt. A non-empty Override wins.
func BuildBaseInstructions(input InstructionsInput) string {
	if override := strings.TrimSpace(input.Override); override != "" {
		return override
	}
	var b strings.Builder
	b.WriteString("You are Equus, a minimal terminal coding agent written in Go. ")
	b.WriteString("You help with software engineering tasks in the user's working directory.\n")

	b.WriteString("\n## Environment\n")
	if input.CWD != "" {
		b.WriteString("- Working directory: " + input.CWD + "\n")
	}
	if input.Platform != "" {
		b.WriteString("- Platform: " + input.Platform + "\n")
	}
	if len(input.Tools) > 0 {
		b.WriteString("- Available tools: " + strings.Join(input.Tools, ", ") + "\n")
	}

	b.WriteString("\n## Using tools\n")
	b.WriteString("- Locate code with glob and grep before reading or editing; do not guess file contents.\n")
	b.WriteString("- Read a file before editing it. Make changes with apply_patch using enough surrounding context to be unambiguous.\n")
	b.WriteString("- Use apply_patch (not write_file) for edits to existing files; reserve write_file for creating new files.\n")
	b.WriteString("- Use shell to build, test and inspect the project; commands may require the user's approval.\n")
	b.WriteString("- All file paths are relative to the working directory and must stay inside it.\n")

	b.WriteString("\n## Editing\n")
	b.WriteString("- Prefer ASCII unless the file already uses other characters.\n")
	b.WriteString("- Add comments only where they aid understanding; avoid obvious ones.\n")
	b.WriteString("- Do not revert or overwrite changes you did not make.\n")
	b.WriteString("- Answer concisely and report what you did.\n")

	if len(input.ProjectDocs) > 0 {
		b.WriteString("\n## Project instructions\n")
		b.WriteString("The following project-specific guidance (AGENTS.md) applies and takes precedence:\n\n")
		for _, doc := range input.ProjectDocs {
			b.WriteString(strings.TrimSpace(doc))
			b.WriteString("\n\n")
		}
	}
	if len(input.Skills) > 0 {
		b.WriteString("\n## Available skills\n")
		b.WriteString("Reusable instructions you can read with read_file when relevant:\n")
		for _, s := range input.Skills {
			line := "- " + s.Name
			if s.Description != "" {
				line += ": " + s.Description
			}
			if s.Path != "" {
				line += " (" + s.Path + ")"
			}
			b.WriteString(line + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// DiscoverAgentsDocs collects AGENTS.md contents from the project root down to
// cwd (parent first, child last so child guidance can override). It returns an
// empty slice when none are found. Files larger than maxAgentsDocBytes are
// truncated.
func DiscoverAgentsDocs(cwd string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil
	}
	root := projectRoot(abs)
	dirs := pathFromRootToCwd(root, abs)

	var docs []string
	for _, dir := range dirs {
		path := filepath.Join(dir, "AGENTS.md")
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(content)
		if len(text) > maxAgentsDocBytes {
			text = text[:maxAgentsDocBytes] + "\n... (truncated)"
		}
		if strings.TrimSpace(text) != "" {
			docs = append(docs, text)
		}
	}
	return docs
}

// projectRoot walks upwards from dir to the first directory containing a .git
// entry, falling back to dir itself.
func projectRoot(dir string) string {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return dir
		}
		current = parent
	}
}

// pathFromRootToCwd returns the directories from root down to cwd inclusive.
func pathFromRootToCwd(root, cwd string) []string {
	if !strings.HasPrefix(cwd, root) {
		return []string{cwd}
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == "." {
		return []string{root}
	}
	dirs := []string{root}
	current := root
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		dirs = append(dirs, current)
	}
	return dirs
}
