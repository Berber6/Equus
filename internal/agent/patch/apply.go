package patch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result summarizes one applied (or attempted) hunk.
type Result struct {
	Path    string
	Action  string // "add", "delete", "update", "move"
	Applied bool
}

// Apply applies hunks to the filesystem rooted at root, returning a summary
// per hunk. Hunks are applied in order; when one fails, earlier changes are
// kept (no rollback) and the error reports the failing hunk.
func Apply(root string, hunks []Hunk) ([]Result, error) {
	results := make([]Result, 0, len(hunks))
	for _, hunk := range hunks {
		result, err := applyHunk(root, hunk)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func applyHunk(root string, hunk Hunk) (Result, error) {
	switch h := hunk.(type) {
	case *AddFile:
		return applyAdd(root, h)
	case *DeleteFile:
		return applyDelete(root, h)
	case *UpdateFile:
		return applyUpdate(root, h)
	default:
		return Result{}, fmt.Errorf("unknown hunk type %T", hunk)
	}
}

func applyAdd(root string, hunk *AddFile) (Result, error) {
	path, err := resolvePath(root, hunk.FilePath)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(path, []byte(hunk.Contents), 0o644); err != nil {
		return Result{}, err
	}
	return Result{Path: hunk.FilePath, Action: "add", Applied: true}, nil
}

func applyDelete(root string, hunk *DeleteFile) (Result, error) {
	path, err := resolvePath(root, hunk.FilePath)
	if err != nil {
		return Result{}, err
	}
	if err := os.Remove(path); err != nil {
		return Result{}, fmt.Errorf("delete %s: %w", hunk.FilePath, err)
	}
	return Result{Path: hunk.FilePath, Action: "delete", Applied: true}, nil
}

func applyUpdate(root string, hunk *UpdateFile) (Result, error) {
	source, err := resolvePath(root, hunk.FilePath)
	if err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(source)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", hunk.FilePath, err)
	}
	updated, err := applyChunks(string(content), hunk.Chunks, hunk.FilePath)
	if err != nil {
		return Result{}, err
	}

	action := "update"
	if hunk.MovePath != "" {
		action = "move"
	}
	target := source
	if hunk.MovePath != "" {
		target, err = resolvePath(root, hunk.MovePath)
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Result{}, err
		}
	}
	if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}
	if hunk.MovePath != "" && target != source {
		if err := os.Remove(source); err != nil {
			return Result{}, fmt.Errorf("move %s -> %s: %w", hunk.FilePath, hunk.MovePath, err)
		}
	}
	return Result{Path: hunk.Path(), Action: action, Applied: true}, nil
}

// applyChunks rewrites the file text by applying the chunks in order.
func applyChunks(content string, chunks []Chunk, path string) (string, error) {
	lines, hadTrailingNewline := splitLines(content)

	type replacement struct {
		start    int
		remove   int
		inserted []string
	}
	var replacements []replacement
	lineIndex := 0

	for _, chunk := range chunks {
		if chunk.ChangeContext != "" {
			found := seekSequence(lines, []string{chunk.ChangeContext}, lineIndex, false)
			if found < 0 {
				return "", fmt.Errorf("failed to find context '%s' in %s", chunk.ChangeContext, path)
			}
			lineIndex = found + 1
		}

		if len(chunk.OldLines) == 0 {
			replacements = append(replacements, replacement{start: len(lines), inserted: chunk.NewLines})
			continue
		}

		pattern := chunk.OldLines
		newLines := chunk.NewLines
		found := seekSequence(lines, pattern, lineIndex, chunk.EndOfFile)
		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			// Retry without a trailing empty line that represents the final
			// newline of the file.
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekSequence(lines, pattern, lineIndex, chunk.EndOfFile)
		}
		if found < 0 {
			return "", fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.OldLines, "\n"))
		}

		// Split the matched region into context (preserved verbatim) and
		// changed spans, so context lines keep their exact contents.
		oldStart, newStart := 0, 0
		for _, pair := range chunk.ContextIndices {
			oldContext, newContext := pair[0], pair[1]
			if oldContext >= len(pattern) || newContext >= len(newLines) {
				break
			}
			if oldStart != oldContext || newStart != newContext {
				replacements = append(replacements, replacement{
					start:    found + oldStart,
					remove:   oldContext - oldStart,
					inserted: append([]string(nil), newLines[newStart:newContext]...),
				})
			}
			oldStart = oldContext + 1
			newStart = newContext + 1
		}
		if oldStart != len(pattern) || newStart != len(newLines) {
			replacements = append(replacements, replacement{
				start:    found + oldStart,
				remove:   len(pattern) - oldStart,
				inserted: append([]string(nil), newLines[newStart:]...),
			})
		}
		lineIndex = found + len(pattern)
	}

	sort.SliceStable(replacements, func(i, j int) bool {
		return replacements[i].start < replacements[j].start
	})

	result := make([]string, 0, len(lines))
	cursor := 0
	for _, r := range replacements {
		if r.start < cursor {
			// Overlapping replacements: skip to keep the result well-formed.
			continue
		}
		result = append(result, lines[cursor:r.start]...)
		result = append(result, r.inserted...)
		cursor = r.start + r.remove
	}
	result = append(result, lines[cursor:]...)

	return joinLines(result, hadTrailingNewline), nil
}

// splitLines splits content into lines without their terminators, reporting
// whether the content ended with a newline.
func splitLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	trailing := strings.HasSuffix(content, "\n")
	body := content
	if trailing {
		body = body[:len(body)-1]
	}
	return strings.Split(body, "\n"), trailing
}

func joinLines(lines []string, trailingNewline bool) string {
	joined := strings.Join(lines, "\n")
	if trailingNewline && joined != "" {
		joined += "\n"
	}
	return joined
}

// seekSequence finds pattern within lines starting at start, relaxing the
// match in stages: exact, then ignoring trailing whitespace, then ignoring
// leading and trailing whitespace, then normalizing Unicode punctuation.
// When eof is set, the search prefers matching at the end of the file.
func seekSequence(lines, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	searchStart := start
	if eof && len(lines) >= len(pattern) {
		eofStart := len(lines) - len(pattern)
		if eofStart > searchStart {
			searchStart = eofStart
		}
	}
	limit := len(lines) - len(pattern)

	// 1. Exact match.
	for i := searchStart; i <= limit; i++ {
		if equalRun(lines[i:i+len(pattern)], pattern, exactEq) {
			return i
		}
	}
	// 2. Ignore trailing whitespace.
	for i := searchStart; i <= limit; i++ {
		if equalRun(lines[i:i+len(pattern)], pattern, trimRightEq) {
			return i
		}
	}
	// 3. Ignore leading and trailing whitespace.
	for i := searchStart; i <= limit; i++ {
		if equalRun(lines[i:i+len(pattern)], pattern, trimEq) {
			return i
		}
	}
	// 4. Normalize Unicode punctuation (typographic dashes/quotes/spaces).
	for i := searchStart; i <= limit; i++ {
		if equalRun(lines[i:i+len(pattern)], pattern, normalizeEq) {
			return i
		}
	}
	return -1
}

func equalRun(candidate, pattern []string, eq func(a, b string) bool) bool {
	for index, want := range pattern {
		if !eq(candidate[index], want) {
			return false
		}
	}
	return true
}

func exactEq(a, b string) bool { return a == b }
func trimRightEq(a, b string) bool {
	return strings.TrimRight(a, " \t\r") == strings.TrimRight(b, " \t\r")
}
func trimEq(a, b string) bool      { return strings.TrimSpace(a) == strings.TrimSpace(b) }
func normalizeEq(a, b string) bool { return normalizePunctuation(a) == normalizePunctuation(b) }

// normalizePunctuation maps common typographic punctuation to ASCII so patches
// authored with plain characters still match files that contain fancy glyphs.
func normalizePunctuation(s string) string {
	s = strings.TrimSpace(s)
	var builder strings.Builder
	builder.Grow(len(s))
	for _, r := range s {
		switch r {
		case '‐', '‑', '‒', '–', '—', '―', '−':
			builder.WriteByte('-')
		case '‘', '’', '‚', '‛':
			builder.WriteByte('\'')
		case '“', '”', '„', '‟':
			builder.WriteByte('"')
		case ' ', ' ', ' ', ' ', ' ', ' ', ' ',
			' ', ' ', ' ', ' ', ' ', '　':
			builder.WriteByte(' ')
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// resolvePath keeps patched file access inside root.
func resolvePath(root, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("path is empty")
	}
	cleaned := filepath.Clean(raw)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(root, cleaned)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}
	if abs != absRoot && !strings.HasPrefix(abs, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the working directory", raw)
	}
	return abs, nil
}
