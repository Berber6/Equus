package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Godric-W/Equus/internal/agent/patch"
)

// maxSearchResults bounds how many matches glob and grep return, keeping a
// single tool call from flooding the model's context.
const maxSearchResults = 200

// searchSkipDirs are directory names pruned during glob/grep traversal.
var searchSkipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, "bin": {}, "dist": {}, "target": {},
	"vendor": {}, ".venv": {}, "__pycache__": {},
}

// ApplyPatchTool edits files using a patch in the apply_patch format.
type ApplyPatchTool struct{ Root string }

func (tool ApplyPatchTool) Name() string { return "apply_patch" }

// ApprovalKind reports the policy category for this tool.
func (tool ApplyPatchTool) ApprovalKind() approvalKind { return approvalKindWrite }

// ApprovalSummary returns a short description of the patch for approval.
func (tool ApplyPatchTool) ApprovalSummary(arguments []byte) (string, bool) {
	var input struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", false
	}
	first := strings.TrimSpace(input.Patch)
	if index := strings.IndexByte(first, '\n'); index >= 0 {
		first = strings.TrimSpace(first[:index])
	}
	return "apply patch: " + first, strings.TrimSpace(input.Patch) != ""
}

func (tool ApplyPatchTool) Description() string {
	return "Apply a patch to create, update, delete or move files. The patch is a block of text " +
		"starting with '*** Begin Patch' and ending with '*** End Patch', containing hunks of the form " +
		"'*** Add File: <path>', '*** Delete File: <path>' or '*** Update File: <path>' (optionally followed by " +
		"'*** Move to: <path>'). Update hunks contain '@@' context markers and lines prefixed with ' ' (context), " +
		"'-' (remove) or '+' (add). Paths are relative to the working directory."
}

func (tool ApplyPatchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {"type": "string", "description": "The full patch text, starting with '*** Begin Patch' and ending with '*** End Patch'."}
		},
		"required": ["patch"]
	}`)
}

func (tool ApplyPatchTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("apply_patch: decode arguments: %w", err)
	}
	if strings.TrimSpace(input.Patch) == "" {
		return "", fmt.Errorf("apply_patch: patch is empty")
	}
	hunks, err := patch.Parse(input.Patch)
	if err != nil {
		return "", fmt.Errorf("apply_patch: %w", err)
	}
	if len(hunks) == 0 {
		return "apply_patch: patch contained no hunks", nil
	}
	results, err := patch.Apply(tool.Root, hunks)
	if err != nil {
		// Report what succeeded before the failure so the model can react.
		if len(results) > 0 {
			return summarizeResults(results), fmt.Errorf("apply_patch: %w", err)
		}
		return "", fmt.Errorf("apply_patch: %w", err)
	}
	return summarizeResults(results), nil
}

func summarizeResults(results []patch.Result) string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		lines = append(lines, fmt.Sprintf("%s %s", result.Action, result.Path))
	}
	return "applied:\n" + strings.Join(lines, "\n")
}

// GlobTool finds files matching a glob pattern inside the working directory.
type GlobTool struct{ Root string }

func (tool GlobTool) Name() string { return "glob" }

func (tool GlobTool) Description() string {
	return "Find files whose path matches a glob pattern relative to the working directory. " +
		"'**' matches any number of directories, e.g. '**/*.go' or 'internal/**/*.go'."
}

func (tool GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern relative to the working directory; '**' matches recursively."}
		},
		"required": ["pattern"]
	}`)
}

func (tool GlobTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("glob: decode arguments: %w", err)
	}
	pattern := strings.TrimSpace(input.Pattern)
	if pattern == "" {
		return "", fmt.Errorf("glob: pattern is empty")
	}
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")

	var matches []string
	err := walkFiles(tool.Root, func(relPath string) {
		if matchGlob(pattern, relPath) {
			matches = append(matches, relPath)
		}
	})
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	truncated := false
	if len(matches) > maxSearchResults {
		matches = matches[:maxSearchResults]
		truncated = true
	}
	result := strings.Join(matches, "\n")
	if truncated {
		result += fmt.Sprintf("\n... (%d results shown; narrow the pattern for more)", maxSearchResults)
	}
	return result, nil
}

// GrepTool searches file contents for a regular expression.
type GrepTool struct{ Root string }

func (tool GrepTool) Name() string { return "grep" }

func (tool GrepTool) Description() string {
	return "Search file contents for a Go regular expression and return matching lines as 'path:line: text'. " +
		"Optionally restrict by a glob (e.g. '*.go') and/or a starting path. Binary and non-UTF-8 files are skipped."
}

func (tool GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Go regular expression to search for."},
			"path": {"type": "string", "description": "File or directory to search, relative to the working directory. Defaults to the working directory."},
			"glob": {"type": "string", "description": "Optional filename glob (e.g. '*.go') to restrict which files are searched."},
			"case_insensitive": {"type": "boolean", "description": "Match case-insensitively."}
		},
		"required": ["pattern"]
	}`)
}

func (tool GrepTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Glob            string `json:"glob"`
		CaseInsensitive bool   `json:"case_insensitive"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("grep: decode arguments: %w", err)
	}
	pattern := input.Pattern
	if pattern == "" {
		return "", fmt.Errorf("grep: pattern is empty")
	}
	if input.CaseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid regexp: %w", err)
	}

	target := strings.TrimSpace(input.Path)
	resolvedTarget, err := resolvePath(tool.Root, target)
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}

	var matches []string
	appendMatches := func(displayPath, content string) {
		scanner := bufio.NewScanner(strings.NewReader(content))
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			if re.MatchString(scanner.Text()) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", displayPath, lineNumber, scanner.Text()))
			}
		}
	}

	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}
	if info.IsDir() {
		globPattern := strings.TrimSpace(input.Glob)
		err = walkFiles(resolvedTarget, func(relPath string) {
			if globPattern != "" && !matchGlob(filepath.ToSlash(globPattern), filepath.Base(relPath)) &&
				!matchGlob(filepath.ToSlash(globPattern), filepath.ToSlash(relPath)) {
				return
			}
			content, ok := readTextFile(filepath.Join(resolvedTarget, relPath))
			if !ok {
				return
			}
			display := relPath
			if rel, err := filepath.Rel(tool.Root, filepath.Join(resolvedTarget, relPath)); err == nil {
				display = filepath.ToSlash(rel)
			}
			appendMatches(display, content)
		})
		if err != nil {
			return "", fmt.Errorf("grep: %w", err)
		}
	} else {
		content, ok := readTextFile(resolvedTarget)
		if !ok {
			return "(no matches)", nil
		}
		display := target
		if rel, err := filepath.Rel(tool.Root, resolvedTarget); err == nil {
			display = filepath.ToSlash(rel)
		}
		appendMatches(display, content)
	}

	if len(matches) == 0 {
		return "(no matches)", nil
	}
	truncated := false
	if len(matches) > maxSearchResults {
		matches = matches[:maxSearchResults]
		truncated = true
	}
	result := strings.Join(matches, "\n")
	if truncated {
		result += fmt.Sprintf("\n... (%d matches shown; narrow the search)", maxSearchResults)
	}
	return result, nil
}

// walkFiles walks root, invoking visit for every regular file with its
// slash-separated path relative to root. Pruned directories are skipped.
func walkFiles(root string, visit func(relPath string)) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the walk
		}
		if entry.IsDir() {
			if path != absRoot {
				if _, skip := searchSkipDirs[entry.Name()]; skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		visit(filepath.ToSlash(rel))
		return nil
	})
}

// readTextFile reads a file when it looks like UTF-8 text, reporting whether
// it should be searched.
func readTextFile(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return "", false
	}
	return string(content), true
}
