# Parallel Tool Execution Design

**Date**: 2026-05-04
**Issue**: #45
**Status**: Draft

## Problem

DocsClaw's agentic loop executes tool calls sequentially. When
the LLM requests multiple independent tools (e.g., fetch document +
check permissions + read file), each waits for the previous to
finish. This adds unnecessary latency, especially for I/O-bound
tools like `webfetch` and `fetchdoc`.

## Design

### Parallel execution with errgroup

Replace the sequential for-loop in `RunToolLoop` (lines 68-77)
with `errgroup.Group`. Each tool call runs in its own goroutine.

Results are written to a pre-allocated slice indexed by position,
so ordering is deterministic without synchronization:

```go
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
_ = g.Wait()
```

All tools default to parallel. No per-tool execution mode.

### AfterToolCall hook

Extend the `Hook` interface with `AfterToolCall`:

```go
type Hook interface {
    BeforeToolCall(ctx context.Context, name string,
        args map[string]any) (allow bool, reason string)
    AfterToolCall(ctx context.Context, name string,
        args map[string]any, result *ToolResult)
}
```

Called in `executeTool` after execution completes. Enables
logging, audit, and result observation for business compliance.

### Wire hook from LoopConfig

Add `Hook` field to `LoopConfig` so callers can provide one:

```go
type LoopConfig struct {
    MaxIterations  int
    MaxResultBytes int
    Hook           Hook
}
```

Update `RunToolLoop` to pass `config.Hook` to `executeTool`
instead of the hardcoded `nil`.

## Files to modify

| File | Change |
|------|--------|
| `pkg/tools/hooks.go` | Add `AfterToolCall` to `Hook` interface |
| `pkg/tools/loop.go` | Add `Hook` to config, use errgroup for parallel execution, call `AfterToolCall` |
| `pkg/tools/loop_test.go` | Add parallel execution test, AfterToolCall test |

## Testing

- `TestRunToolLoopParallelExecution`: multiple tools with a
  short delay, verify total time is less than sequential sum
- `TestAfterToolCallHook`: verify hook is called with correct
  name, args, and result after each tool execution
- `TestBeforeToolCallHookDenial`: verify existing BeforeToolCall
  behavior still works (regression)
- All existing loop tests must continue to pass

## Decisions

- **All tools parallel by default**: trust the LLM not to issue
  conflicting parallel calls. Can add per-tool mode later.
- **Single Hook interface**: one interface with both Before and
  After methods. Nothing implements Hook today, so adding a
  method is non-breaking.
- **errgroup over WaitGroup**: idiomatic Go, context propagation,
  already an indirect dependency.
