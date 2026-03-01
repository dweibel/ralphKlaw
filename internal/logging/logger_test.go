package logging

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNewLogger_CreatesFile(t *testing.T) {
	tmpFile := t.TempDir() + "/test-logger.log"

	logger, err := NewLogger("info", tmpFile)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

func TestLogger_WritesValidJSON(t *testing.T) {
	tmpFile := t.TempDir() + "/test-logger-json.log"

	logger, err := NewLogger("info", tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("test message")
	logger.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("log entry is not valid JSON: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("level = %q, want %q", entry.Level, "INFO")
	}
	if entry.Message != "test message" {
		t.Errorf("message = %q, want %q", entry.Message, "test message")
	}
	if entry.Timestamp == "" {
		t.Error("timestamp is empty")
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	tmpFile := t.TempDir() + "/test-logger-filter.log"

	logger, err := NewLogger("info", tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if strings.Contains(content, "debug message") {
		t.Error("DEBUG message was logged at INFO level")
	}
	if !strings.Contains(content, "info message") {
		t.Error("INFO message was not logged")
	}
}

func TestParseLevel_AllLevels(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"WARN", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo}, // default
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLogger_Close_FlushesFile(t *testing.T) {
	tmpFile := t.TempDir() + "/test-logger-close.log"

	logger, err := NewLogger("info", tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("test")
	
	// Close should flush
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("log file is empty after close")
	}
}

func TestLogger_AllLevels(t *testing.T) {
	tmpFile := t.TempDir() + "/test-logger-all.log"

	logger, err := NewLogger("debug", tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")
	logger.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	for _, msg := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(content, msg) {
			t.Errorf("log does not contain %q", msg)
		}
	}
}

func TestNewNopLogger(t *testing.T) {
	logger := NewNopLogger()

	// Should not panic
	logger.Info("test")
	logger.Debug("test")
	logger.Warn("test")
	logger.Error("test")
	logger.Close()
}

// --- Sensitive data filtering tests (task 22.3) ---
// These tests call redact(), which is not yet implemented (task 22.4).
// They are intentionally RED until task 22.4 is complete.

func TestRedact_APIKey(t *testing.T) {
	// sk- prefix is a common API key pattern (e.g. OpenAI, Anthropic)
	input := "calling provider with key sk-abc123secretkey"
	got := redact(input)
	if strings.Contains(got, "sk-abc123secretkey") {
		t.Errorf("redact() did not remove API key: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("redact() did not insert [REDACTED]: %q", got)
	}
}

func TestRedact_BearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig"
	got := redact(input)
	if strings.Contains(got, "eyJhbGciOiJIUzI1NiJ9") {
		t.Errorf("redact() did not remove bearer token: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("redact() did not insert [REDACTED]: %q", got)
	}
}

func TestRedact_Password(t *testing.T) {
	input := "connecting with password=supersecret123"
	got := redact(input)
	if strings.Contains(got, "supersecret123") {
		t.Errorf("redact() did not remove password value: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("redact() did not insert [REDACTED]: %q", got)
	}
}

func TestRedact_Token(t *testing.T) {
	input := "using token=ghp_myGitHubToken for auth"
	got := redact(input)
	if strings.Contains(got, "ghp_myGitHubToken") {
		t.Errorf("redact() did not remove token value: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("redact() did not insert [REDACTED]: %q", got)
	}
}

func TestRedact_CleanMessage(t *testing.T) {
	// Messages with no sensitive patterns should pass through unchanged.
	input := "iteration 3 starting in EXECUTE_TASK mode"
	got := redact(input)
	if got != input {
		t.Errorf("redact() modified a clean message: got %q, want %q", got, input)
	}
}

func TestLogger_RedactsInLogFile(t *testing.T) {
	// End-to-end: sensitive content logged via Info() must not appear in the file.
	tmpFile := t.TempDir() + "/test-logger-redact.log"

	logger, err := NewLogger("info", tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("provider key is sk-topsecret and token=abc999")
	logger.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if strings.Contains(content, "sk-topsecret") {
		t.Error("log file contains raw API key (sk-topsecret)")
	}
	if strings.Contains(content, "abc999") {
		t.Error("log file contains raw token value (abc999)")
	}
	if !strings.Contains(content, "[REDACTED]") {
		t.Error("log file does not contain [REDACTED] placeholder")
	}
}
