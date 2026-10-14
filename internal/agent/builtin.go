package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolvePath keeps tool file access inside the agent working directory.
func resolvePath(root, raw string) (string, error) {
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

// ReadFileTool reads a UTF-8 text file inside the working directory.
type ReadFileTool struct{ Root string }

func (tool ReadFileTool) Name() string { return "read_file" }

func (tool ReadFileTool) Description() string {
	return "Read a UTF-8 text file relative to the working directory and return its content."
}

func (tool ReadFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path relative to the working directory."}
		},
		"required": ["path"]
	}`)
}

func (tool ReadFileTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("read_file: decode arguments: %w", err)
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", errors.New("read_file: path is empty")
	}
	resolved, err := resolvePath(tool.Root, input.Path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return string(content), nil
}

// WriteFileTool writes a UTF-8 text file inside the working directory.
type WriteFileTool struct{ Root string }

func (tool WriteFileTool) Name() string { return "write_file" }

// ApprovalKind reports the policy category for this tool.
func (tool WriteFileTool) ApprovalKind() approvalKind { return approvalKindWrite }

// ApprovalSummary returns the file being written for approval prompts.
func (tool WriteFileTool) ApprovalSummary(arguments []byte) (string, bool) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", false
	}
	return "write " + strings.TrimSpace(input.Path), strings.TrimSpace(input.Path) != ""
}

func (tool WriteFileTool) Description() string {
	return "Create or overwrite a UTF-8 text file relative to the working directory with the given content."
}

func (tool WriteFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":    {"type": "string", "description": "File path relative to the working directory."},
			"content": {"type": "string", "description": "Full file content to write."}
		},
		"required": ["path", "content"]
	}`)
}

func (tool WriteFileTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", fmt.Errorf("write_file: decode arguments: %w", err)
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", errors.New("write_file: path is empty")
	}
	resolved, err := resolvePath(tool.Root, input.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(input.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(input.Content), input.Path), nil
}

// ListFilesTool lists files in a directory inside the working directory.
type ListFilesTool struct{ Root string }

func (tool ListFilesTool) Name() string { return "list_files" }

func (tool ListFilesTool) Description() string {
	return "List the file and directory names under a path in the working directory."
}

func (tool ListFilesTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path relative to the working directory. Defaults to the working directory itself."}
		},
		"required": []
	}`)
}

func (tool ListFilesTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &input); err != nil {
			return "", fmt.Errorf("list_files: decode arguments: %w", err)
		}
	}
	resolved, err := resolvePath(tool.Root, input.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("list_files: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name()+"/")
		} else {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(names, "\n"), nil
}

// NewReadOnlyRegistry builds the read-only tool set (no writes, no shell, no
// network) used by subagents.
func NewReadOnlyRegistry(root string) (*toolRegistry, error) {
	registry := &toolRegistry{}
	for _, tool := range []Tool{
		ReadFileTool{Root: root},
		ListFilesTool{Root: root},
		GlobTool{Root: root},
		GrepTool{Root: root},
	} {
		if err := registry.register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// NewDefaultRegistry builds the default tool set rooted at the working
// directory.
func NewDefaultRegistry(root string) (*toolRegistry, error) {
	registry := &toolRegistry{}
	for _, tool := range []Tool{
		ReadFileTool{Root: root},
		WriteFileTool{Root: root},
		ListFilesTool{Root: root},
		ApplyPatchTool{Root: root},
		GlobTool{Root: root},
		GrepTool{Root: root},
		ShellTool{Root: root},
		WebFetchTool{},
	} {
		if err := registry.register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
