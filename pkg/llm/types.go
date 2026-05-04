package llm

// StopReason indicates why the LLM stopped generating.
type StopReason string

const (
	StopReasonEndTurn StopReason = "end_turn"
	StopReasonToolUse StopReason = "tool_use"
)

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents the LLM's request to invoke a tool.
type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// ToolResultContent carries the result of a tool execution
// back to the LLM.
type ToolResultContent struct {
	ToolUseID string `json:"tool_use_id"`
	Output    string `json:"output"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Message represents a conversation message for multi-turn
// tool-use interactions.
type Message struct {
	Role        string              // "user", "assistant", "tool"
	Content     string              // text content (for user/assistant)
	ToolCalls   []ToolCall          // tool calls (assistant only)
	ToolResults []ToolResultContent // tool results (tool role only)
}

// Usage tracks token consumption from an LLM response.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens"`
}

// Response is the LLM's reply from CompleteWithTools.
type Response struct {
	StopReason StopReason
	Content    string     // text content (may be empty if only tool calls)
	ToolCalls  []ToolCall // tool calls (empty if end_turn)
	Usage      Usage
}

// HasToolCalls returns true if the response contains tool call
// requests.
func (r *Response) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}
