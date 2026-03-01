package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	klawchannel "github.com/eachlabs/klaw/pkg/channel"
	klawprovider "github.com/eachlabs/klaw/pkg/provider"
	klawtool "github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/config"
	"github.com/eachlabs/ralphklaw/internal/git"
	"github.com/eachlabs/ralphklaw/internal/logging"
	"github.com/eachlabs/ralphklaw/internal/loop"
	"github.com/eachlabs/ralphklaw/internal/state"
	"github.com/eachlabs/ralphklaw/internal/validation"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace directory")
	initFlag := flag.Bool("init", false, "initialize workspace")
	flag.Parse()

	// --init path: unchanged
	if *initFlag {
		if err := loop.Initialize(*workspace); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Workspace initialized. Edit TODO.md and run ralphklaw.")
		return
	}

	// Load ralphKlaw config
	cfg, err := config.LoadConfig(*workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logger
	logFile := cfg.Logging.File
	if logFile != "" && logFile[0] != '/' {
		logFile = *workspace + "/" + logFile
	}
	logger, err := logging.NewLogger(cfg.Logging.Level, logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Build provider from ANTHROPIC_API_KEY env var
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is required")
		os.Exit(1)
	}
	prov, err := klawprovider.NewAnthropic(klawprovider.AnthropicConfig{
		APIKey: apiKey,
		Model:  cfg.InnerLoop.Model,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider error: %v\n", err)
		os.Exit(1)
	}

	// Build tool registry with standard klaw tools rooted at workspace
	tools := klawtool.DefaultRegistry(*workspace)

	// Build terminal channel for CLI output
	ch := klawchannel.NewTerminal()

	// Wire up components
	sm := state.NewStateManager(*workspace, tools)

	validator := validation.NewValidator(tools, cfg.Validation.Command, logger)

	gitCfg := &git.GitConfig{
		Enabled:        cfg.Git.Enabled,
		AutoPush:       cfg.Git.AutoPush,
		AuthorName:     cfg.Git.AuthorName,
		AuthorEmail:    cfg.Git.AuthorEmail,
		CommitTemplate: cfg.Git.CommitTemplate,
		CreateBranch:   cfg.Git.CreateBranch,
		BranchPattern:  cfg.Git.BranchPattern,
		RemoteName:     cfg.Git.RemoteName,
	}
	gm := git.NewGitManager(tools, gitCfg, logger)

	innerCfg := &loop.InnerLoopConfig{
		MaxRounds:   cfg.InnerLoop.MaxRounds,
		MaxTokens:   cfg.InnerLoop.MaxTokens,
		Temperature: cfg.InnerLoop.Temperature,
		Model:       cfg.InnerLoop.Model,
	}
	inner := loop.NewInnerLoop(prov, tools, innerCfg, logger)

	ralph := loop.NewRalphLoop(
		*workspace,
		inner,
		validator,
		sm,
		gm,
		ch,
		cfg.Loop.MaxIterations,
		cfg.Loop.MaxFixAttempts,
		logger,
	)

	ctx := context.Background()
	if err := ralph.RunWithShutdown(ctx); err != nil {
		logger.Error("ralph loop exited: %v", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
