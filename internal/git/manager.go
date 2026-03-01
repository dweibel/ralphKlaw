// Package git manages git operations for ralphKlaw.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
)

// GitConfig controls git behavior.
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

// GitManager handles git commit, push, and branch operations.
type GitManager struct {
	tools  *tool.Registry
	config *GitConfig
	logger *logging.Logger
}

// NewGitManager creates a GitManager.
func NewGitManager(tools *tool.Registry, config *GitConfig, logger *logging.Logger) *GitManager {
	return &GitManager{tools: tools, config: config, logger: logger}
}

// IsGitRepo checks if the workspace is a git repository.
func (g *GitManager) IsGitRepo(ctx context.Context) bool {
	return g.bash(ctx, "git rev-parse --git-dir") == nil
}

// SetupBranch creates and pushes a new branch if configured.
func (g *GitManager) SetupBranch(ctx context.Context) error {
	if !g.config.CreateBranch || !g.IsGitRepo(ctx) {
		return nil
	}

	branch := g.generateBranchName()
	g.logger.Info("creating branch: %s", branch)
	if err := g.bash(ctx, fmt.Sprintf("git checkout -b %s", branch)); err != nil {
		return err
	}

	remote := g.remoteName()
	if err := g.bash(ctx, fmt.Sprintf("git push -u %s %s", remote, branch)); err != nil {
		g.logger.Warn("initial branch push failed: %v", err)
	}

	return nil
}

// CommitAndPush stages, commits, and pushes all changes. Git failures are non-fatal.
func (g *GitManager) CommitAndPush(ctx context.Context, iteration int, mode, outcome string) {
	if !g.config.Enabled || !g.IsGitRepo(ctx) {
		return
	}

	if err := g.bash(ctx, "git add -A"); err != nil {
		g.logger.Warn("git add failed: %v", err)
		return
	}

	msg := fmt.Sprintf("ralphKlaw: iteration %d [%s] %s", iteration, mode, outcome)
	if g.config.CommitTemplate != "" {
		msg = g.ExpandTemplate(g.config.CommitTemplate, iteration, mode, outcome)
	}

	cmd := fmt.Sprintf("git commit -m %q --allow-empty", msg)
	if g.config.AuthorName != "" && g.config.AuthorEmail != "" {
		cmd = fmt.Sprintf("git -c user.name=%q -c user.email=%q commit -m %q --allow-empty",
			g.config.AuthorName, g.config.AuthorEmail, msg)
	}

	if err := g.bash(ctx, cmd); err != nil {
		g.logger.Warn("git commit failed: %v", err)
		return
	}

	g.logger.Info("committed: %s", msg)

	remote := g.remoteName()
	if err := g.bash(ctx, fmt.Sprintf("git push %s HEAD", remote)); err != nil {
		g.logger.Warn("git push failed (commit is local): %v", err)
	}
}

// ExpandTemplate replaces placeholders in a commit message template.
func (g *GitManager) ExpandTemplate(tmpl string, iteration int, mode, outcome string) string {
	r := strings.NewReplacer(
		"{iteration}", fmt.Sprintf("%d", iteration),
		"{mode}", mode,
		"{outcome}", outcome,
	)
	return r.Replace(tmpl)
}

func (g *GitManager) remoteName() string {
	if g.config.RemoteName != "" {
		return g.config.RemoteName
	}
	return "origin"
}

// GenerateBranchName creates a branch name from the configured pattern.
func (g *GitManager) generateBranchName() string {
	if g.config.BranchPattern != "" {
		return strings.ReplaceAll(g.config.BranchPattern, "{timestamp}",
			time.Now().Format("20060102-150405"))
	}
	return fmt.Sprintf("ralphklaw/%s", time.Now().Format("20060102-150405"))
}

func (g *GitManager) bash(ctx context.Context, cmd string) error {
	b, ok := g.tools.Get("bash")
	if !ok {
		return fmt.Errorf("bash tool not available")
	}
	params, _ := json.Marshal(map[string]string{"command": cmd})
	result, err := b.Execute(ctx, params)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("%s", result.Content)
	}
	return nil
}
