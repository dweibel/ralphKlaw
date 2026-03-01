// Package logging provides structured JSON logging for ralphKlaw.
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// LogEntry is a single structured log record.
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Logger writes structured JSON log entries.
type Logger struct {
	level   LogLevel
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewLogger creates a Logger that writes to the given file path at the given level.
func NewLogger(level string, filePath string) (*Logger, error) {
	lvl := ParseLevel(level)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &Logger{
		level:   lvl,
		file:    f,
		encoder: json.NewEncoder(f),
	}, nil
}

// NewNopLogger returns a logger that discards all output.
func NewNopLogger() *Logger {
	return &Logger{level: LevelError + 1}
}

// redact replaces known sensitive patterns in msg with [REDACTED].
// Patterns: sk- (API keys), Bearer  (auth headers), password= and token= (credential values).
func redact(msg string) string {
	// Replace everything after "sk-" up to the next whitespace or end of string.
	msg = replaceAfter(msg, "sk-")
	// Replace everything after "Bearer " up to the next whitespace or end of string.
	msg = replaceAfter(msg, "Bearer ")
	// Replace everything after "password=" up to the next whitespace or end of string.
	msg = replaceAfter(msg, "password=")
	// Replace everything after "token=" up to the next whitespace or end of string.
	msg = replaceAfter(msg, "token=")
	return msg
}

// replaceAfter finds prefix in s and replaces the text immediately following it
// (up to the next whitespace or end of string) with [REDACTED].
func replaceAfter(s, prefix string) string {
	idx := strings.Index(s, prefix)
	if idx == -1 {
		return s
	}
	start := idx + len(prefix)
	end := start
	for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' && s[end] != '\r' {
		end++
	}
	return s[:idx] + prefix + "[REDACTED]" + s[end:]
}

func (l *Logger) log(level LogLevel, msg string, fields map[string]interface{}) {
	if level < l.level || l.file == nil {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     levelName(level),
		Message:   redact(msg),
		Fields:    fields,
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.encoder.Encode(entry) //nolint:errcheck
}

// Info logs at INFO level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, fmt.Sprintf(format, args...), nil)
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, fmt.Sprintf(format, args...), nil)
}

// Warn logs at WARN level.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, fmt.Sprintf(format, args...), nil)
}

// Error logs at ERROR level.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...), nil)
}

// WithFields logs at the given level with extra structured fields.
func (l *Logger) WithFields(level LogLevel, msg string, fields map[string]interface{}) {
	l.log(level, msg, fields)
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	if l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// ParseLevel converts a string level name to a LogLevel.
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func levelName(l LogLevel) string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
