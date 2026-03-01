package state

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: ralphklaw, Property 3: LAST_ERROR.txt parse/format round-trip
// Validates: Requirements 10.3, 10.4
func TestProperty_ErrorParseFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid ValidationError
		ve := &ValidationError{
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Iteration: rapid.IntRange(1, 1000).Draw(t, "iteration"),
			Task:      rapid.StringMatching(`^[a-zA-Z0-9 _-]+$`).Draw(t, "task"),
			Error:     rapid.StringMatching(`^[a-zA-Z0-9 .:/_-]+$`).Draw(t, "error"),
			Attempt:   rapid.IntRange(1, 100).Draw(t, "attempt"),
		}

		// Format
		formatted := formatError(ve)

		// Parse back
		parsed, err := parseError(formatted)
		if err != nil {
			t.Fatalf("parseError failed: %v\nformatted:\n%s", err, formatted)
		}

		// Compare (timestamps truncated to second precision)
		if !parsed.Timestamp.Equal(ve.Timestamp) {
			t.Fatalf("timestamp mismatch: got %v, want %v", parsed.Timestamp, ve.Timestamp)
		}
		if parsed.Iteration != ve.Iteration {
			t.Fatalf("iteration mismatch: got %d, want %d", parsed.Iteration, ve.Iteration)
		}
		if parsed.Task != ve.Task {
			t.Fatalf("task mismatch: got %q, want %q", parsed.Task, ve.Task)
		}
		if parsed.Error != ve.Error {
			t.Fatalf("error mismatch: got %q, want %q", parsed.Error, ve.Error)
		}
		if parsed.Attempt != ve.Attempt {
			t.Fatalf("attempt mismatch: got %d, want %d", parsed.Attempt, ve.Attempt)
		}
	})
}

func TestFormatError_ProducesValidYAML(t *testing.T) {
	ve := &ValidationError{
		Timestamp: time.Date(2026, 2, 20, 10, 30, 0, 0, time.UTC),
		Iteration: 5,
		Task:      "Test task",
		Error:     "test error",
		Attempt:   2,
	}

	formatted := formatError(ve)

	// Should contain all fields
	if !strings.Contains(formatted, "timestamp:") {
		t.Error("formatted output missing timestamp")
	}
	if !strings.Contains(formatted, "iteration: 5") {
		t.Error("formatted output missing iteration")
	}
	if !strings.Contains(formatted, `task: "Test task"`) {
		t.Error("formatted output missing task")
	}
	if !strings.Contains(formatted, "error: |") {
		t.Error("formatted output missing error block")
	}
	if !strings.Contains(formatted, "attempt: 2") {
		t.Error("formatted output missing attempt")
	}
}

func TestParseError_HandlesMultilineError(t *testing.T) {
	content := `timestamp: 2026-02-20T10:30:00Z
iteration: 3
task: "Build failed"
error: |
  line 1
  line 2
  line 3
attempt: 1
`

	ve, err := parseError(content)
	if err != nil {
		t.Fatalf("parseError failed: %v", err)
	}

	if ve.Iteration != 3 {
		t.Errorf("iteration = %d, want 3", ve.Iteration)
	}
	if ve.Task != "Build failed" {
		t.Errorf("task = %q, want %q", ve.Task, "Build failed")
	}
	if !strings.Contains(ve.Error, "line 1") || !strings.Contains(ve.Error, "line 2") {
		t.Errorf("error does not contain multiline content: %q", ve.Error)
	}
	if ve.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", ve.Attempt)
	}
}

func TestParseError_InvalidContent_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"invalid yaml", "invalid: [unclosed"},
		{"missing fields", "timestamp: 2026-02-20T10:30:00Z\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseError(tt.content)
			if err == nil {
				t.Error("expected error for invalid content, got nil")
			}
		})
	}
}

func TestFormatError_SetsTimestamp(t *testing.T) {
	ve := &ValidationError{
		Iteration: 1,
		Task:      "test",
		Error:     "error",
		Attempt:   1,
	}

	formatted := formatError(ve)

	// Should have set timestamp
	if !strings.Contains(formatted, "timestamp:") {
		t.Error("timestamp not set in formatted output")
	}
}
