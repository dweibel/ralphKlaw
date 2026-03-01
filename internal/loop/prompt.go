package loop

import (
	"fmt"

	"github.com/eachlabs/ralphklaw/internal/state"
)

const systemPrompt = `You are ralphKlaw, an autonomous coding agent working on a Go project.

Your workflow:
- Read files to understand the codebase before making changes
- Make minimal, focused changes to accomplish the task
- Use the edit tool for existing files, write tool for new files
- Use glob and grep to find relevant files
- Use bash to run commands when needed

Available tools: bash, read, write, edit, glob, grep

Guidelines:
- Always read relevant files before editing them
- Make the smallest change that accomplishes the task
- Ensure all Go code compiles (go build) and passes go vet
- When fixing errors, analyze the root cause before changing code
- Do not modify files unrelated to the current task`

// BuildTaskPrompt constructs the user prompt for EXECUTE_TASK mode.
func BuildTaskPrompt(task *state.Task, workspace string) string {
	return fmt.Sprintf(`Current task: %s

Workspace directory: %s

Implement this task. Use the available tools to:
1. Explore the codebase to understand the current structure
2. Make the necessary code changes
3. Verify your changes make sense

The workspace is a Go project. After you finish, validation will run automatically (go build ./... && go vet ./...).`,
		task.Description, workspace)
}

// BuildFixPrompt constructs the user prompt for FIX_ERROR mode.
func BuildFixPrompt(errInfo *state.ValidationError) string {
	return fmt.Sprintf(`Go validation failed. Fix the error below.

Task that caused the error: %s
Validator output:
%s

Analyze the error, read the relevant files, and fix the issue.
After you finish, validation will run automatically (go build ./... && go vet ./...).`,
		errInfo.Task, errInfo.Error)
}

// SystemPrompt returns the system prompt for the inner loop.
func SystemPrompt() string {
	return systemPrompt
}
