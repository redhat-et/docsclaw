package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/redhat-et/docsclaw/pkg/llm"
	"golang.org/x/sync/errgroup"
)

const defaultMaxResultBytes = 32768 // 32KB

// LoopConfig controls the agentic loop behavior.
type LoopConfig struct {
	MaxIterations  int
	MaxResultBytes int  // per-tool output limit; 0 disables truncation
	Hook           Hook // optional hook for intercepting tool calls
}

// DefaultLoopConfig returns sensible defaults.
func DefaultLoopConfig() LoopConfig {
	return LoopConfig{
		MaxIterations:  10,
		MaxResultBytes: defaultMaxResultBytes,
	}
}

// RunToolLoop executes the agentic tool-use loop.
func RunToolLoop(ctx context.Context, provider llm.Provider,
	messages []llm.Message, registry *Registry,
	config LoopConfig) (string, error) {

	toolDefs := registry.Definitions()

	for i := 0; i < config.MaxIterations; i++ {
		resp, err := provider.CompleteWithTools(ctx, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		logAttrs := []any{
			"provider", provider.ProviderName(),
			"model", provider.Model(),
			"iteration", i + 1,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens,
			"total_tokens", resp.Usage.TotalTokens,
			"stop_reason", resp.StopReason,
		}
		if resp.Usage.CacheReadTokens > 0 {
			logAttrs = append(logAttrs, "cache_read_tokens", resp.Usage.CacheReadTokens)
		}
		if resp.Usage.CacheWriteTokens > 0 {
			logAttrs = append(logAttrs, "cache_write_tokens", resp.Usage.CacheWriteTokens)
		}
		slog.Info("LLM response received", logAttrs...)

		if !resp.HasToolCalls() {
			return resp.Content, nil
		}

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		results := make([]llm.ToolResultContent, len(resp.ToolCalls))
		g, gctx := errgroup.WithContext(ctx)
		for i, tc := range resp.ToolCalls {
			g.Go(func() error {
				result := executeTool(gctx, registry, tc, config.Hook)
				output := truncateResult(result.Output, config.MaxResultBytes)
				results[i] = llm.ToolResultContent{
					ToolUseID: tc.ID,
					Output:    output,
					IsError:   result.Error,
				}
				return nil
			})
		}
		// Tool errors are captured in ToolResult.Error, not returned from goroutines.
		_ = g.Wait()

		messages = append(messages, llm.Message{
			Role:        "tool",
			ToolResults: results,
		})
	}

	return "", fmt.Errorf("max iterations (%d) reached without final response", config.MaxIterations)
}

func executeTool(ctx context.Context, registry *Registry,
	tc llm.ToolCall, hook Hook) *ToolResult {

	if hook != nil {
		allow, reason := hook.BeforeToolCall(ctx, tc.Name, tc.Args)
		if !allow {
			slog.Warn("tool call denied by hook",
				"tool", tc.Name, "reason", reason)
			return Errorf("Tool call denied: %s", reason)
		}
	}

	tool, ok := registry.Get(tc.Name)
	if !ok {
		slog.Warn("unknown tool requested", "tool", tc.Name)
		return Errorf("Unknown tool: %s", tc.Name)
	}

	slog.Info("executing tool", "tool", tc.Name)
	result := tool.Execute(ctx, tc.Args)
	if result.Error {
		slog.Warn("tool returned error",
			"tool", tc.Name, "output", truncateLog(result.Output))
	}

	if hook != nil {
		hook.AfterToolCall(ctx, tc.Name, tc.Args, result)
	}

	return result
}

func truncateResult(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	// Don't split a multi-byte UTF-8 character.
	truncated := strings.ToValidUTF8(s[:maxBytes], "")
	slog.Warn("tool output truncated",
		"original_bytes", len(s), "retained_bytes", len(truncated))
	return truncated + fmt.Sprintf("\n\n[Truncated: showing first %d bytes of %d total]", len(truncated), len(s))
}

func truncateLog(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
