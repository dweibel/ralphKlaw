package state

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ValidationError represents the contents of LAST_ERROR.txt.
type ValidationError struct {
	Timestamp time.Time `yaml:"timestamp"`
	Iteration int       `yaml:"iteration"`
	Task      string    `yaml:"task"`
	Error     string    `yaml:"error"`
	Attempt   int       `yaml:"attempt"`
}

// errorScalarStyle picks the YAML scalar style for the error field.
// Block literal (|) is used normally, but strings whose lines are all
// whitespace would be mangled by a block scalar, so we fall back to
// double-quoted style which preserves every character exactly.
func errorScalarStyle(s string) yaml.Style {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimRight(line, " \t") != line {
			// line has trailing whitespace — block scalar would strip it
			return yaml.DoubleQuotedStyle
		}
	}
	if strings.TrimSpace(s) == "" && s != "" {
		// whitespace-only string — block scalar collapses it
		return yaml.DoubleQuotedStyle
	}
	return yaml.LiteralStyle
}

// formatError serializes a ValidationError to YAML for LAST_ERROR.txt.
func formatError(ve *ValidationError) string {
	ve.Timestamp = ve.Timestamp.UTC().Truncate(time.Second)

	errorNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: ve.Error,
		Style: errorScalarStyle(ve.Error),
	}

	doc := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "timestamp"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: ve.Timestamp.Format(time.RFC3339)},
			{Kind: yaml.ScalarNode, Value: "iteration"},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", ve.Iteration)},
			{Kind: yaml.ScalarNode, Value: "task"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: ve.Task, Style: yaml.DoubleQuotedStyle},
			{Kind: yaml.ScalarNode, Value: "error"},
			errorNode,
			{Kind: yaml.ScalarNode, Value: "attempt"},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", ve.Attempt)},
		},
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{doc}})
	enc.Close()

	// yaml.Encoder wraps output in "---\n" document marker; strip it.
	result := buf.String()
	result = strings.TrimPrefix(result, "---\n")
	return result
}

// parseError deserializes LAST_ERROR.txt content into a ValidationError.
func parseError(content string) (*ValidationError, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("failed to parse LAST_ERROR.txt: empty content")
	}
	var ve ValidationError
	if err := yaml.Unmarshal([]byte(content), &ve); err != nil {
		return nil, fmt.Errorf("failed to parse LAST_ERROR.txt: %w", err)
	}
	// Validate required keys are present in the raw content.
	// (Parsed zero-values can't distinguish "absent" from "empty".)
	if !strings.Contains(content, "task:") || !strings.Contains(content, "error:") {
		return nil, fmt.Errorf("failed to parse LAST_ERROR.txt: missing required fields (task, error)")
	}
	return &ve, nil
}
