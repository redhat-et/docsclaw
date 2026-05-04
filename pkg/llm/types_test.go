package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolDefinitionJSON(t *testing.T) {
	td := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file and return its contents",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
	}

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtrip ToolDefinition
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if roundtrip.Name != "read_file" {
		t.Fatalf("expected read_file, got %q", roundtrip.Name)
	}
}

func TestResponseWithToolCalls(t *testing.T) {
	resp := &Response{
		StopReason: StopReasonToolUse,
		Content:    "I'll read the file.",
		ToolCalls: []ToolCall{
			{
				ID:   "tc_001",
				Name: "read_file",
				Args: map[string]any{"path": "main.go"},
			},
		},
		Usage: Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}

	if !resp.HasToolCalls() {
		t.Fatal("expected HasToolCalls to be true")
	}
	if resp.Usage.TotalTokens != 150 {
		t.Fatalf("expected TotalTokens 150, got %d", resp.Usage.TotalTokens)
	}
}

func TestResponseWithoutToolCalls(t *testing.T) {
	resp := &Response{
		StopReason: StopReasonEndTurn,
		Content:    "Here is the summary.",
	}

	if resp.HasToolCalls() {
		t.Fatal("expected HasToolCalls to be false")
	}
}

func TestUsageJSON(t *testing.T) {
	usage := Usage{
		InputTokens:      1000,
		OutputTokens:     500,
		CacheReadTokens:  200,
		CacheWriteTokens: 300,
		TotalTokens:      1500,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtrip Usage
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if roundtrip != usage {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", roundtrip, usage)
	}
}

func TestUsageOmitsZeroCacheTokens(t *testing.T) {
	usage := Usage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "cache_read_tokens") {
		t.Fatal("expected cache_read_tokens to be omitted when zero")
	}
	if strings.Contains(s, "cache_write_tokens") {
		t.Fatal("expected cache_write_tokens to be omitted when zero")
	}
}
