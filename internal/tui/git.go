package tui

import (
	"os"
	"path/filepath"
	"strings"
)

// detectGitBranch reports the current git branch for the repository containing
// dir, or "" when dir is not inside a git work tree. For a detached HEAD it
// returns the short commit hash. It reads .git/HEAD directly (no git process),
// so it is fast and cross-platform.
func detectGitBranch(dir string) string {
	gitDir, ok := findGitDir(dir)
	if !ok {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(head))
	if branch, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok {
		return strings.TrimSpace(branch)
	}
	// Detached HEAD: ref is a full commit hash.
	if len(ref) >= 8 {
		return ref[:7]
	}
	return ref
}

// findGitDir walks up from dir to locate the .git directory, following a .git
// file (worktree/submodule) to its gitdir.
func findGitDir(dir string) (string, bool) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(current, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate, true
			}
			// A .git file: "gitdir: <path>".
			if data, err := os.ReadFile(candidate); err == nil {
				line := strings.TrimSpace(string(data))
				if path, ok := strings.CutPrefix(line, "gitdir:"); ok {
					target := strings.TrimSpace(path)
					if !filepath.IsAbs(target) {
						target = filepath.Join(current, target)
					}
					return target, true
				}
			}
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}
