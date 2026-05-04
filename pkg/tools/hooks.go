package tools

import "context"

// Hook allows external systems to intercept tool calls.
// Implementations must be safe for concurrent use — with parallel
// tool execution, both methods may be called from multiple goroutines.
// AfterToolCall receives the result for observation only; implementations
// must not mutate the ToolResult as it is read by the caller after return.
type Hook interface {
	BeforeToolCall(ctx context.Context, name string,
		args map[string]any) (allow bool, reason string)
	AfterToolCall(ctx context.Context, name string,
		args map[string]any, result *ToolResult)
}
