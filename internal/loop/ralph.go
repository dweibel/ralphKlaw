package loop

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/eachlabs/klaw/pkg/channel"
	"github.com/eachlabs/ralphklaw/internal/git"
	"github.com/eachlabs/ralphklaw/internal/logging"
	"github.com/eachlabs/ralphklaw/internal/state"
	"github.com/eachlabs/ralphklaw/internal/validation"
)

// InnerLoopRunner abstracts InnerLoop.Execute for testing.
type InnerLoopRunner interface {
	Execute(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Validator abstracts Validator.Validate for testing.
type Validator interface {
	Validate(ctx context.Context) error
}

// StateManagerIface abstracts StateManager for testing.
type StateManagerIface interface {
	DetectMode(ctx context.Context) state.Mode
	ReadTODO(ctx context.Context) ([]state.Task, error)
	WriteTODO(ctx context.Context, tasks []state.Task) error
	ReadError(ctx context.Context) (*state.ValidationError, error)
	WriteError(ctx context.Context, ve *state.ValidationError) error
	DeleteError(ctx context.Context) error
}

// GitManagerIface abstracts GitManager for testing.
type GitManagerIface interface {
	SetupBranch(ctx context.Context) error
	CommitAndPush(ctx context.Context, iteration int, mode, outcome string)
}

// RalphLoop is the outer loop controller.
type RalphLoop struct {
	workspace      string
	inner          InnerLoopRunner
	validator      Validator
	state          StateManagerIface
	git            GitManagerIface
	channel        channel.Channel
	maxIterations  int
	maxFixAttempts int
	logger         *logging.Logger
	iteration      int
}

// NewRalphLoop creates a RalphLoop with concrete dependencies.
func NewRalphLoop(
	workspace string,
	inner *InnerLoop,
	validator *validation.Validator,
	sm *state.StateManager,
	gm *git.GitManager,
	ch channel.Channel,
	maxIterations int,
	maxFixAttempts int,
	logger *logging.Logger,
) *RalphLoop {
	return &RalphLoop{
		workspace:      workspace,
		inner:          inner,
		validator:      validator,
		state:          sm,
		git:            gm,
		channel:        ch,
		maxIterations:  maxIterations,
		maxFixAttempts: maxFixAttempts,
		logger:         logger,
	}
}

// NewRalphLoopWithInterfaces creates a RalphLoop with interface-based dependencies (for testing).
func NewRalphLoopWithInterfaces(
	workspace string,
	inner InnerLoopRunner,
	validator Validator,
	sm StateManagerIface,
	gm GitManagerIface,
	ch channel.Channel,
	maxIterations int,
	maxFixAttempts int,
	logger *logging.Logger,
) *RalphLoop {
	return &RalphLoop{
		workspace:      workspace,
		inner:          inner,
		validator:      validator,
		state:          sm,
		git:            gm,
		channel:        ch,
		maxIterations:  maxIterations,
		maxFixAttempts: maxFixAttempts,
		logger:         logger,
	}
}

// Run executes the Ralph Loop until TODO.md is empty or limits are reached.
func (r *RalphLoop) Run(ctx context.Context) error {
	if err := r.git.SetupBranch(ctx); err != nil {
		r.logger.Warn("git branch setup failed: %v", err)
	}

	for r.iteration < r.maxIterations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.iteration++
		r.logger.WithFields(logging.LevelInfo, "iteration starting", map[string]interface{}{
			"iteration": r.iteration,
		})

		mode := r.state.DetectMode(ctx)
		r.logger.WithFields(logging.LevelInfo, "mode detected", map[string]interface{}{
			"iteration": r.iteration,
			"mode":      mode.String(),
		})
		r.channel.Send(ctx, progressMsg("Iteration %d — mode: %s", r.iteration, mode))

		var iterationOutcome string

		switch mode {
		case state.ModeExecuteTask:
			done, outcome, err := r.executeTask(ctx)
			iterationOutcome = outcome
			if err != nil {
				r.git.CommitAndPush(ctx, r.iteration, mode.String(), "error: "+err.Error())
				return err
			}
			if done {
				r.git.CommitAndPush(ctx, r.iteration, mode.String(), "all tasks completed")
				r.channel.Send(ctx, completionMsg("All tasks completed"))
				return nil
			}

		case state.ModeFixError:
			outcome, err := r.fixError(ctx)
			iterationOutcome = outcome
			if err != nil {
				r.git.CommitAndPush(ctx, r.iteration, mode.String(), "error: "+err.Error())
				return err
			}
		}

		r.git.CommitAndPush(ctx, r.iteration, mode.String(), iterationOutcome)
	}

	r.git.CommitAndPush(ctx, r.iteration, "MAX_ITERATIONS", "limit reached")
	r.channel.Send(ctx, errorMsg("Max iterations (%d) reached", r.maxIterations))
	return ErrMaxIterationsReached
}

func (r *RalphLoop) executeTask(ctx context.Context) (bool, string, error) {
	tasks, err := r.state.ReadTODO(ctx)
	if err != nil {
		return false, "", fmt.Errorf("failed to read TODO.md: %w", err)
	}

	var task *state.Task
	for i := range tasks {
		if !tasks[i].Completed {
			task = &tasks[i]
			break
		}
	}

	if task == nil {
		return true, "all tasks complete", nil
	}

	r.channel.Send(ctx, progressMsg("Task: %s", task.Description))

	r.logger.WithFields(logging.LevelInfo, "executing task", map[string]interface{}{
		"task":      task.Description,
		"iteration": r.iteration,
	})

	prompt := BuildTaskPrompt(task, r.workspace)
	_, err = r.inner.Execute(ctx, SystemPrompt(), prompt)
	if err != nil {
		return false, "", fmt.Errorf("inner loop failed: %w", err)
	}

	if err := r.validator.Validate(ctx); err != nil {
		r.logger.WithFields(logging.LevelInfo, "validation result", map[string]interface{}{
			"passed":    false,
			"iteration": r.iteration,
		})
		r.channel.Send(ctx, errorMsg("Validation failed: %s", err))
		writeErr := r.state.WriteError(ctx, &state.ValidationError{
			Iteration: r.iteration,
			Task:      task.Description,
			Error:     err.Error(),
			Attempt:   1,
		})
		return false, fmt.Sprintf("task failed validation: %s", task.Description), writeErr
	}

	r.logger.WithFields(logging.LevelInfo, "validation result", map[string]interface{}{
		"passed":    true,
		"iteration": r.iteration,
	})
	r.channel.Send(ctx, progressMsg("Validation passed"))

	task.Completed = true
	if err := r.state.WriteTODO(ctx, tasks); err != nil {
		return false, "", err
	}

	return false, fmt.Sprintf("task completed: %s", task.Description), nil
}

func (r *RalphLoop) fixError(ctx context.Context) (string, error) {
	errorInfo, err := r.state.ReadError(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read LAST_ERROR.txt: %w", err)
	}

	if errorInfo.Attempt >= r.maxFixAttempts {
		r.channel.Send(ctx, errorMsg(
			"Max fix attempts (%d) reached for: %s",
			r.maxFixAttempts, errorInfo.Task,
		))
		return fmt.Sprintf("max fix attempts reached: %s", errorInfo.Task), ErrMaxFixAttemptsReached
	}

	r.channel.Send(ctx, progressMsg(
		"Fixing error (attempt %d/%d): %s",
		errorInfo.Attempt, r.maxFixAttempts, errorInfo.Task,
	))

	r.logger.WithFields(logging.LevelInfo, "fix attempt", map[string]interface{}{
		"attempt": errorInfo.Attempt,
		"max":     r.maxFixAttempts,
		"task":    errorInfo.Task,
	})

	prompt := BuildFixPrompt(errorInfo)
	_, err = r.inner.Execute(ctx, SystemPrompt(), prompt)
	if err != nil {
		return "", fmt.Errorf("inner loop failed: %w", err)
	}

	if err := r.validator.Validate(ctx); err != nil {
		r.logger.WithFields(logging.LevelInfo, "validation result", map[string]interface{}{
			"passed":    false,
			"iteration": r.iteration,
		})
		r.channel.Send(ctx, errorMsg("Fix validation failed: %s", err))
		errorInfo.Attempt++
		errorInfo.Error = err.Error()
		writeErr := r.state.WriteError(ctx, errorInfo)
		return fmt.Sprintf("fix attempt %d failed: %s", errorInfo.Attempt-1, errorInfo.Task), writeErr
	}

	r.logger.WithFields(logging.LevelInfo, "validation result", map[string]interface{}{
		"passed":    true,
		"iteration": r.iteration,
	})
	r.channel.Send(ctx, progressMsg("Fix validated successfully"))

	if err := r.state.DeleteError(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("fix succeeded: %s", errorInfo.Task), nil
}

// RunWithShutdown wraps Run with graceful SIGINT/SIGTERM handling.
func (r *RalphLoop) RunWithShutdown(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		r.logger.Info("shutdown signal received, finishing current iteration...")
		cancel()
	}()

	return r.Run(ctx)
}
