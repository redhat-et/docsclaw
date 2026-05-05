package tools

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

// mockProvider implements llm.Provider for testing the loop.
type mockProvider struct {
	responses []*llm.Response
	callIdx   int
}

func (m *mockProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (m *mockProvider) CompleteWithTools(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	if m.callIdx >= len(m.responses) {
		return &llm.Response{StopReason: llm.StopReasonEndTurn, Content: "done"}, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

func (m *mockProvider) StreamWithTools(_ context.Context,
	msgs []llm.Message, tools []llm.ToolDefinition,
	_ func(llm.StreamEvent)) (*llm.Response, error) {
	return m.CompleteWithTools(context.Background(), msgs, tools)
}

func (m *mockProvider) Model() string        { return "mock" }
func (m *mockProvider) ProviderName() string { return "mock" }

func TestRunToolLoopNoTools(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "Hello!",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			},
		},
	}
	registry := NewRegistry(nil)
	messages := []llm.Message{
		{Role: "user", Content: "Say hello"},
	}

	result, err := RunToolLoop(context.Background(), provider, messages,
		registry, DefaultLoopConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Fatalf("expected 'Hello!', got %q", result)
	}
}

func TestRunToolLoopWithToolCall(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				Content:    "I'll check that.",
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "test_tool", Args: map[string]any{}},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "The result is: ok",
				Usage:      llm.Usage{InputTokens: 150, OutputTokens: 30, TotalTokens: 180},
			},
		},
	}

	registry := NewRegistry(nil)
	registry.Register(&mockTool{name: "test_tool", output: "ok"})

	messages := []llm.Message{
		{Role: "user", Content: "Use the tool"},
	}

	result, err := RunToolLoop(context.Background(), provider, messages,
		registry, DefaultLoopConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "The result is: ok" {
		t.Fatalf("expected 'The result is: ok', got %q", result)
	}
}

func TestRunToolLoopMaxIterations(t *testing.T) {
	provider := &mockProvider{
		responses: make([]*llm.Response, 20),
	}
	for i := range provider.responses {
		provider.responses[i] = &llm.Response{
			StopReason: llm.StopReasonToolUse,
			ToolCalls: []llm.ToolCall{
				{ID: "tc", Name: "test_tool", Args: map[string]any{}},
			},
		}
	}

	registry := NewRegistry(nil)
	registry.Register(&mockTool{name: "test_tool", output: "ok"})

	cfg := DefaultLoopConfig()
	cfg.MaxIterations = 3

	messages := []llm.Message{
		{Role: "user", Content: "Loop forever"},
	}

	_, err := RunToolLoop(context.Background(), provider, messages,
		registry, cfg)
	if err == nil {
		t.Fatal("expected error for max iterations")
	}
}

func TestRunToolLoopUnknownTool(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "nonexistent", Args: map[string]any{}},
				},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "Tool not found, sorry.",
			},
		},
	}

	registry := NewRegistry(nil)
	messages := []llm.Message{
		{Role: "user", Content: "Use unknown tool"},
	}

	result, err := RunToolLoop(context.Background(), provider, messages,
		registry, DefaultLoopConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Tool not found, sorry." {
		t.Fatalf("expected error recovery response, got %q", result)
	}
}

func TestRunToolLoopTruncatesLargeOutput(t *testing.T) {
	largeOutput := strings.Repeat("x", 50000)
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "big_tool", Args: map[string]any{}},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "Done.",
				Usage:      llm.Usage{InputTokens: 200, OutputTokens: 10, TotalTokens: 210},
			},
		},
	}

	registry := NewRegistry(nil)
	registry.Register(&mockTool{name: "big_tool", output: largeOutput})

	cfg := DefaultLoopConfig()
	cfg.MaxResultBytes = 1000

	messages := []llm.Message{
		{Role: "user", Content: "Run the tool"},
	}

	result, err := RunToolLoop(context.Background(), provider, messages,
		registry, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Done." {
		t.Fatalf("expected 'Done.', got %q", result)
	}
}

func TestTruncateResult(t *testing.T) {
	// No truncation when under limit
	short := "hello"
	if got := truncateResult(short, 100); got != short {
		t.Fatalf("expected no truncation, got %q", got)
	}

	// No truncation when limit is 0 (disabled)
	long := strings.Repeat("x", 1000)
	if got := truncateResult(long, 0); got != long {
		t.Fatal("expected no truncation with limit 0")
	}

	// Truncation applies
	got := truncateResult(long, 100)
	if !strings.HasPrefix(got, strings.Repeat("x", 100)) {
		t.Fatal("expected output to start with 100 x's")
	}
	if !strings.Contains(got, "[Truncated: showing first 100 bytes of 1000 total]") {
		t.Fatalf("expected truncation notice, got %q", got)
	}

	// UTF-8 safe: don't split multi-byte characters
	// "日" is 3 bytes (E6 97 A5); cutting at byte 2 would produce invalid UTF-8
	utf8Str := strings.Repeat("日", 10) // 30 bytes
	got = truncateResult(utf8Str, 5)
	// Should back up to nearest valid rune boundary (3 bytes = 1 char)
	if !strings.HasPrefix(got, "日") {
		t.Fatal("expected UTF-8 safe prefix")
	}
	if strings.ContainsRune(got[:strings.Index(got, "\n")], '�') {
		t.Fatal("truncation produced invalid UTF-8")
	}
}

// slowMockTool sleeps before returning, used to verify parallel execution.
type slowMockTool struct {
	name   string
	output string
	delay  time.Duration
}

func (m *slowMockTool) Name() string               { return m.name }
func (m *slowMockTool) Description() string        { return "slow mock tool" }
func (m *slowMockTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (m *slowMockTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	time.Sleep(m.delay)
	return &ToolResult{Output: m.output}
}

func TestRunToolLoopParallelExecution(t *testing.T) {
	delay := 100 * time.Millisecond
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "slow_a", Args: map[string]any{}},
					{ID: "tc2", Name: "slow_b", Args: map[string]any{}},
					{ID: "tc3", Name: "slow_c", Args: map[string]any{}},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "All done.",
				Usage:      llm.Usage{InputTokens: 200, OutputTokens: 10, TotalTokens: 210},
			},
		},
	}

	registry := NewRegistry(nil)
	registry.Register(&slowMockTool{name: "slow_a", output: "a", delay: delay})
	registry.Register(&slowMockTool{name: "slow_b", output: "b", delay: delay})
	registry.Register(&slowMockTool{name: "slow_c", output: "c", delay: delay})

	hook := &mockHook{}
	cfg := DefaultLoopConfig()
	cfg.Hook = hook

	messages := []llm.Message{
		{Role: "user", Content: "Run all tools"},
	}

	start := time.Now()
	result, err := RunToolLoop(context.Background(), provider, messages,
		registry, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "All done." {
		t.Fatalf("expected 'All done.', got %q", result)
	}
	// Sequential would take 3*delay (300ms). Parallel should finish in ~delay.
	if elapsed >= 2*delay {
		t.Fatalf("expected parallel execution under %v, took %v", 2*delay, elapsed)
	}
	// Verify all three tools were executed (order may vary due to parallelism)
	if len(hook.afterCalls) != 3 {
		t.Fatalf("expected 3 AfterToolCall invocations, got %d", len(hook.afterCalls))
	}
	// Verify each tool's result matches its expected output
	resultsByName := make(map[string]string)
	for i, name := range hook.afterCalls {
		resultsByName[name] = hook.afterResults[i].Output
	}
	for _, tc := range []struct{ name, output string }{
		{"slow_a", "a"}, {"slow_b", "b"}, {"slow_c", "c"},
	} {
		if got := resultsByName[tc.name]; got != tc.output {
			t.Fatalf("tool %s: expected output %q, got %q", tc.name, tc.output, got)
		}
	}
}

// mockHook records Before/After calls for verification.
type mockHook struct {
	mu           sync.Mutex
	beforeCalls  []string
	afterCalls   []string
	afterResults []*ToolResult
	denyTool     string
}

func (h *mockHook) BeforeToolCall(_ context.Context, name string, _ map[string]any) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeCalls = append(h.beforeCalls, name)
	if name == h.denyTool {
		return false, "denied by test"
	}
	return true, ""
}

func (h *mockHook) AfterToolCall(_ context.Context, name string, _ map[string]any, result *ToolResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.afterCalls = append(h.afterCalls, name)
	h.afterResults = append(h.afterResults, result)
}

func TestAfterToolCallHook(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "test_tool", Args: map[string]any{}},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "Done.",
				Usage:      llm.Usage{InputTokens: 150, OutputTokens: 10, TotalTokens: 160},
			},
		},
	}

	registry := NewRegistry(nil)
	registry.Register(&mockTool{name: "test_tool", output: "result_value"})

	hook := &mockHook{}
	cfg := DefaultLoopConfig()
	cfg.Hook = hook

	messages := []llm.Message{
		{Role: "user", Content: "Use the tool"},
	}

	_, err := RunToolLoop(context.Background(), provider, messages, registry, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hook.beforeCalls) != 1 || hook.beforeCalls[0] != "test_tool" {
		t.Fatalf("expected BeforeToolCall for test_tool, got %v", hook.beforeCalls)
	}
	if len(hook.afterCalls) != 1 || hook.afterCalls[0] != "test_tool" {
		t.Fatalf("expected AfterToolCall for test_tool, got %v", hook.afterCalls)
	}
	if hook.afterResults[0].Output != "result_value" {
		t.Fatalf("expected result_value, got %q", hook.afterResults[0].Output)
	}
}

func TestBeforeToolCallHookDenial(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				StopReason: llm.StopReasonToolUse,
				ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: "blocked_tool", Args: map[string]any{}},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			},
			{
				StopReason: llm.StopReasonEndTurn,
				Content:    "Tool was denied.",
				Usage:      llm.Usage{InputTokens: 150, OutputTokens: 10, TotalTokens: 160},
			},
		},
	}

	registry := NewRegistry(nil)
	registry.Register(&mockTool{name: "blocked_tool", output: "should not see this"})

	hook := &mockHook{denyTool: "blocked_tool"}
	cfg := DefaultLoopConfig()
	cfg.Hook = hook

	messages := []llm.Message{
		{Role: "user", Content: "Use the tool"},
	}

	result, err := RunToolLoop(context.Background(), provider, messages, registry, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Tool was denied." {
		t.Fatalf("expected 'Tool was denied.', got %q", result)
	}
	// AfterToolCall should NOT be called for denied tools
	if len(hook.afterCalls) != 0 {
		t.Fatalf("expected no AfterToolCall for denied tool, got %v", hook.afterCalls)
	}
}
