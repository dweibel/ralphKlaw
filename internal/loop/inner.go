package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/eachlabs/klaw/pkg/provider"
	"github.com/eachlabs/klaw/pkg/tool"
	"github.com/eachlabs/ralphklaw/internal/logging"
)

// InnerLoop runs the Agent SDK-style multi-turn LLM conversation.
type InnerLoop struct {
	provider provider.Provider
	tools    *tool.Registry
	config   *InnerLoopConfig
	logger   *logging.Logger
}

// InnerLoopConfig controls the inner loop behavior.
type InnerLoopConfig struct {
	MaxRounds   int
	MaxTokens   int
	Temperature float64
	Model       string
}

// NewInnerLoop creates an InnerLoop.
func NewInnerLoop(p provider.Provider, tools *tool.Registry, cfg *InnerLoopConfig, logger *logging.Logger) *InnerLoop {
	return &InnerLoop{provider: p, tools: tools, config: cfg, logger: logger}
}

// Execute runs the multi-turn loop until the LLM returns no tool calls or max rounds is reached.
func (l *InnerLoop) Execute(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	toolDefs := l.buildToolDefs()

	messages := []provider.Message{
		{Role: "user", Content: userPrompt},
	}

	for round := 0; round < l.config.MaxRounds; round++ {
		l.logger.Debug("inner loop round %d", round+1)

		var resp *provider.ChatResponse
		err := RetryWithBackoff(ctx, 3, func() error {
			var chatErr error
			resp, chatErr = l.provider.Chat(ctx, &provider.ChatRequest{
				System:      systemPrompt,
				Messages:    messages,
				Tools:       toolDefs,
				MaxTokens:   l.config.MaxTokens,
				Model:       l.config.Model,
				Temperature: l.config.Temperature,
			})
			return chatErr
		})
		if err != nil {
			return "", fmt.Errorf("provider chat failed (round %d): %w", round+1, err)
		}

		var text strings.Builder
		var toolCalls []provider.ToolCall

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				text.WriteString(block.Text)
			case "tool_use":
				if block.ToolUse != nil {
					toolCalls = append(toolCalls, *block.ToolUse)
				}
			}
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   text.String(),
			ToolCalls: toolCalls,
		})

		if len(toolCalls) == 0 {
			l.logger.Debug("inner loop complete after %d rounds", round+1)
			return text.String(), nil
		}

		for _, tc := range toolCalls {
			l.logger.Debug("tool call: %s", tc.Name)

			t, ok := l.tools.Get(tc.Name)
			if !ok {
				l.logger.WithFields(logging.LevelDebug, "tool call result", map[string]interface{}{
					"round":    round + 1,
					"tool":     tc.Name,
					"is_error": true,
				})
				messages = append(messages, provider.Message{
					Role: "user",
					ToolResult: &provider.ToolResult{
						ToolUseID: tc.ID,
						Content:   fmt.Sprintf("unknown tool: %s", tc.Name),
						IsError:   true,
					},
				})
				continue
			}

			result, err := t.Execute(ctx, tc.Input)
			if err != nil {
				l.logger.WithFields(logging.LevelDebug, "tool call result", map[string]interface{}{
					"round":    round + 1,
					"tool":     tc.Name,
					"is_error": true,
				})
				messages = append(messages, provider.Message{
					Role: "user",
					ToolResult: &provider.ToolResult{
						ToolUseID: tc.ID,
						Content:   fmt.Sprintf("tool error: %v", err),
						IsError:   true,
					},
				})
				continue
			}

			l.logger.WithFields(logging.LevelDebug, "tool call result", map[string]interface{}{
				"round":    round + 1,
				"tool":     tc.Name,
				"is_error": result.IsError,
			})
			messages = append(messages, provider.Message{
				Role: "user",
				ToolResult: &provider.ToolResult{
					ToolUseID: tc.ID,
					Content:   result.Content,
					IsError:   result.IsError,
				},
			})
		}

		if resp.StopReason == "end_turn" {
			l.logger.Debug("inner loop ended by stop reason after %d rounds", round+1)
			return text.String(), nil
		}
	}

	return "", fmt.Errorf("inner loop exceeded max rounds (%d)", l.config.MaxRounds)
}

func (l *InnerLoop) buildToolDefs() []provider.ToolDefinition {
	tools := l.tools.All()
	defs := make([]provider.ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = provider.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		}
	}
	return defs
}
