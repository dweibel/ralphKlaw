// Package config handles loading and managing ralphKlaw configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for ralphKlaw.
type Config struct {
	Version    string           `yaml:"version"`
	Loop       LoopConfig       `yaml:"loop"`
	InnerLoop  InnerLoopConfig  `yaml:"inner_loop"`
	Validation ValidationConfig `yaml:"validation"`
	Git        GitConfig        `yaml:"git"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// LoopConfig controls the outer Ralph Loop.
type LoopConfig struct {
	MaxIterations  int `yaml:"max_iterations"`
	MaxFixAttempts int `yaml:"max_fix_attempts"`
}

// InnerLoopConfig controls the inner agentic loop.
type InnerLoopConfig struct {
	MaxRounds   int     `yaml:"max_rounds"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

// ValidationConfig controls the validation step.
type ValidationConfig struct {
	Command string `yaml:"command"`
}

// GitConfig controls git operations.
type GitConfig struct {
	Enabled        bool   `yaml:"enabled"`
	AutoPush       bool   `yaml:"auto_push"`
	AuthorName     string `yaml:"author_name"`
	AuthorEmail    string `yaml:"author_email"`
	CommitTemplate string `yaml:"commit_template"`
	CreateBranch   bool   `yaml:"create_branch"`
	BranchPattern  string `yaml:"branch_pattern"`
	RemoteName     string `yaml:"remote_name"`
}

// LoggingConfig controls structured logging.
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// LoadConfig reads configuration from the workspace, returning defaults if the file is missing.
func LoadConfig(workspace string) (*Config, error) {
	path := filepath.Join(workspace, ".klaw", "agents", "ralphklaw.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg.withDefaults(), nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: "1.0",
		Loop: LoopConfig{
			MaxIterations:  50,
			MaxFixAttempts: 3,
		},
		InnerLoop: InnerLoopConfig{
			MaxRounds:   20,
			Model:       "claude-sonnet-4-20250514",
			Temperature: 0.0,
			MaxTokens:   8192,
		},
		Validation: ValidationConfig{
			Command: "go build ./... && go vet ./...",
		},
		Git: GitConfig{
			Enabled:        true,
			AutoPush:       true,
			CommitTemplate: "ralphKlaw: iteration {iteration} [{mode}] {outcome}",
			RemoteName:     "origin",
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  ".klaw/logs/ralphklaw.log",
		},
	}
}

func (c *Config) withDefaults() *Config {
	d := DefaultConfig()
	if c.Loop.MaxIterations == 0 {
		c.Loop.MaxIterations = d.Loop.MaxIterations
	}
	if c.Loop.MaxFixAttempts == 0 {
		c.Loop.MaxFixAttempts = d.Loop.MaxFixAttempts
	}
	if c.InnerLoop.MaxRounds == 0 {
		c.InnerLoop.MaxRounds = d.InnerLoop.MaxRounds
	}
	if c.InnerLoop.Model == "" {
		c.InnerLoop.Model = d.InnerLoop.Model
	}
	if c.InnerLoop.MaxTokens == 0 {
		c.InnerLoop.MaxTokens = d.InnerLoop.MaxTokens
	}
	if c.Validation.Command == "" {
		c.Validation.Command = d.Validation.Command
	}
	if c.Logging.Level == "" {
		c.Logging.Level = d.Logging.Level
	}
	if c.Logging.File == "" {
		c.Logging.File = d.Logging.File
	}
	return c
}
