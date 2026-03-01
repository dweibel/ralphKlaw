package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
	"pgregory.net/rapid"
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

// Feature: ralphklaw, Property 5: Commit messages contain required metadata
// Validates: Requirements 13.3, 13.4, 13.5, 13.6
func TestProperty_CommitMessagesContainMetadata(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		iteration := rapid.IntRange(1, 1000).Draw(t, "iteration")
		mode := rapid.SampledFrom([]string{"EXECUTE_TASK", "FIX_ERROR", "MAX_ITERATIONS"}).Draw(t, "mode")
		outcome := rapid.StringMatching(`^[a-zA-Z0-9 :_-]+$`).Draw(t, "outcome")

		config := &GitConfig{
			Enabled:        true,
			CommitTemplate: "ralphKlaw: iteration {iteration} [{mode}] {outcome}",
		}

		gm := &GitManager{config: config, logger: logging.NewNopLogger()}
		msg := gm.ExpandTemplate(config.CommitTemplate, iteration, mode, outcome)

		// Verify all three values are present
		iterStr := fmt.Sprintf("%d", iteration)
		if !strings.Contains(msg, iterStr) {
			t.Fatalf("commit message missing iteration %d: %q", iteration, msg)
		}
		if !strings.Contains(msg, mode) {
			t.Fatalf("commit message missing mode %q: %q", mode, msg)
		}
		if !strings.Contains(msg, outcome) {
			t.Fatalf("commit message missing outcome %q: %q", outcome, msg)
		}
	})
}

// Feature: ralphklaw, Property 6: Git operations are non-destructive
// Validates: Requirements 14.5
func TestProperty_GitOperationsNonDestructive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numCalls := rapid.IntRange(1, 20).Draw(t, "num_calls")
		
		var commands []string
		bashTool := &mockTool{
			name: "bash",
			execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
				var p map[string]string
				json.Unmarshal(params, &p)
				commands = append(commands, p["command"])
				return &tool.Result{Content: "ok", IsError: false}, nil
			},
		}

		registry := tool.NewRegistry()
		registry.Register(bashTool)

		config := &GitConfig{Enabled: true, AutoPush: true}
		gm := NewGitManager(registry, config, logging.NewNopLogger())

		// Simulate multiple CommitAndPush calls
		for i := 0; i < numCalls; i++ {
			gm.CommitAndPush(context.Background(), i+1, "EXECUTE_TASK", "test")
		}

		// Verify no destructive commands
		destructive := []string{"force", "reset --hard", "push -f", "push --force", "-f"}
		for _, cmd := range commands {
			for _, d := range destructive {
				if strings.Contains(cmd, d) {
					t.Fatalf("destructive command found: %q contains %q", cmd, d)
				}
			}
		}
	})
}

func TestGitManager_GenerateBranchName_DefaultPattern(t *testing.T) {
	config := &GitConfig{}
	gm := &GitManager{config: config, logger: logging.NewNopLogger()}

	name := gm.generateBranchName()

	if !strings.HasPrefix(name, "ralphklaw/") {
		t.Errorf("branch name should start with 'ralphklaw/', got: %q", name)
	}
}

func TestGitManager_GenerateBranchName_CustomPattern(t *testing.T) {
	config := &GitConfig{BranchPattern: "custom/{timestamp}"}
	gm := &GitManager{config: config, logger: logging.NewNopLogger()}

	name := gm.generateBranchName()

	if !strings.HasPrefix(name, "custom/") {
		t.Errorf("branch name should start with 'custom/', got: %q", name)
	}
}

func TestGitManager_ExpandTemplate(t *testing.T) {
	config := &GitConfig{}
	gm := &GitManager{config: config, logger: logging.NewNopLogger()}

	msg := gm.ExpandTemplate("iter {iteration} mode {mode} out {outcome}", 5, "TEST", "success")

	if !strings.Contains(msg, "iter 5") {
		t.Error("expanded message missing iteration")
	}
	if !strings.Contains(msg, "mode TEST") {
		t.Error("expanded message missing mode")
	}
	if !strings.Contains(msg, "out success") {
		t.Error("expanded message missing outcome")
	}
}

func TestGitManager_CommitAndPush_SkipsWhenDisabled(t *testing.T) {
	var bashCalled bool
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			bashCalled = true
			return &tool.Result{Content: "ok", IsError: false}, nil
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	config := &GitConfig{Enabled: false}
	gm := NewGitManager(registry, config, logging.NewNopLogger())

	gm.CommitAndPush(context.Background(), 1, "TEST", "test")

	if bashCalled {
		t.Error("bash should not be called when git is disabled")
	}
}

func TestGitManager_CommitAndPush_SkipsWhenNotGitRepo(t *testing.T) {
	var bashCalled bool
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			bashCalled = true
			// First call is IsGitRepo check - return error
			return &tool.Result{Content: "", IsError: true}, fmt.Errorf("not a git repo")
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	config := &GitConfig{Enabled: true}
	gm := NewGitManager(registry, config, logging.NewNopLogger())

	gm.CommitAndPush(context.Background(), 1, "TEST", "test")

	// Should have called bash once for IsGitRepo check, but not for git operations
	if !bashCalled {
		t.Error("bash should be called for IsGitRepo check")
	}
}

func TestGitManager_GitFailuresAreNonFatal(t *testing.T) {
	callCount := 0
	bashTool := &mockTool{
		name: "bash",
		execFunc: func(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
			callCount++
			if callCount == 1 {
				// IsGitRepo succeeds
				return &tool.Result{Content: "ok", IsError: false}, nil
			}
			// All other git commands fail
			return &tool.Result{Content: "", IsError: true}, fmt.Errorf("git error")
		},
	}

	registry := tool.NewRegistry()
	registry.Register(bashTool)

	config := &GitConfig{Enabled: true}
	gm := NewGitManager(registry, config, logging.NewNopLogger())

	// Should not panic or return error
	gm.CommitAndPush(context.Background(), 1, "TEST", "test")

	if callCount < 2 {
		t.Error("expected multiple bash calls despite failures")
	}
}
