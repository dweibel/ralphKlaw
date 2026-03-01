// Package validation runs Go build and vet checks.
package validation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
)

// Validator runs configured validation commands via the bash tool.
type Validator struct {
	tools   *tool.Registry
	command string
	logger  *logging.Logger
}

// NewValidator creates a Validator with the given command (empty = default).
func NewValidator(tools *tool.Registry, command string, logger *logging.Logger) *Validator {
	if command == "" {
		command = "go build ./... && go vet ./..."
	}
	return &Validator{tools: tools, command: command, logger: logger}
}

// Validate runs the configured validation command and returns nil on success.
func (v *Validator) Validate(ctx context.Context) error {
	v.logger.Info("running validation: %s", v.command)

	bash, ok := v.tools.Get("bash")
	if !ok {
		return fmt.Errorf("bash tool not available")
	}

	params, _ := json.Marshal(map[string]string{"command": v.command})
	result, err := bash.Execute(ctx, params)
	if err != nil {
		return fmt.Errorf("validation command failed: %w", err)
	}

	if result.IsError {
		return fmt.Errorf("%s", result.Content)
	}

	v.logger.Info("validation passed")
	return nil
}
