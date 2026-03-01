package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

// Feature: ralphklaw, Property 1: Config round-trip consistency
// Validates: Requirements 12.1, 12.2, 12.3
func TestProperty_ConfigRoundTripConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid Config
		cfg := &Config{
			Version: rapid.StringMatching(`^[0-9]+\.[0-9]+$`).Draw(t, "version"),
			Loop: LoopConfig{
				MaxIterations:  rapid.IntRange(1, 1000).Draw(t, "max_iterations"),
				MaxFixAttempts: rapid.IntRange(1, 100).Draw(t, "max_fix_attempts"),
			},
			InnerLoop: InnerLoopConfig{
				MaxRounds:   rapid.IntRange(1, 100).Draw(t, "max_rounds"),
				Model:       rapid.StringMatching(`^[a-zA-Z0-9_-]+$`).Draw(t, "model"),
				Temperature: rapid.Float64Range(0, 2).Draw(t, "temperature"),
				MaxTokens:   rapid.IntRange(100, 100000).Draw(t, "max_tokens"),
			},
			Validation: ValidationConfig{
				Command: rapid.StringMatching(`^[a-zA-Z0-9 ./&|_-]+$`).Draw(t, "command"),
			},
			Git: GitConfig{
				Enabled:        rapid.Bool().Draw(t, "enabled"),
				AutoPush:       rapid.Bool().Draw(t, "auto_push"),
				AuthorName:     rapid.StringMatching(`^[a-zA-Z0-9 _-]*$`).Draw(t, "author_name"),
				AuthorEmail:    rapid.StringMatching(`^[a-zA-Z0-9@._-]*$`).Draw(t, "author_email"),
				CommitTemplate: rapid.StringMatching(`^[a-zA-Z0-9 {}:_-]*$`).Draw(t, "commit_template"),
				CreateBranch:   rapid.Bool().Draw(t, "create_branch"),
				BranchPattern:  rapid.StringMatching(`^[a-zA-Z0-9/{}_-]*$`).Draw(t, "branch_pattern"),
				RemoteName:     rapid.StringMatching(`^[a-zA-Z0-9_-]*$`).Draw(t, "remote_name"),
			},
			Logging: LoggingConfig{
				Level: rapid.SampledFrom([]string{"debug", "info", "warn", "error"}).Draw(t, "level"),
				File:  rapid.StringMatching(`^[a-zA-Z0-9./_-]+$`).Draw(t, "file"),
			},
		}

		// Marshal to YAML
		data, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		// Write to temp file
		tmpDir, err := os.MkdirTemp("", "ralphklaw-test-*")
		if err != nil {
			t.Fatalf("MkdirTemp failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		configPath := filepath.Join(tmpDir, ".klaw", "agents", "ralphklaw.yaml")
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		// Load via LoadConfig
		loaded, err := LoadConfig(tmpDir)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Apply defaults to original for comparison
		cfg = cfg.withDefaults()

		// Compare (after defaults applied, should be equivalent)
		if loaded.Version != cfg.Version {
			t.Fatalf("version mismatch: got %q, want %q", loaded.Version, cfg.Version)
		}
		if loaded.Loop.MaxIterations != cfg.Loop.MaxIterations {
			t.Fatalf("max_iterations mismatch: got %d, want %d", loaded.Loop.MaxIterations, cfg.Loop.MaxIterations)
		}
		if loaded.Loop.MaxFixAttempts != cfg.Loop.MaxFixAttempts {
			t.Fatalf("max_fix_attempts mismatch: got %d, want %d", loaded.Loop.MaxFixAttempts, cfg.Loop.MaxFixAttempts)
		}
		if loaded.InnerLoop.MaxRounds != cfg.InnerLoop.MaxRounds {
			t.Fatalf("max_rounds mismatch: got %d, want %d", loaded.InnerLoop.MaxRounds, cfg.InnerLoop.MaxRounds)
		}
		if loaded.InnerLoop.Model != cfg.InnerLoop.Model {
			t.Fatalf("model mismatch: got %q, want %q", loaded.InnerLoop.Model, cfg.InnerLoop.Model)
		}
		if loaded.InnerLoop.Temperature != cfg.InnerLoop.Temperature {
			t.Fatalf("temperature mismatch: got %f, want %f", loaded.InnerLoop.Temperature, cfg.InnerLoop.Temperature)
		}
		if loaded.InnerLoop.MaxTokens != cfg.InnerLoop.MaxTokens {
			t.Fatalf("max_tokens mismatch: got %d, want %d", loaded.InnerLoop.MaxTokens, cfg.InnerLoop.MaxTokens)
		}
	})
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".klaw", "agents", "ralphklaw.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := `version: "1.0"
loop:
  max_iterations: 100
  max_fix_attempts: 5
inner_loop:
  max_rounds: 30
  model: "test-model"
  temperature: 0.5
  max_tokens: 4096
validation:
  command: "go test ./..."
git:
  enabled: true
  auto_push: false
logging:
  level: "debug"
  file: "test.log"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Version != "1.0" {
		t.Errorf("version = %q, want %q", cfg.Version, "1.0")
	}
	if cfg.Loop.MaxIterations != 100 {
		t.Errorf("max_iterations = %d, want %d", cfg.Loop.MaxIterations, 100)
	}
	if cfg.InnerLoop.Model != "test-model" {
		t.Errorf("model = %q, want %q", cfg.InnerLoop.Model, "test-model")
	}
}

func TestLoadConfig_MissingFile_ReturnsDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	defaults := DefaultConfig()
	if cfg.Loop.MaxIterations != defaults.Loop.MaxIterations {
		t.Errorf("max_iterations = %d, want %d", cfg.Loop.MaxIterations, defaults.Loop.MaxIterations)
	}
	if cfg.InnerLoop.Model != defaults.InnerLoop.Model {
		t.Errorf("model = %q, want %q", cfg.InnerLoop.Model, defaults.InnerLoop.Model)
	}
}

func TestLoadConfig_InvalidYAML_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".klaw", "agents", "ralphklaw.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestWithDefaults_FillsZeroValues(t *testing.T) {
	cfg := &Config{
		Version: "1.0",
		// Leave other fields zero
	}

	cfg = cfg.withDefaults()

	defaults := DefaultConfig()
	if cfg.Loop.MaxIterations != defaults.Loop.MaxIterations {
		t.Errorf("max_iterations not filled: got %d, want %d", cfg.Loop.MaxIterations, defaults.Loop.MaxIterations)
	}
	if cfg.InnerLoop.Model != defaults.InnerLoop.Model {
		t.Errorf("model not filled: got %q, want %q", cfg.InnerLoop.Model, defaults.InnerLoop.Model)
	}
	if cfg.Validation.Command != defaults.Validation.Command {
		t.Errorf("command not filled: got %q, want %q", cfg.Validation.Command, defaults.Validation.Command)
	}
}

func TestDefaultConfig_HasSensibleValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Loop.MaxIterations <= 0 {
		t.Errorf("max_iterations should be positive, got %d", cfg.Loop.MaxIterations)
	}
	if cfg.Loop.MaxFixAttempts <= 0 {
		t.Errorf("max_fix_attempts should be positive, got %d", cfg.Loop.MaxFixAttempts)
	}
	if cfg.InnerLoop.MaxRounds <= 0 {
		t.Errorf("max_rounds should be positive, got %d", cfg.InnerLoop.MaxRounds)
	}
	if cfg.InnerLoop.Model == "" {
		t.Error("model should not be empty")
	}
	if cfg.InnerLoop.MaxTokens <= 0 {
		t.Errorf("max_tokens should be positive, got %d", cfg.InnerLoop.MaxTokens)
	}
	if cfg.Validation.Command == "" {
		t.Error("validation command should not be empty")
	}
	if cfg.Logging.Level == "" {
		t.Error("log level should not be empty")
	}
	if cfg.Logging.File == "" {
		t.Error("log file should not be empty")
	}
}
