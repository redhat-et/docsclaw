package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func init() {
	llm.RegisterAnthropicProvider(func(cfg llm.Config) (llm.Provider, error) {
		return NewAnthropicProvider(cfg)
	})
}

// AnthropicProvider wraps the Anthropic API client
type AnthropicProvider struct {
	client    anthropic.Client
	model     string
	maxTokens int
	timeout   time.Duration
}

// NewAnthropicProvider creates a new Anthropic LLM provider with the given configuration
func NewAnthropicProvider(cfg llm.Config) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.Model == "" {
		cfg.Model = llm.DefaultAnthropicModel
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = llm.DefaultMaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = llm.DefaultTimeout
	}

	client := anthropic.NewClient(
		option.WithAPIKey(cfg.APIKey),
	)

	return &AnthropicProvider{
		client:    client,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		timeout:   time.Duration(cfg.Timeout) * time.Second,
	}, nil
}

// Complete sends a message to the Claude API and returns the response
func (p *AnthropicProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	message, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: int64(p.maxTokens),
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}

	// Extract text content from response
	if len(message.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	var result string
	for _, block := range message.Content {
		if block.Type == "text" {
			result += block.Text
		}
	}

	return result, nil
}

// CompleteWithTools sends a multi-turn conversation with tool
// definitions to the Anthropic API.
func (p *AnthropicProvider) CompleteWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return p.StreamWithTools(ctx, messages, tools, nil)
}

// StreamWithTools is like CompleteWithTools but calls onEvent with
// incremental text deltas as they arrive. If onEvent is nil, events
// are silently discarded. Returns the fully accumulated Response.
func (p *AnthropicProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Convert messages to Anthropic format
	var anthropicMsgs []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			anthropicMsgs = append(anthropicMsgs,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Args, tc.Name))
			}
			anthropicMsgs = append(anthropicMsgs,
				anthropic.MessageParam{Role: "assistant", Content: blocks})
		case "tool":
			var blocks []anthropic.ContentBlockParamUnion
			for _, tr := range msg.ToolResults {
				blocks = append(blocks, anthropic.NewToolResultBlock(
					tr.ToolUseID, tr.Output, tr.IsError))
			}
			anthropicMsgs = append(anthropicMsgs,
				anthropic.MessageParam{Role: "user", Content: blocks})
		}
	}

	// Convert tool definitions
	var anthropicTools []anthropic.ToolUnionParam
	for _, td := range tools {
		// Extract properties and required from the parameters map
		properties := td.Parameters["properties"]
		var required []string
		switch v := td.Parameters["required"].(type) {
		case []string:
			required = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					required = append(required, s)
				}
			}
		}

		schema := anthropic.ToolInputSchemaParam{
			Properties: properties,
			Required:   required,
		}

		tool := anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        td.Name,
				Description: anthropic.String(td.Description),
				InputSchema: schema,
			},
		}
		anthropicTools = append(anthropicTools, tool)
	}

	// Build params
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: int64(p.maxTokens),
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}

	// Stream the response
	stream := p.client.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	accumulated := &anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := accumulated.Accumulate(event); err != nil {
			if onEvent != nil {
				onEvent(llm.StreamEvent{
					Type:    llm.StreamEventError,
					Content: fmt.Sprintf("accumulate error: %v", err),
				})
			}
			return nil, fmt.Errorf("stream accumulate failed: %w", err)
		}

		// Emit text deltas for content_block_delta events
		if event.Type == "content_block_delta" && event.Delta.Text != "" && onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventTextDelta,
				Content: event.Delta.Text,
			})
		}
	}
	if err := stream.Err(); err != nil {
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: fmt.Sprintf("stream error: %v", err),
			})
		}
		return nil, fmt.Errorf("API streaming failed: %w", err)
	}

	// Parse accumulated response
	resp := &llm.Response{
		Usage: llm.Usage{
			InputTokens:      int(accumulated.Usage.InputTokens),
			OutputTokens:     int(accumulated.Usage.OutputTokens),
			CacheReadTokens:  int(accumulated.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(accumulated.Usage.CacheCreationInputTokens),
			TotalTokens:      int(accumulated.Usage.InputTokens + accumulated.Usage.OutputTokens),
		},
	}
	switch accumulated.StopReason {
	case "tool_use":
		resp.StopReason = llm.StopReasonToolUse
	default:
		resp.StopReason = llm.StopReasonEndTurn
	}

	for _, block := range accumulated.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			// Parse the JSON RawMessage input into a map
			var args map[string]any
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &args); err != nil {
					return nil, fmt.Errorf("failed to parse tool input: %w", err)
				}
			}
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:   block.ID,
				Name: block.Name,
				Args: args,
			})
		}
	}

	// Emit done event with usage info
	if onEvent != nil {
		onEvent(llm.StreamEvent{
			Type:  llm.StreamEventDone,
			Usage: resp.Usage,
		})
	}

	return resp, nil
}

// Model returns the configured model name
func (p *AnthropicProvider) Model() string {
	return p.model
}

// ProviderName returns the name of this provider
func (p *AnthropicProvider) ProviderName() string {
	return llm.ProviderAnthropic
}

// Ensure AnthropicProvider implements Provider interface
var _ llm.Provider = (*AnthropicProvider)(nil)
