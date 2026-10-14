// Package patch implements a trimmed port of the apply_patch patch format: a
// line-oriented grammar that supports adding, deleting, updating and moving
// files with fuzzy context matching. It is adapted from the format used by
// Codex's apply_patch tool, reduced to a non-streaming parser and applier.
package patch

import (
	"fmt"
	"strings"
)

// Patch markers.
const (
	beginPatchMarker  = "*** Begin Patch"
	endPatchMarker    = "*** End Patch"
	addFileMarker     = "*** Add File: "
	deleteFileMarker  = "*** Delete File: "
	updateFileMarker  = "*** Update File: "
	moveToMarker      = "*** Move to: "
	eofMarker         = "*** End of File"
	changeContextMark = "@@ "
	emptyContextMark  = "@@"
)

// Hunk is one operation in a patch. Implementations are AddFile, DeleteFile
// and UpdateFile.
type Hunk interface {
	// Path returns the file the hunk operates on (the move destination for a
	// rename).
	Path() string
}

// AddFile creates a new file with Contents.
type AddFile struct {
	FilePath string
	Contents string
}

// Path returns the affected file.
func (h AddFile) Path() string { return h.FilePath }

// DeleteFile removes a file.
type DeleteFile struct {
	FilePath string
}

// Path returns the affected file.
func (h DeleteFile) Path() string { return h.FilePath }

// UpdateFile applies chunks to a file, optionally moving it to MovePath.
type UpdateFile struct {
	FilePath string
	MovePath string // empty when the file is not moved
	Chunks   []Chunk
}

// Path returns the move destination when set, otherwise the source file.
func (h UpdateFile) Path() string {
	if h.MovePath != "" {
		return h.MovePath
	}
	return h.FilePath
}

// Chunk is one contiguous edit within an UpdateFile hunk.
type Chunk struct {
	// ChangeContext is an optional anchor line (the text after "@@ ") that must
	// appear before the chunk's old lines.
	ChangeContext string
	OldLines      []string
	NewLines      []string
	// ContextIndices records (old, new) index pairs for lines that are context
	// (unchanged) so their exact contents survive application.
	ContextIndices [][2]int
	// EndOfFile anchors the chunk to the end of the file.
	EndOfFile bool
}

func (c *Chunk) pushContext(line string) {
	c.ContextIndices = append(c.ContextIndices, [2]int{len(c.OldLines), len(c.NewLines)})
	c.OldLines = append(c.OldLines, line)
	c.NewLines = append(c.NewLines, line)
}

// ParseError describes a malformed patch.
type ParseError struct {
	message    string
	lineNumber int // 0 when the error is not tied to a specific line
}

func (err *ParseError) Error() string {
	if err.lineNumber > 0 {
		return fmt.Sprintf("invalid hunk at line %d, %s", err.lineNumber, err.message)
	}
	return "invalid patch: " + err.message
}

func invalidPatch(message string) error { return &ParseError{message: message} }

func invalidHunk(line int, message string) error {
	return &ParseError{message: message, lineNumber: line}
}

const validHeadersHint = "'*** Add File: {path}', '*** Delete File: {path}', '*** Update File: {path}'"

// Parse parses a patch text into its hunks. It tolerates surrounding
// whitespace (both around markers and around the whole patch).
func Parse(text string) ([]Hunk, error) {
	rawLines := strings.Split(text, "\n")

	// Trim leading/trailing blank lines to locate the markers robustly.
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		lines = append(lines, strings.TrimRight(line, "\r"))
	}
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	lines = lines[start:end]

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != beginPatchMarker {
		return nil, invalidPatch("the first line of the patch must be '" + beginPatchMarker + "'")
	}
	if strings.TrimSpace(lines[len(lines)-1]) != endPatchMarker {
		return nil, invalidPatch("the last line of the patch must be '" + endPatchMarker + "'")
	}
	// Strip the Begin/End markers; line numbers below are 1-based over the
	// original patch text, so offset accordingly.
	body := lines[1 : len(lines)-1]
	baseLine := start + 1 // 1-based index of the first body line

	var hunks []Hunk
	mode := modeNone
	bodyIndex := 0
	for bodyIndex < len(body) {
		line := body[bodyIndex]
		trimmed := strings.TrimSpace(line)
		lineNumber := baseLine + bodyIndex
		bodyIndex++

		switch {
		case trimmed == beginPatchMarker:
			return nil, invalidHunk(lineNumber, "'"+trimmed+"' is not allowed inside a patch")
		case strings.HasPrefix(trimmed, addFileMarker):
			path := strings.TrimPrefix(trimmed, addFileMarker)
			if strings.TrimSpace(path) == "" {
				return nil, invalidHunk(lineNumber, "Add File requires a path")
			}
			mode = modeAdd
			hunks = append(hunks, &AddFile{FilePath: path})
		case strings.HasPrefix(trimmed, deleteFileMarker):
			path := strings.TrimPrefix(trimmed, deleteFileMarker)
			if strings.TrimSpace(path) == "" {
				return nil, invalidHunk(lineNumber, "Delete File requires a path")
			}
			mode = modeDelete
			hunks = append(hunks, &DeleteFile{FilePath: path})
		case strings.HasPrefix(trimmed, updateFileMarker):
			path := strings.TrimPrefix(trimmed, updateFileMarker)
			if strings.TrimSpace(path) == "" {
				return nil, invalidHunk(lineNumber, "Update File requires a path")
			}
			mode = modeUpdate
			hunks = append(hunks, &UpdateFile{FilePath: path})
		default:
			if err := parseBodyLine(&mode, &hunks, line, trimmed, lineNumber); err != nil {
				return nil, err
			}
		}
	}

	// Validate that update hunks are non-empty.
	for _, hunk := range hunks {
		if update, ok := hunk.(*UpdateFile); ok && len(update.Chunks) == 0 {
			return nil, invalidPatch("update file hunk for path '" + update.FilePath + "' is empty")
		}
	}
	return hunks, nil
}

type parseMode int

const (
	modeNone parseMode = iota
	modeAdd
	modeDelete
	modeUpdate
)

// parseBodyLine handles a line that is not a file-level header.
func parseBodyLine(mode *parseMode, hunks *[]Hunk, line, trimmed string, lineNumber int) error {
	switch *mode {
	case modeAdd:
		last := (*hunks)[len(*hunks)-1].(*AddFile)
		if content, ok := strings.CutPrefix(line, "+"); ok {
			last.Contents += content + "\n"
			return nil
		}
		return invalidHunk(lineNumber, "'"+trimmed+"' is not a valid hunk header. Valid hunk headers: "+validHeadersHint)
	case modeDelete:
		return invalidHunk(lineNumber, "'"+trimmed+"' is not a valid hunk header. Valid hunk headers: "+validHeadersHint)
	case modeUpdate:
		return parseUpdateLine((*hunks)[len(*hunks)-1].(*UpdateFile), line, trimmed, lineNumber)
	default:
		return invalidHunk(lineNumber, "'"+trimmed+"' is not a valid hunk header. Valid hunk headers: "+validHeadersHint)
	}
}

// parseUpdateLine folds one line into the current UpdateFile hunk.
func parseUpdateLine(update *UpdateFile, line, trimmed string, lineNumber int) error {
	// A move directive is only allowed before any chunk is started.
	if strings.HasPrefix(trimmed, moveToMarker) {
		if len(update.Chunks) > 0 || update.MovePath != "" {
			return invalidHunk(lineNumber, "unexpected '"+trimmed+"' in update hunk")
		}
		update.MovePath = strings.TrimPrefix(trimmed, moveToMarker)
		return nil
	}

	last := currentChunk(update)

	// "@@" starts a new chunk (optionally with a context anchor).
	if trimmed == emptyContextMark || strings.HasPrefix(trimmed, changeContextMark) {
		if last != nil && len(last.OldLines) == 0 && len(last.NewLines) == 0 {
			return invalidHunk(lineNumber, "unexpected line found in update hunk: '"+line+"'. Every line should start with ' ' (context line), '+' (added line), or '-' (removed line)")
		}
		chunk := Chunk{}
		if strings.HasPrefix(trimmed, changeContextMark) {
			chunk.ChangeContext = strings.TrimPrefix(trimmed, changeContextMark)
		}
		update.Chunks = append(update.Chunks, chunk)
		return nil
	}

	if last == nil {
		// Lines before any "@@" implicitly open the first chunk.
		update.Chunks = append(update.Chunks, Chunk{})
		last = &update.Chunks[len(update.Chunks)-1]
	}

	if trimmed == eofMarker {
		if len(last.OldLines) == 0 && len(last.NewLines) == 0 {
			return invalidHunk(lineNumber, "update hunk does not contain any lines")
		}
		last.EndOfFile = true
		return nil
	}

	if line == "" {
		last.pushContext("")
		return nil
	}
	switch line[0] {
	case ' ':
		last.pushContext(line[1:])
	case '+':
		last.NewLines = append(last.NewLines, line[1:])
	case '-':
		last.OldLines = append(last.OldLines, line[1:])
	default:
		return invalidHunk(lineNumber, "expected update hunk to start with a @@ context marker, got: '"+line+"'")
	}
	return nil
}

// currentChunk returns a pointer to the last chunk, or nil when none exists.
func currentChunk(update *UpdateFile) *Chunk {
	if len(update.Chunks) == 0 {
		return nil
	}
	return &update.Chunks[len(update.Chunks)-1]
}
