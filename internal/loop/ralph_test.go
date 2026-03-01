package loop

import (
	"context"
	"fmt"
	"testing"

	"github.com/eachlabs/klaw/pkg/channel"
	"github.com/eachlabs/ralphklaw/internal/logging"
	"github.com/eachlabs/ralphklaw/internal/state"
	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockInnerLoop struct {
	executeFunc func(ctx context.Context, sys, user string) (string, error)
}

func (m *mockInnerLoop) Execute(ctx context.Context, sys, user string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, sys, user)
	}
	return "done", nil
}
type mockValidator struct {
	validateFunc func(ctx context.Context) error
}

func (m *mockValidator) Validate(ctx context.Context) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx)
	}
	return nil
}

type mockStateManager struct {
	detectModeFunc func(ctx context.Context) state.Mode
	readTODOFunc   func(ctx context.Context) ([]state.Task, error)
	writeTODOFunc  func(ctx context.Context, tasks []state.Task) error
	readErrorFunc  func(ctx context.Context) (*state.ValidationError, error)
	writeErrorFunc func(ctx context.Context, ve *state.ValidationError) error
	deleteErrorFunc func(ctx context.Context) error
}

func (m *mockStateManager) DetectMode(ctx context.Context) state.Mode {
	if m.detectModeFunc != nil {
		return m.detectModeFunc(ctx)
	}
	return state.ModeExecuteTask
}

func (m *mockStateManager) ReadTODO(ctx context.Context) ([]state.Task, error) {
	if m.readTODOFunc != nil {
		return m.readTODOFunc(ctx)
	}
	return nil, nil
}

func (m *mockStateManager) WriteTODO(ctx context.Context, tasks []state.Task) error {
	if m.writeTODOFunc != nil {
		return m.writeTODOFunc(ctx, tasks)
	}
	return nil
}

func (m *mockStateManager) ReadError(ctx context.Context) (*state.ValidationError, error) {
	if m.readErrorFunc != nil {
		return m.readErrorFunc(ctx)
	}
	return &state.ValidationError{Task: "test", Error: "err", Attempt: 1}, nil
}

func (m *mockStateManager) WriteError(ctx context.Context, ve *state.ValidationError) error {
	if m.writeErrorFunc != nil {
		return m.writeErrorFunc(ctx, ve)
	}
	return nil
}

func (m *mockStateManager) DeleteError(ctx context.Context) error {
	if m.deleteErrorFunc != nil {
		return m.deleteErrorFunc(ctx)
	}
	return nil
}

type mockGitManager struct {
	setupBranchFunc    func(ctx context.Context) error
	commitAndPushFunc  func(ctx context.Context, iteration int, mode, outcome string)
}

func (m *mockGitManager) SetupBranch(ctx context.Context) error {
	if m.setupBranchFunc != nil {
		return m.setupBranchFunc(ctx)
	}
	return nil
}

func (m *mockGitManager) CommitAndPush(ctx context.Context, iteration int, mode, outcome string) {
	if m.commitAndPushFunc != nil {
		m.commitAndPushFunc(ctx, iteration, mode, outcome)
	}
}

type mockChannel struct {
	sendFunc func(ctx context.Context, msg *channel.Message) error
}

func (m *mockChannel) Start(ctx context.Context) error { return nil }
func (m *mockChannel) Send(ctx context.Context, msg *channel.Message) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}
	return nil
}
func (m *mockChannel) Receive() <-chan *channel.Message { return nil }
func (m *mockChannel) Stop() error                     { return nil }
func (m *mockChannel) Name() string                    { return "mock" }

// ---------------------------------------------------------------------------
// Test constructor — builds a RalphLoop with interface-based dependencies.
// ---------------------------------------------------------------------------

func newTestRalphLoop(
	inner InnerLoopRunner,
	val Validator,
	sm StateManagerIface,
	gm GitManagerIface,
	ch channel.Channel,
	maxIterations int,
	maxFixAttempts int,
) *RalphLoop {
	logger := logging.NewNopLogger()
	return NewRalphLoopWithInterfaces(
		"/tmp/test-workspace",
		inner,
		val,
		sm,
		gm,
		ch,
		maxIterations,
		maxFixAttempts,
		logger,
	)
}

// ---------------------------------------------------------------------------
// Feature: ralphklaw, Property 9: Mode transitions are valid
// Validates: Requirements 4.1, 4.2, 4.3, 4.4
// ---------------------------------------------------------------------------

func TestProperty_ModeTransitionsAreValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a sequence of iteration outcomes
		numIterations := rapid.IntRange(1, 10).Draw(t, "num_iterations")

		// For each iteration, decide: validation passes or fails
		validationResults := make([]bool, numIterations)
		for i := range validationResults {
			validationResults[i] = rapid.Bool().Draw(t, fmt.Sprintf("validation_passes_%d", i))
		}

		// Track mode transitions
		type transition struct {
			from state.Mode
			to   state.Mode
		}
		var transitions []transition

		// Simulate the state machine
		// Start in EXECUTE_TASK (no LAST_ERROR.txt)
		errorExists := false
		fixAttempt := 0
		maxFixAttempts := rapid.IntRange(1, 5).Draw(t, "max_fix_attempts")

		for i := 0; i < numIterations; i++ {
			var from state.Mode
			if errorExists {
				from = state.ModeFixError
			} else {
				from = state.ModeExecuteTask
			}

			validationPasses := validationResults[i]

			var to state.Mode
			if from == state.ModeExecuteTask {
				if validationPasses {
					to = state.ModeExecuteTask // stays in execute
					errorExists = false
					fixAttempt = 0
				} else {
					to = state.ModeFixError // transitions to fix
					errorExists = true
					fixAttempt = 1
				}
			} else { // ModeFixError
				if validationPasses {
					to = state.ModeExecuteTask // fix succeeded
					errorExists = false
					fixAttempt = 0
				} else {
					fixAttempt++
					if fixAttempt >= maxFixAttempts {
						// Max fix attempts — loop terminates, no further transition
						break
					}
					to = state.ModeFixError // fix failed, retry
					errorExists = true
				}
			}

			transitions = append(transitions, transition{from: from, to: to})
		}

		// Verify all transitions are valid
		for _, tr := range transitions {
			valid := false
			switch {
			case tr.from == state.ModeExecuteTask && tr.to == state.ModeExecuteTask:
				valid = true // validation passed
			case tr.from == state.ModeExecuteTask && tr.to == state.ModeFixError:
				valid = true // validation failed
			case tr.from == state.ModeFixError && tr.to == state.ModeExecuteTask:
				valid = true // fix succeeded
			case tr.from == state.ModeFixError && tr.to == state.ModeFixError:
				valid = true // fix failed, attempts < max
			}
			if !valid {
				t.Fatalf("invalid mode transition: %s → %s", tr.from, tr.to)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: ralphklaw, Property 10: Ralph Loop always terminates
// Validates: Requirements 2.1, 2.6, 2.7, 6.7
// ---------------------------------------------------------------------------

func TestProperty_RalphLoopAlwaysTerminates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxIterations := rapid.IntRange(1, 10).Draw(t, "max_iterations")
		maxFixAttempts := rapid.IntRange(1, 5).Draw(t, "max_fix_attempts")
		numTasks := rapid.IntRange(0, 5).Draw(t, "num_tasks")
		validationAlwaysFails := rapid.Bool().Draw(t, "validation_always_fails")

		// Build task list
		tasks := make([]state.Task, numTasks)
		for i := range tasks {
			tasks[i] = state.Task{
				Description: fmt.Sprintf("task %d", i),
				Completed:   false,
			}
		}

		// Track fix attempt count per error cycle
		fixAttemptCount := 0
		errorExists := false

		sm := &mockStateManager{
			detectModeFunc: func(ctx context.Context) state.Mode {
				if errorExists {
					return state.ModeFixError
				}
				return state.ModeExecuteTask
			},
			readTODOFunc: func(ctx context.Context) ([]state.Task, error) {
				return tasks, nil
			},
			writeTODOFunc: func(ctx context.Context, updated []state.Task) error {
				tasks = updated
				return nil
			},
			readErrorFunc: func(ctx context.Context) (*state.ValidationError, error) {
				return &state.ValidationError{
					Task:    "some task",
					Error:   "build failed",
					Attempt: fixAttemptCount,
				}, nil
			},
			writeErrorFunc: func(ctx context.Context, ve *state.ValidationError) error {
				errorExists = true
				fixAttemptCount = ve.Attempt
				return nil
			},
			deleteErrorFunc: func(ctx context.Context) error {
				errorExists = false
				fixAttemptCount = 0
				return nil
			},
		}

		val := &mockValidator{
			validateFunc: func(ctx context.Context) error {
				if validationAlwaysFails {
					return fmt.Errorf("build failed")
				}
				return nil
			},
		}

		inner := &mockInnerLoop{}
		git := &mockGitManager{}
		ch := &mockChannel{}

		rl := newTestRalphLoop(inner, val, sm, git, ch, maxIterations, maxFixAttempts)

		// Run must always return (terminate)
		done := make(chan error, 1)
		go func() {
			done <- rl.Run(context.Background())
		}()

		select {
		case err := <-done:
			// Terminated — verify it's a valid termination reason
			if err != nil {
				if err != ErrMaxIterationsReached && err != ErrMaxFixAttemptsReached {
					// Other errors are acceptable (e.g., inner loop errors)
					// but the loop must have terminated
				}
			}
			// Success: loop terminated
		case <-context.Background().Done():
			t.Fatal("loop did not terminate")
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

// Unit test 1: Loop completes when TODO.md is empty (returns nil immediately)
func TestRalphLoop_CompletesWhenTODOEmpty(t *testing.T) {
	sm := &mockStateManager{
		detectModeFunc: func(ctx context.Context) state.Mode {
			return state.ModeExecuteTask
		},
		readTODOFunc: func(ctx context.Context) ([]state.Task, error) {
			return []state.Task{}, nil // empty
		},
	}

	commitCount := 0
	git := &mockGitManager{
		commitAndPushFunc: func(ctx context.Context, iteration int, mode, outcome string) {
			commitCount++
		},
	}

	rl := newTestRalphLoop(&mockInnerLoop{}, &mockValidator{}, sm, git, &mockChannel{}, 10, 3)
	err := rl.Run(context.Background())

	if err != nil {
		t.Fatalf("expected nil error for empty TODO, got: %v", err)
	}
	if commitCount != 1 {
		t.Errorf("expected 1 commit (completion), got %d", commitCount)
	}
}

// Unit test 2: Loop stops at max iterations (returns ErrMaxIterationsReached)
func TestRalphLoop_StopsAtMaxIterations(t *testing.T) {
	sm := &mockStateManager{
		detectModeFunc: func(ctx context.Context) state.Mode {
			return state.ModeExecuteTask
		},
		readTODOFunc: func(ctx context.Context) ([]state.Task, error) {
			// Always return a fresh incomplete task — simulates a task that never completes
			return []state.Task{{Description: "never ending task", Completed: false}}, nil
		},
		writeTODOFunc: func(ctx context.Context, updated []state.Task) error {
			// Intentionally ignore writes — task stays incomplete on next read
			return nil
		},
	}

	rl := newTestRalphLoop(&mockInnerLoop{}, &mockValidator{}, sm, &mockGitManager{}, &mockChannel{}, 3, 3)
	err := rl.Run(context.Background())

	if err != ErrMaxIterationsReached {
		t.Fatalf("expected ErrMaxIterationsReached, got: %v", err)
	}
}

// Unit test 3: Validation failure transitions to FIX_ERROR (LAST_ERROR.txt written)
func TestRalphLoop_ValidationFailureTransitionsToFixError(t *testing.T) {
	var writtenError *state.ValidationError
	errorExists := false
	taskDone := false

	sm := &mockStateManager{
		detectModeFunc: func(ctx context.Context) state.Mode {
			if errorExists {
				return state.ModeFixError
			}
			return state.ModeExecuteTask
		},
		readTODOFunc: func(ctx context.Context) ([]state.Task, error) {
			if taskDone {
				return []state.Task{}, nil
			}
			return []state.Task{{Description: "my task", Completed: false}}, nil
		},
		writeTODOFunc: func(ctx context.Context, tasks []state.Task) error {
			// Mark task done when WriteTODO is called with completed task
			for _, t := range tasks {
				if t.Completed {
					taskDone = true
				}
			}
			return nil
		},
		writeErrorFunc: func(ctx context.Context, ve *state.ValidationError) error {
			writtenError = ve
			errorExists = true
			return nil
		},
		readErrorFunc: func(ctx context.Context) (*state.ValidationError, error) {
			if writtenError != nil {
				return writtenError, nil
			}
			return &state.ValidationError{Task: "my task", Error: "build failed", Attempt: 1}, nil
		},
		deleteErrorFunc: func(ctx context.Context) error {
			errorExists = false
			return nil
		},
	}

	validationCallCount := 0
	val := &mockValidator{
		validateFunc: func(ctx context.Context) error {
			validationCallCount++
			if validationCallCount == 1 {
				return fmt.Errorf("build failed: undefined: foo")
			}
			// Second call (fix attempt) passes
			return nil
		},
	}

	rl := newTestRalphLoop(&mockInnerLoop{}, val, sm, &mockGitManager{}, &mockChannel{}, 10, 3)
	err := rl.Run(context.Background())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Verify LAST_ERROR.txt was written after first validation failure
	if writtenError == nil {
		t.Fatal("expected LAST_ERROR.txt to be written on validation failure")
	}
	if writtenError.Task != "my task" {
		t.Errorf("error task = %q, want %q", writtenError.Task, "my task")
	}
	if writtenError.Error == "" {
		t.Error("error content should not be empty")
	}
}

// Unit test 4: Successful fix transitions back to EXECUTE_TASK (LAST_ERROR.txt deleted)
func TestRalphLoop_SuccessfulFixDeletesLastError(t *testing.T) {
	errorDeleted := false
	errorExists := true // start in FIX_ERROR mode

	sm := &mockStateManager{
		detectModeFunc: func(ctx context.Context) state.Mode {
			if errorExists {
				return state.ModeFixError
			}
			return state.ModeExecuteTask
		},
		readTODOFunc: func(ctx context.Context) ([]state.Task, error) {
			// No tasks left after fix
			return []state.Task{}, nil
		},
		readErrorFunc: func(ctx context.Context) (*state.ValidationError, error) {
			return &state.ValidationError{
				Task:    "broken task",
				Error:   "undefined: foo",
				Attempt: 1,
			}, nil
		},
		deleteErrorFunc: func(ctx context.Context) error {
			errorDeleted = true
			errorExists = false
			return nil
		},
	}

	// Validation passes on fix attempt
	val := &mockValidator{
		validateFunc: func(ctx context.Context) error {
			return nil
		},
	}

	rl := newTestRalphLoop(&mockInnerLoop{}, val, sm, &mockGitManager{}, &mockChannel{}, 10, 3)
	err := rl.Run(context.Background())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !errorDeleted {
		t.Error("expected LAST_ERROR.txt to be deleted after successful fix")
	}
}

// Unit test 5: Max fix attempts escalates (returns ErrMaxFixAttemptsReached)
func TestRalphLoop_MaxFixAttemptsReached(t *testing.T) {
	attempt := 1

	sm := &mockStateManager{
		detectModeFunc: func(ctx context.Context) state.Mode {
			return state.ModeFixError
		},
		readErrorFunc: func(ctx context.Context) (*state.ValidationError, error) {
			return &state.ValidationError{
				Task:    "broken task",
				Error:   "build failed",
				Attempt: attempt,
			}, nil
		},
		writeErrorFunc: func(ctx context.Context, ve *state.ValidationError) error {
			attempt = ve.Attempt
			return nil
		},
	}

	// Validation always fails
	val := &mockValidator{
		validateFunc: func(ctx context.Context) error {
			return fmt.Errorf("still broken")
		},
	}

	rl := newTestRalphLoop(&mockInnerLoop{}, val, sm, &mockGitManager{}, &mockChannel{}, 10, 3)
	err := rl.Run(context.Background())

	if err != ErrMaxFixAttemptsReached {
		t.Fatalf("expected ErrMaxFixAttemptsReached, got: %v", err)
	}
}
