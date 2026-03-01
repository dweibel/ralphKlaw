package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eachlabs/klaw/pkg/provider"
	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
	"pgregory.net/rapid"
)

// Mock provider for testing
type mockProvider struct {
	responses []*provider.ChatResponse
	callCount int
	chatFunc  func(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error)
}

func (m *mockProvider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, req)
	}
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) Models() []string {
	return []string{"mock-model"}
}

// Mock tool for testing
type mockTool struct {
	name        string
	execFunc    func(ctx context.Context, params json.RawMessage) (*tool.Result, error)
	description string
	schema      json.RawMessage
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Schema() json.RawMessage { return m.schema }
func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	return m.execFunc(ctx, params)
}

// Feature: ralphklaw, Property 7: Inner loop always terminates
// Validates: Requirements 3.4
func TestProperty_InnerLoopAlwaysTerminates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a sequence of mock LLM responses
		maxRounds := rapid.IntRange(1, 20).Draw(t, "max_rounds")
		numResponses := rapid.IntRange(1, maxRounds+5).Draw(t, "num_responses")
		
		responses := make([]*provider.ChatResponse, numResponses)
		for i := 0; i < numResponses; i++ {
			// Randomly decide if this response has tool calls
			hasToolCalls := rapid.Bool().Draw(t, fmt.Sprintf("has_tool_calls_%d", i))
			
			if hasToolCalls && i < numResponses-1 {
				// Response with tool calls
				numToolCalls := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("num_tool_calls_%d", i))
				toolCalls := make([]provider.ToolCall, numToolCalls)
				for j := 0; j < numToolCalls; j++ {
					toolCalls[j] = provider.ToolCall{
						ID:    fmt.Sprintf("tool_%d_%d", i, j),
						Name:  "test_tool",
						Input: json.RawMessage(`{}`),
					}
				}
				
				responses[i] = &provider.ChatResponse{
					ID: fmt.Sprintf("resp_%d", i),
					Content: []provider.ContentBlock{
						{Type: "text", Text: fmt.Sprintf("response %d", i)},
						{Type: "tool_use", ToolUse: &toolCalls[0]},
					},
					StopReason: "tool_use",
				}
			} else {
				// Response without tool calls (terminal)
				responses[i] = &provider.ChatResponse{
					ID: fmt.Sprintf("resp_%d", i),
					Content: []provider.ContentBlock{
						{Type: "text", Text: fmt.Sprintf("final response %d", i)},
					},
					StopReason: "end_turn",
				}
			}
		}
		
		mockProv := &mockProvider{responses: responses}
		
		// Create mock tool
		testTool := &mockTool{
			name: "test_tool",
			execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
				return &tool.Result{Content: "tool result", IsError: false}, nil
			},
			description: "test tool",
			schema:      json.RawMessage(`{"type":"object"}`),
		}
		
		registry := tool.NewRegistry()
		registry.Register(testTool)
		
		logger, _ := logging.NewLogger("error", "/dev/null")
		defer logger.Close()
		
		config := &InnerLoopConfig{
			MaxRounds:   maxRounds,
			MaxTokens:   1000,
			Temperature: 0.0,
			Model:       "test-model",
		}
		
		inner := NewInnerLoop(mockProv, registry, config, logger)
		
		// Execute should always terminate
		_, err := inner.Execute(context.Background(), "system", "user prompt")
		
		// Either succeeds (no tool calls) or fails (max rounds exceeded)
		// But it MUST terminate
		if err != nil {
			// If error, it should be max rounds exceeded
			if mockProv.callCount > maxRounds {
				t.Fatalf("inner loop exceeded max rounds: called %d times with max %d", mockProv.callCount, maxRounds)
			}
		} else {
			// If success, it should have terminated before max rounds
			if mockProv.callCount > maxRounds {
				t.Fatalf("inner loop succeeded but exceeded max rounds: called %d times with max %d", mockProv.callCount, maxRounds)
			}
		}
	})
}

// Feature: ralphklaw, Property 8: Tool errors are fed back to LLM
// Validates: Requirements 16.1
func TestProperty_ToolErrorsFedBackToLLM(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate whether tool execution should fail
		toolShouldFail := rapid.Bool().Draw(t, "tool_should_fail")
		
		var capturedMessages []provider.Message
		
		mockProv := &mockProvider{
			chatFunc: func(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
				capturedMessages = req.Messages
				
				// First call: return tool call
				if len(capturedMessages) == 1 {
					return &provider.ChatResponse{
						ID: "resp_1",
						Content: []provider.ContentBlock{
							{Type: "text", Text: "calling tool"},
							{Type: "tool_use", ToolUse: &provider.ToolCall{
								ID:    "tool_1",
								Name:  "test_tool",
								Input: json.RawMessage(`{}`),
							}},
						},
						StopReason: "tool_use",
					}, nil
				}
				
				// Second call: no tool calls (terminate)
				return &provider.ChatResponse{
					ID: "resp_2",
					Content: []provider.ContentBlock{
						{Type: "text", Text: "done"},
					},
					StopReason: "end_turn",
				}, nil
			},
		}
		
		testTool := &mockTool{
			name: "test_tool",
			execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
				if toolShouldFail {
					return nil, fmt.Errorf("tool execution failed")
				}
				return &tool.Result{Content: "success", IsError: false}, nil
			},
			description: "test tool",
			schema:      json.RawMessage(`{"type":"object"}`),
		}
		
		registry := tool.NewRegistry()
		registry.Register(testTool)
		
		logger, _ := logging.NewLogger("error", "/dev/null")
		defer logger.Close()
		
		config := &InnerLoopConfig{
			MaxRounds:   10,
			MaxTokens:   1000,
			Temperature: 0.0,
			Model:       "test-model",
		}
		
		inner := NewInnerLoop(mockProv, registry, config, logger)
		_, _ = inner.Execute(context.Background(), "system", "user prompt")
		
		// Verify: if tool failed, error should be in messages with IsError=true
		if toolShouldFail && len(capturedMessages) >= 3 {
			// Third message should be tool result with error
			toolResultMsg := capturedMessages[2]
			if toolResultMsg.ToolResult == nil {
				t.Fatalf("expected tool result message, got nil")
			}
			if !toolResultMsg.ToolResult.IsError {
				t.Fatalf("expected IsError=true for failed tool, got false")
			}
			if toolResultMsg.ToolResult.Content == "" {
				t.Fatalf("expected error content in tool result, got empty")
			}
		}
		
		// Verify: if tool succeeded, result should be in messages with IsError=false
		if !toolShouldFail && len(capturedMessages) >= 3 {
			toolResultMsg := capturedMessages[2]
			if toolResultMsg.ToolResult == nil {
				t.Fatalf("expected tool result message, got nil")
			}
			if toolResultMsg.ToolResult.IsError {
				t.Fatalf("expected IsError=false for successful tool, got true")
			}
		}
	})
}

// Unit test: single-turn completion (no tool calls)
func TestInnerLoop_SingleTurnCompletion(t *testing.T) {
	mockProv := &mockProvider{
		responses: []*provider.ChatResponse{
			{
				ID: "resp_1",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "task completed"},
				},
				StopReason: "end_turn",
			},
		},
	}
	
	registry := tool.NewRegistry()
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   10,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	result, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "task completed" {
		t.Errorf("result = %q, want %q", result, "task completed")
	}
	if mockProv.callCount != 1 {
		t.Errorf("callCount = %d, want 1", mockProv.callCount)
	}
}

// Unit test: multi-turn with tool calls and results
func TestInnerLoop_MultiTurnWithToolCalls(t *testing.T) {
	toolCallCount := 0
	
	mockProv := &mockProvider{
		responses: []*provider.ChatResponse{
			// First response: tool call
			{
				ID: "resp_1",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "calling tool"},
					{Type: "tool_use", ToolUse: &provider.ToolCall{
						ID:    "tool_1",
						Name:  "read_tool",
						Input: json.RawMessage(`{"path":"test.txt"}`),
					}},
				},
				StopReason: "tool_use",
			},
			// Second response: done
			{
				ID: "resp_2",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "task completed"},
				},
				StopReason: "end_turn",
			},
		},
	}
	
	readTool := &mockTool{
		name: "read_tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			toolCallCount++
			return &tool.Result{Content: "file content", IsError: false}, nil
		},
		description: "read file",
		schema:      json.RawMessage(`{"type":"object"}`),
	}
	
	registry := tool.NewRegistry()
	registry.Register(readTool)
	
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   10,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	result, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "task completed" {
		t.Errorf("result = %q, want %q", result, "task completed")
	}
	if mockProv.callCount != 2 {
		t.Errorf("provider callCount = %d, want 2", mockProv.callCount)
	}
	if toolCallCount != 1 {
		t.Errorf("tool callCount = %d, want 1", toolCallCount)
	}
}

// Unit test: unknown tool returns error to LLM
func TestInnerLoop_UnknownToolReturnsError(t *testing.T) {
	var capturedMessages []provider.Message
	
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
			capturedMessages = req.Messages
			
			// First call: return unknown tool call
			if len(capturedMessages) == 1 {
				return &provider.ChatResponse{
					ID: "resp_1",
					Content: []provider.ContentBlock{
						{Type: "tool_use", ToolUse: &provider.ToolCall{
							ID:    "tool_1",
							Name:  "unknown_tool",
							Input: json.RawMessage(`{}`),
						}},
					},
					StopReason: "tool_use",
				}, nil
			}
			
			// Second call: terminate
			return &provider.ChatResponse{
				ID: "resp_2",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "done"},
				},
				StopReason: "end_turn",
			}, nil
		},
	}
	
	registry := tool.NewRegistry()
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   10,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	_, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Verify error was fed back to LLM
	if len(capturedMessages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(capturedMessages))
	}
	
	toolResultMsg := capturedMessages[2]
	if toolResultMsg.ToolResult == nil {
		t.Fatal("expected tool result message")
	}
	if !toolResultMsg.ToolResult.IsError {
		t.Error("expected IsError=true for unknown tool")
	}
	if toolResultMsg.ToolResult.Content == "" {
		t.Error("expected error content")
	}
	if !strings.Contains(toolResultMsg.ToolResult.Content, "unknown tool") {
		t.Errorf("expected 'unknown tool' in error, got: %q", toolResultMsg.ToolResult.Content)
	}
}

// Unit test: max rounds exceeded returns error
func TestInnerLoop_MaxRoundsExceeded(t *testing.T) {
	callCount := 0
	// Provider always returns tool calls
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
			callCount++
			return &provider.ChatResponse{
				ID: "resp",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolUse: &provider.ToolCall{
						ID:    "tool_1",
						Name:  "test_tool",
						Input: json.RawMessage(`{}`),
					}},
				},
				StopReason: "tool_use",
			}, nil
		},
	}
	
	testTool := &mockTool{
		name: "test_tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "result", IsError: false}, nil
		},
		description: "test",
		schema:      json.RawMessage(`{}`),
	}
	
	registry := tool.NewRegistry()
	registry.Register(testTool)
	
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   3,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	_, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err == nil {
		t.Fatal("expected error for max rounds exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "max rounds") {
		t.Errorf("expected 'max rounds' in error, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

// Unit test: end_turn stop reason terminates loop
func TestInnerLoop_EndTurnStopReason(t *testing.T) {
	mockProv := &mockProvider{
		responses: []*provider.ChatResponse{
			// Response with tool call but end_turn stop reason
			{
				ID: "resp_1",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "done"},
					{Type: "tool_use", ToolUse: &provider.ToolCall{
						ID:    "tool_1",
						Name:  "test_tool",
						Input: json.RawMessage(`{}`),
					}},
				},
				StopReason: "end_turn",
			},
		},
	}
	
	testTool := &mockTool{
		name: "test_tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "result", IsError: false}, nil
		},
		description: "test",
		schema:      json.RawMessage(`{}`),
	}
	
	registry := tool.NewRegistry()
	registry.Register(testTool)
	
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   10,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	result, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "done" {
		t.Errorf("result = %q, want %q", result, "done")
	}
	// Should terminate after first call despite tool call
	if mockProv.callCount != 1 {
		t.Errorf("callCount = %d, want 1", mockProv.callCount)
	}
}

// Unit test: tool execution error is fed back with IsError=true
func TestInnerLoop_ToolExecutionError(t *testing.T) {
	var capturedMessages []provider.Message
	
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
			capturedMessages = req.Messages
			
			// First call: return tool call
			if len(capturedMessages) == 1 {
				return &provider.ChatResponse{
					ID: "resp_1",
					Content: []provider.ContentBlock{
						{Type: "tool_use", ToolUse: &provider.ToolCall{
							ID:    "tool_1",
							Name:  "failing_tool",
							Input: json.RawMessage(`{}`),
						}},
					},
					StopReason: "tool_use",
				}, nil
			}
			
			// Second call: terminate
			return &provider.ChatResponse{
				ID: "resp_2",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "handled error"},
				},
				StopReason: "end_turn",
			}, nil
		},
	}
	
	failingTool := &mockTool{
		name: "failing_tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			return nil, fmt.Errorf("tool execution failed")
		},
		description: "failing tool",
		schema:      json.RawMessage(`{}`),
	}
	
	registry := tool.NewRegistry()
	registry.Register(failingTool)
	
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{
		MaxRounds:   10,
		MaxTokens:   1000,
		Temperature: 0.0,
		Model:       "test-model",
	}
	
	inner := NewInnerLoop(mockProv, registry, config, logger)
	_, err := inner.Execute(context.Background(), "system", "user prompt")
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Verify error was fed back
	if len(capturedMessages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(capturedMessages))
	}
	
	toolResultMsg := capturedMessages[2]
	if toolResultMsg.ToolResult == nil {
		t.Fatal("expected tool result message")
	}
	if !toolResultMsg.ToolResult.IsError {
		t.Error("expected IsError=true for failed tool execution")
	}
	if !strings.Contains(toolResultMsg.ToolResult.Content, "tool error") {
		t.Errorf("expected 'tool error' in content, got: %q", toolResultMsg.ToolResult.Content)
	}
}

// Unit test: buildToolDefs converts registry to provider format
func TestInnerLoop_BuildToolDefs(t *testing.T) {
	tool1 := &mockTool{
		name:        "tool1",
		description: "first tool",
		schema:      json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
	}
	
	tool2 := &mockTool{
		name:        "tool2",
		description: "second tool",
		schema:      json.RawMessage(`{"type":"object"}`),
	}
	
	registry := tool.NewRegistry()
	registry.Register(tool1)
	registry.Register(tool2)
	
	logger, _ := logging.NewLogger("error", "/dev/null")
	defer logger.Close()
	
	config := &InnerLoopConfig{MaxRounds: 10, MaxTokens: 1000, Model: "test"}
	inner := NewInnerLoop(&mockProvider{}, registry, config, logger)
	
	defs := inner.buildToolDefs()
	
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool defs, got %d", len(defs))
	}
	
	// Check first tool
	if defs[0].Name != "tool1" {
		t.Errorf("defs[0].Name = %q, want %q", defs[0].Name, "tool1")
	}
	if defs[0].Description != "first tool" {
		t.Errorf("defs[0].Description = %q, want %q", defs[0].Description, "first tool")
	}
	if string(defs[0].InputSchema) != string(tool1.schema) {
		t.Errorf("defs[0].InputSchema mismatch")
	}
	
	// Check second tool
	if defs[1].Name != "tool2" {
		t.Errorf("defs[1].Name = %q, want %q", defs[1].Name, "tool2")
	}
}


