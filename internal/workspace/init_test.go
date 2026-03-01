package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eachlabs/ralphklaw/internal/workspace"
)

// TestInitialize_CreatesDirectories verifies that .klaw/agents and .klaw/logs
// are created when they don't exist.
func TestInitialize_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	for _, sub := range []string{".klaw/agents", ".klaw/logs"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}
}

// TestInitialize_WritesDefaultConfig verifies that ralphklaw.yaml is written
// with default content when it doesn't exist.
func TestInitialize_WritesDefaultConfig(t *testing.T) {
	dir := t.TempDir()

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	configPath := filepath.Join(dir, ".klaw", "agents", "ralphklaw.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected config file to have content")
	}
	// Should contain recognizable YAML keys from DefaultConfig
	content := string(data)
	if !strings.Contains(content, "version") {
		t.Error("expected config to contain 'version' key")
	}
	if !strings.Contains(content, "loop") {
		t.Error("expected config to contain 'loop' key")
	}
}

// TestInitialize_DoesNotOverwriteExistingConfig verifies that an existing
// ralphklaw.yaml is not overwritten.
func TestInitialize_DoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()

	// Pre-create the config directory and file with custom content
	agentsDir := filepath.Join(dir, ".klaw", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(agentsDir, "ralphklaw.yaml")
	original := []byte("# my custom config\nversion: \"custom\"\n")
	if err := os.WriteFile(configPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Errorf("config was overwritten; got %q, want %q", string(data), string(original))
	}
}

// TestInitialize_CreatesTODOMd verifies that TODO.md is created with a
// template when it doesn't exist.
func TestInitialize_CreatesTODOMd(t *testing.T) {
	dir := t.TempDir()

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	todoPath := filepath.Join(dir, "TODO.md")
	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("expected TODO.md to exist: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# TODO") {
		t.Error("expected TODO.md to contain '# TODO' header")
	}
	if !strings.Contains(content, "- [ ]") {
		t.Error("expected TODO.md to contain at least one task item")
	}
}

// TestInitialize_DoesNotOverwriteExistingTODO verifies that an existing
// TODO.md is not overwritten.
func TestInitialize_DoesNotOverwriteExistingTODO(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "TODO.md")
	original := []byte("# My Tasks\n\n- [ ] do the thing\n")
	if err := os.WriteFile(todoPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Errorf("TODO.md was overwritten; got %q, want %q", string(data), string(original))
	}
}

// TestInitialize_AppendsGitignore verifies that LAST_ERROR.txt is appended to
// .gitignore when it's not already present.
func TestInitialize_AppendsGitignore(t *testing.T) {
	dir := t.TempDir()

	// Create a .gitignore without LAST_ERROR.txt
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*.log\nbuild/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "LAST_ERROR.txt") {
		t.Error("expected .gitignore to contain LAST_ERROR.txt after initialization")
	}
	// Original content should still be present
	if !strings.Contains(string(data), "*.log") {
		t.Error("expected original .gitignore content to be preserved")
	}
}

// TestInitialize_DoesNotDuplicateGitignoreEntry verifies that LAST_ERROR.txt
// is not added to .gitignore if it's already present.
func TestInitialize_DoesNotDuplicateGitignoreEntry(t *testing.T) {
	dir := t.TempDir()

	gitignorePath := filepath.Join(dir, ".gitignore")
	original := "*.log\nLAST_ERROR.txt\nbuild/\n"
	if err := os.WriteFile(gitignorePath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(string(data), "LAST_ERROR.txt")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of LAST_ERROR.txt in .gitignore, got %d", count)
	}
}

// TestInitialize_CreatesGitignoreWhenMissing verifies that .gitignore is
// created with LAST_ERROR.txt when it doesn't exist at all.
func TestInitialize_CreatesGitignoreWhenMissing(t *testing.T) {
	dir := t.TempDir()

	if err := workspace.Initialize(dir); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected .gitignore to be created: %v", err)
	}
	if !strings.Contains(string(data), "LAST_ERROR.txt") {
		t.Error("expected newly created .gitignore to contain LAST_ERROR.txt")
	}
}
