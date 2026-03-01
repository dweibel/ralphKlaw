// Package state manages file-based state for ralphKlaw.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eachlabs/klaw/pkg/tool"
)

// Mode represents the current execution mode.
type Mode int

const (
	ModeExecuteTask Mode = iota
	ModeFixError
)

// String returns the mode name.
func (m Mode) String() string {
	if m == ModeFixError {
		return "FIX_ERROR"
	}
	return "EXECUTE_TASK"
}

// StateManager reads and writes TODO.md and LAST_ERROR.txt via klaw tools.
type StateManager struct {
	workspace string
	tools     *tool.Registry
}

// NewStateManager creates a StateManager for the given workspace.
func NewStateManager(workspace string, tools *tool.Registry) *StateManager {
	return &StateManager{workspace: workspace, tools: tools}
}

// DetectMode checks for LAST_ERROR.txt to determine the execution mode.
func (s *StateManager) DetectMode(ctx context.Context) Mode {
	read, ok := s.tools.Get("read")
	if !ok {
		return ModeExecuteTask
	}

	params, _ := json.Marshal(map[string]string{
		"path": filepath.Join(s.workspace, "LAST_ERROR.txt"),
	})

	result, err := read.Execute(ctx, params)
	if err != nil || result.IsError {
		return ModeExecuteTask
	}

	return ModeFixError
}

// ReadTODO parses TODO.md and returns the task list.
func (s *StateManager) ReadTODO(ctx context.Context) ([]Task, error) {
	content, err := s.readFile(ctx, "TODO.md")
	if err != nil {
		return nil, err
	}
	return parseTasks(content), nil
}

// WriteTODO writes the task list back to TODO.md.
func (s *StateManager) WriteTODO(ctx context.Context, tasks []Task) error {
	return s.writeFile(ctx, "TODO.md", formatTasks(tasks))
}

// ReadError parses LAST_ERROR.txt.
func (s *StateManager) ReadError(ctx context.Context) (*ValidationError, error) {
	content, err := s.readFile(ctx, "LAST_ERROR.txt")
	if err != nil {
		return nil, err
	}
	return parseError(content)
}

// WriteError writes a validation error to LAST_ERROR.txt.
func (s *StateManager) WriteError(ctx context.Context, ve *ValidationError) error {
	return s.writeFile(ctx, "LAST_ERROR.txt", formatError(ve))
}

// DeleteError removes LAST_ERROR.txt.
func (s *StateManager) DeleteError(ctx context.Context) error {
	bash, ok := s.tools.Get("bash")
	if !ok {
		return fmt.Errorf("bash tool not available")
	}
	params, _ := json.Marshal(map[string]string{
		"command": fmt.Sprintf("rm -f %s", filepath.Join(s.workspace, "LAST_ERROR.txt")),
	})
	_, err := bash.Execute(ctx, params)
	return err
}

// validatePath resolves the absolute path of workspace+name and returns an
// error if the result does not reside within the workspace root, preventing
// directory traversal attacks.
func validatePath(workspace, name string) error {
	root := filepath.Clean(workspace)
	abs := filepath.Clean(filepath.Join(workspace, name))
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		return fmt.Errorf("path %q escapes workspace root", name)
	}
	return nil
}

func (s *StateManager) readFile(ctx context.Context, name string) (string, error) {
	if err := validatePath(s.workspace, name); err != nil {
		return "", err
	}
	read, ok := s.tools.Get("read")
	if !ok {
		return "", fmt.Errorf("read tool not available")
	}
	params, _ := json.Marshal(map[string]string{
		"path": filepath.Join(s.workspace, name),
	})
	result, err := read.Execute(ctx, params)
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", fmt.Errorf("read %s: %s", name, result.Content)
	}
	return result.Content, nil
}

func (s *StateManager) writeFile(ctx context.Context, name, content string) error {
	if err := validatePath(s.workspace, name); err != nil {
		return err
	}
	write, ok := s.tools.Get("write")
	if !ok {
		return fmt.Errorf("write tool not available")
	}
	params, _ := json.Marshal(map[string]interface{}{
		"path":    filepath.Join(s.workspace, name),
		"content": content,
	})
	result, err := write.Execute(ctx, params)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("write %s: %s", name, result.Content)
	}
	return nil
}
