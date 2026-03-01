// Package testutil provides shared test helpers for ralphklaw packages.
package testutil

import (
	"context"
	"encoding/json"

	"github.com/eachlabs/klaw/pkg/tool"
)

// MockTool is a reusable tool.Tool implementation for tests.
type MockTool struct {
	ToolName        string
	ToolDescription string
	ToolSchema      json.RawMessage
	ExecFunc        func(ctx context.Context, params json.RawMessage) (*tool.Result, error)
}

func (m *MockTool) Name() string            { return m.ToolName }
func (m *MockTool) Description() string     { return m.ToolDescription }
func (m *MockTool) Schema() json.RawMessage { return m.ToolSchema }
func (m *MockTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	return m.ExecFunc(ctx, params)
}
