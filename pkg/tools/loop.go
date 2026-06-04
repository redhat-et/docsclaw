package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/redhat-et/docsclaw/internal/telemetry"
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

	tracer := otel.Tracer(telemetry.TracerName)
	toolDefs := registry.Definitions()

	ctx, loopSpan := tracer.Start(ctx, "agent.loop", trace.WithAttributes(
		telemetry.AttrLoopMaxIterations.Int(config.MaxIterations),
		telemetry.AttrLoopToolCount.Int(len(toolDefs)),
		telemetry.AttrLLMProvider.String(provider.ProviderName()),
		telemetry.AttrLLMModel.String(provider.Model()),
	))
	defer loopSpan.End()

	// Record initial conversation messages as span events.
	for _, msg := range messages {
		telemetry.AddMessageEvent(loopSpan, "llm.input", msg.Role, msg.Content)
	}

	for i := 0; i < config.MaxIterations; i++ {
		iterCtx, iterSpan := tracer.Start(ctx,
			fmt.Sprintf("agent.loop.iteration.%d", i+1),
			trace.WithAttributes(
				telemetry.AttrLoopIteration.Int(i+1),
			))

		resp, err := provider.CompleteWithTools(iterCtx, messages, toolDefs)
		if err != nil {
			iterSpan.SetStatus(codes.Error, err.Error())
			iterSpan.End()
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// Set token usage as span attributes.
		iterSpan.SetAttributes(
			telemetry.AttrLLMInputTokens.Int(resp.Usage.InputTokens),
			telemetry.AttrLLMOutputTokens.Int(resp.Usage.OutputTokens),
			telemetry.AttrLLMTotalTokens.Int(resp.Usage.TotalTokens),
			telemetry.AttrLLMStopReason.String(string(resp.StopReason)),
		)
		if resp.Usage.CacheReadTokens > 0 {
			iterSpan.SetAttributes(telemetry.AttrLLMCacheReadTokens.Int(resp.Usage.CacheReadTokens))
		}
		if resp.Usage.CacheWriteTokens > 0 {
			iterSpan.SetAttributes(telemetry.AttrLLMCacheWriteTokens.Int(resp.Usage.CacheWriteTokens))
		}

		telemetry.AddMessageEvent(iterSpan, "llm.response", "assistant", resp.Content)

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
			iterSpan.End()
			return resp.Content, nil
		}

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		results := make([]llm.ToolResultContent, len(resp.ToolCalls))
		g, gctx := errgroup.WithContext(iterCtx)
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
		_ = g.Wait()

		iterSpan.End()

		if ctx.Err() != nil {
			loopSpan.SetStatus(codes.Error, ctx.Err().Error())
			return "", ctx.Err()
		}

		messages = append(messages, llm.Message{
			Role:        "tool",
			ToolResults: results,
		})
	}

	loopSpan.SetStatus(codes.Error, "max iterations reached")
	return "", fmt.Errorf("max iterations (%d) reached without final response", config.MaxIterations)
}

func executeTool(ctx context.Context, registry *Registry,
	tc llm.ToolCall, hook Hook) *ToolResult {

	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "tool.execute",
		trace.WithAttributes(
			telemetry.AttrToolName.String(tc.Name),
		))
	defer span.End()

	telemetry.AddToolArgsEvent(span, tc.Name, tc.Args)

	if hook != nil {
		allow, reason := hook.BeforeToolCall(ctx, tc.Name, tc.Args)
		if !allow {
			span.SetAttributes(
				telemetry.AttrToolDenied.Bool(true),
				telemetry.AttrToolDenyReason.String(reason),
			)
			slog.Warn("tool call denied by hook",
				"tool", tc.Name, "reason", reason)
			return Errorf("Tool call denied: %s", reason)
		}
	}

	tool, ok := registry.Get(tc.Name)
	if !ok {
		span.SetStatus(codes.Error, "unknown tool")
		slog.Warn("unknown tool requested", "tool", tc.Name)
		return Errorf("Unknown tool: %s", tc.Name)
	}

	slog.Info("executing tool", "tool", tc.Name)
	result := tool.Execute(ctx, tc.Args)

	telemetry.AddToolResultEvent(span, tc.Name, result.Output, result.Error)
	if result.Error {
		span.SetAttributes(telemetry.AttrToolError.Bool(true))
		span.SetStatus(codes.Error, "tool returned error")
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
