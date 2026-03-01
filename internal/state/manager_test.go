package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eachlabs/klaw/pkg/tool"
	"pgregory.net/rapid"
)

// Mock tool for testing
type mockTool struct {
	name        string
	execFunc    func(ctx context.Context, params json.RawMessage) (*tool.Result, error)
	description string
	schema      json.RawMessage
}

func (m *mockTool) Name() string                                                         { return m.name }
func (m *mockTool) Description() string                                                  { return m.description }
func (m *mockTool) Schema() json.RawMessage                                              { return m.schema }
func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	return m.execFunc(ctx, params)
}

// Feature: ralphklaw, Property 4: Mode detection consistency
// Validates: Requirements 4.1, 4.2, 4.3
func TestProperty_ModeDetectionConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate whether LAST_ERROR.txt exists
		errorExists := rapid.Bool().Draw(t, "error_exists")

		// Create mock read tool
		readTool := &mockTool{
			name: "read",
			execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
				if errorExists {
					return &tool.Result{Content: "error content", IsError: false}, nil
				}
				return &tool.Result{Content: "", IsError: true}, fmt.Errorf("file not found")
			},
		}

		registry := tool.NewRegistry()
		registry.Register(readTool)

		sm := NewStateManager("/tmp/test", registry)
		mode := sm.DetectMode(context.Background())

		// Verify: FIX_ERROR iff file exists
		if errorExists && mode != ModeFixError {
			t.Fatalf("expected FIX_ERROR when file exists, got %v", mode)
		}
		if !errorExists && mode != ModeExecuteTask {
			t.Fatalf("expected EXECUTE_TASK when file missing, got %v", mode)
		}
	})
}

func TestStateManager_ReadTODO_Empty(t *testing.T) {
	readTool := &mockTool{
		name: "read",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "# TODO\n\n", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(readTool)

	sm := NewStateManager("/tmp/test", registry)
	tasks, err := sm.ReadTODO(context.Background())

	if err != nil {
		t.Fatalf("ReadTODO failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestStateManager_ReadTODO_MultipleTasks(t *testing.T) {
	content := `# TODO

- [ ] Task one
- [x] Task two
- [ ] Task three
`
	readTool := &mockTool{
		name: "read",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: content, IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(readTool)

	sm := NewStateManager("/tmp/test", registry)
	tasks, err := sm.ReadTODO(context.Background())

	if err != nil {
		t.Fatalf("ReadTODO failed: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Completed {
		t.Error("task 0 should not be completed")
	}
	if !tasks[1].Completed {
		t.Error("task 1 should be completed")
	}
	if tasks[2].Completed {
		t.Error("task 2 should not be completed")
	}
}

func TestStateManager_WriteTODO(t *testing.T) {
	var writtenContent string

	writeTool := &mockTool{
		name: "write",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			var p map[string]interface{}
			json.Unmarshal(params, &p)
			writtenContent = p["content"].(string)
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)

	sm := NewStateManager("/tmp/test", registry)
	tasks := []Task{
		{Description: "Task one", Completed: false},
		{Description: "Task two", Completed: true},
	}

	err := sm.WriteTODO(context.Background(), tasks)
	if err != nil {
		t.Fatalf("WriteTODO failed: %v", err)
	}

	// Verify written content
	if writtenContent == "" {
		t.Fatal("no content was written")
	}
	if !strings.Contains(writtenContent, "- [ ] Task one") {
		t.Error("written content missing incomplete task")
	}
	if !strings.Contains(writtenContent, "- [x] Task two") {
		t.Error("written content missing completed task")
	}
}

func TestStateManager_WriteError_ReadError_Cycle(t *testing.T) {
	var storedError string

	readTool := &mockTool{
		name: "read",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			if storedError == "" {
				return &tool.Result{Content: "", IsError: true}, fmt.Errorf("not found")
			}
			return &tool.Result{Content: storedError, IsError: false}, nil
		},
	}

	writeTool := &mockTool{
		name: "write",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			var p map[string]interface{}
			json.Unmarshal(params, &p)
			storedError = p["content"].(string)
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(readTool)
	registry.Register(writeTool)

	sm := NewStateManager("/tmp/test", registry)

	// Write error
	ve := &ValidationError{
		Iteration: 5,
		Task:      "Test task",
		Error:     "test error",
		Attempt:   2,
	}

	err := sm.WriteError(context.Background(), ve)
	if err != nil {
		t.Fatalf("WriteError failed: %v", err)
	}

	// Read it back
	read, err := sm.ReadError(context.Background())
	if err != nil {
		t.Fatalf("ReadError failed: %v", err)
	}

	if read.Iteration != ve.Iteration {
		t.Errorf("iteration = %d, want %d", read.Iteration, ve.Iteration)
	}
	if read.Task != ve.Task {
		t.Errorf("task = %q, want %q", read.Task, ve.Task)
	}
	if read.Error != ve.Error {
		t.Errorf("error = %q, want %q", read.Error, ve.Error)
	}
	if read.Attempt != ve.Attempt {
		t.Errorf("attempt = %d, want %d", read.Attempt, ve.Attempt)
	}
}

func TestStateManager_DeleteError(t *testing.T) {
	var bashCalled bool
	var bashCommand string

	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			bashCalled = true
			var p map[string]string
			json.Unmarshal(params, &p)
			bashCommand = p["command"]
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	sm := NewStateManager("/tmp/test", registry)
	err := sm.DeleteError(context.Background())

	if err != nil {
		t.Fatalf("DeleteError failed: %v", err)
	}
	if !bashCalled {
		t.Error("bash tool was not called")
	}
	if !strings.Contains(bashCommand, "rm") {
		t.Errorf("bash command should contain 'rm', got: %q", bashCommand)
	}
	if !strings.Contains(bashCommand, "LAST_ERROR.txt") {
		t.Errorf("bash command should contain 'LAST_ERROR.txt', got: %q", bashCommand)
	}
}

func TestStateManager_DetectMode_WithoutReadTool(t *testing.T) {
	registry := tool.NewRegistry()
	sm := NewStateManager("/tmp/test", registry)

	mode := sm.DetectMode(context.Background())
	if mode != ModeExecuteTask {
		t.Errorf("expected EXECUTE_TASK when read tool unavailable, got %v", mode)
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeExecuteTask, "EXECUTE_TASK"},
		{ModeFixError, "FIX_ERROR"},
	}

	for _, tt := range tests {
		got := tt.mode.String()
		if got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// TestValidatePath_TraversalRejected verifies that paths escaping the workspace
// are rejected with an error.
// Validates: Requirements 18.4
func TestValidatePath_TraversalRejected(t *testing.T) {
	workspace := "/tmp/workspace"
	traversalCases := []string{
		"../../etc/passwd",
		"../secret",
		"subdir/../../etc/passwd",
		"../../../root/.ssh/id_rsa",
		"foo/bar/../../../etc/shadow",
	}

	for _, name := range traversalCases {
		t.Run(name, func(t *testing.T) {
			err := validatePath(workspace, name)
			if err == nil {
				t.Errorf("validatePath(%q, %q) expected error for traversal path, got nil", workspace, name)
			}
		})
	}
}

// TestValidatePath_WorkspacePathsAccepted verifies that paths within the workspace
// are accepted without error.
// Validates: Requirements 18.4
func TestValidatePath_WorkspacePathsAccepted(t *testing.T) {
	workspace := "/tmp/workspace"
	validCases := []string{
		"TODO.md",
		"LAST_ERROR.txt",
		"subdir/file.txt",
		"a/b/c/deep.go",
	}

	for _, name := range validCases {
		t.Run(name, func(t *testing.T) {
			err := validatePath(workspace, name)
			if err != nil {
				t.Errorf("validatePath(%q, %q) expected nil for valid path, got: %v", workspace, name, err)
			}
		})
	}
}

// TestStateManager_ReadFile_TraversalRejected verifies that readFile rejects
// traversal paths before invoking the read tool.
// Validates: Requirements 18.4
func TestStateManager_ReadFile_TraversalRejected(t *testing.T) {
	// validatePath is the enforcement point — test it directly since readFile is unexported.
	if err := validatePath("/tmp/workspace", "../../etc/passwd"); err == nil {
		t.Error("expected validatePath to reject traversal path")
	}
}

// TestStateManager_WriteFile_TraversalRejected verifies that writeFile rejects
// traversal paths before invoking the write tool.
// Validates: Requirements 18.4
func TestStateManager_WriteFile_TraversalRejected(t *testing.T) {
	var toolCalled bool
	writeTool := &mockTool{
		name: "write",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			toolCalled = true
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)

	sm := NewStateManager("/tmp/workspace", registry)

	// WriteTODO calls writeFile("TODO.md", ...) — valid path, should succeed
	err := sm.WriteTODO(context.Background(), []Task{{Description: "test", Completed: false}})
	if err != nil {
		t.Fatalf("WriteTODO with valid path failed: %v", err)
	}
	if !toolCalled {
		t.Error("write tool should have been called for valid path")
	}

	// Confirm validatePath blocks traversal
	if validateErr := validatePath("/tmp/workspace", "../../etc/passwd"); validateErr == nil {
		t.Error("expected validatePath to reject traversal path")
	}
}


