package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eachlabs/ralphklaw/internal/config"
	"gopkg.in/yaml.v3"
)

// Initialize sets up a workspace with default ralphKlaw files.
func Initialize(workspace string) error {
	// Create directories
	if err := os.MkdirAll(filepath.Join(workspace, ".klaw", "agents"), 0755); err != nil {
		return fmt.Errorf("create .klaw/agents: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".klaw", "logs"), 0755); err != nil {
		return fmt.Errorf("create .klaw/logs: %w", err)
	}

	// Write default config if missing
	configPath := filepath.Join(workspace, ".klaw", "agents", "ralphklaw.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	// Write TODO.md template if missing
	todoPath := filepath.Join(workspace, "TODO.md")
	if _, err := os.Stat(todoPath); os.IsNotExist(err) {
		content := "# TODO\n\n- [ ] Example: implement feature X\n- [ ] Example: add tests for feature X\n"
		if err := os.WriteFile(todoPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write TODO.md: %w", err)
		}
	}

	// Update .gitignore
	gitignorePath := filepath.Join(workspace, ".gitignore")
	if data, err := os.ReadFile(gitignorePath); err == nil {
		if !strings.Contains(string(data), "LAST_ERROR.txt") {
			if err := os.WriteFile(gitignorePath, append(data, []byte("\nLAST_ERROR.txt\n")...), 0644); err != nil {
				return fmt.Errorf("update .gitignore: %w", err)
			}
		}
	} else if os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte("LAST_ERROR.txt\n"), 0644); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	}

	return nil
}
