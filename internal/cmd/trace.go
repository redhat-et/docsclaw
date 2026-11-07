package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/redhat-et/docsclaw/pkg/tools"
)

// traceHook writes a compact trace of tool calls to stderr.
type traceHook struct {
	w  io.Writer
	mu sync.Mutex
}

func (h *traceHook) BeforeToolCall(_ context.Context, name string,
	args map[string]any) (bool, string) {

	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = []byte("{}")
	}
	h.mu.Lock()
	_, _ = fmt.Fprintf(h.w, "→ %s %s\n", name, encoded)
	h.mu.Unlock()
	return true, ""
}

func (h *traceHook) AfterToolCall(_ context.Context, name string,
	_ map[string]any, result *tools.ToolResult) {

	status := "ok"
	if result.Error {
		status = "error"
	}
	output := strings.NewReplacer("\r", " ", "\n", " ").Replace(result.Output)
	if len(output) > 200 {
		output = output[:200]
	}
	h.mu.Lock()
	_, _ = fmt.Fprintf(h.w, "← %s %s: %s\n", name, status, output)
	h.mu.Unlock()
}
