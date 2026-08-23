package memorytool

import (
	"context"
	"strings"

	"github.com/redhat-et/docsclaw/pkg/memory"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// NewRememberTool creates a tool that appends entries to long-term memory.
func NewRememberTool(store memory.Store) tools.Tool {
	return &rememberTool{store: store}
}

type rememberTool struct {
	store memory.Store
}

func (t *rememberTool) Name() string { return "remember" }

func (t *rememberTool) Description() string {
	return "Append a fact, observation, or decision to long-term memory. " +
		"Use this to preserve information that should survive the current session."
}

func (t *rememberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entry": map[string]any{
				"type":        "string",
				"description": "The fact or observation to remember",
			},
		},
		"required": []string{"entry"},
	}
}

func (t *rememberTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	entry, _ := args["entry"].(string)
	if strings.TrimSpace(entry) == "" {
		return tools.Errorf("entry is required")
	}

	if err := t.store.Remember(ctx, entry); err != nil {
		return tools.Errorf("failed to remember: %s", err)
	}
	return tools.OK("Recorded in long-term memory.")
}
