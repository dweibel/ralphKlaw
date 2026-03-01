package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
)

type mockTool struct {
	name     string
	execFunc func(ctx context.Context, params json.RawMessage) (*tool.Result, error)
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return "" }
func (m *mockTool) Schema() json.RawMessage     { return nil }
func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	return m.execFunc(ctx, params)
}

func TestValidator_SuccessfulValidation(t *testing.T) {
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "all good", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	validator := NewValidator(registry, "", logging.NewNopLogger())
	err := validator.Validate(context.Background())

	if err != nil {
		t.Errorf("expected nil error for successful validation, got: %v", err)
	}
}

func TestValidator_FailedBuild_ReturnsError(t *testing.T) {
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{
				Content: "build failed: syntax error",
				IsError: true,
			}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	validator := NewValidator(registry, "", logging.NewNopLogger())
	err := validator.Validate(context.Background())

	if err == nil {
		t.Fatal("expected error for failed validation, got nil")
	}
	if err.Error() != "build failed: syntax error" {
		t.Errorf("error message = %q, want %q", err.Error(), "build failed: syntax error")
	}
}

func TestValidator_CustomCommand_IsUsed(t *testing.T) {
	var executedCommand string

	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			var p map[string]string
			json.Unmarshal(params, &p)
			executedCommand = p["command"]
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	customCmd := "go test ./... && go vet ./..."
	validator := NewValidator(registry, customCmd, logging.NewNopLogger())
	validator.Validate(context.Background())

	if executedCommand != customCmd {
		t.Errorf("executed command = %q, want %q", executedCommand, customCmd)
	}
}

func TestValidator_DefaultCommand_WhenEmpty(t *testing.T) {
	var executedCommand string

	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			var p map[string]string
			json.Unmarshal(params, &p)
			executedCommand = p["command"]
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	validator := NewValidator(registry, "", logging.NewNopLogger())
	validator.Validate(context.Background())

	expectedDefault := "go build ./... && go vet ./..."
	if executedCommand != expectedDefault {
		t.Errorf("executed command = %q, want default %q", executedCommand, expectedDefault)
	}
}

func TestValidator_BashToolUnavailable_ReturnsError(t *testing.T) {
	registry := tool.NewRegistry()

	validator := NewValidator(registry, "", logging.NewNopLogger())
	err := validator.Validate(context.Background())

	if err == nil {
		t.Fatal("expected error when bash tool unavailable, got nil")
	}
	if err.Error() != "bash tool not available" {
		t.Errorf("error message = %q, want %q", err.Error(), "bash tool not available")
	}
}

func TestValidator_ToolExecutionError_ReturnsError(t *testing.T) {
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return nil, fmt.Errorf("execution failed")
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	validator := NewValidator(registry, "", logging.NewNopLogger())
	err := validator.Validate(context.Background())

	if err == nil {
		t.Fatal("expected error when tool execution fails, got nil")
	}
}
