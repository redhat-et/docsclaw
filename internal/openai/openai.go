package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func init() {
	llm.RegisterOpenAICompatProvider(func(cfg llm.Config) (llm.Provider, error) {
		return NewOpenAICompatProvider(cfg)
	})
}

// OpenAICompatProvider implements the Provider interface for OpenAI-compatible APIs
// including OpenAI, Azure OpenAI, LiteLLM, vLLM, and other compatible services.
type OpenAICompatProvider struct {
	baseURL      string
	apiKey       string
	model        string
	maxTokens    int
	timeout      time.Duration
	client       *http.Client
	providerName string
}

// openAIChatRequest represents the request body for OpenAI chat completions API
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

// openAIMessage represents a message in the OpenAI chat format
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIUsage represents token usage from the OpenAI API.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIChatResponse represents the response from OpenAI chat completions API
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage openAIUsage  `json:"usage"`
	Error *openAIError `json:"error,omitempty"`
}

// openAIError represents an error response from the OpenAI API
type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// openAIToolCall represents a tool call in the OpenAI response.
type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string
	} `json:"function"`
}

// openAIToolDef represents a tool definition for the OpenAI API.
type openAIToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// openAIStreamChunk represents a single SSE chunk from the OpenAI streaming API.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                  `json:"role,omitempty"`
			Content   string                  `json:"content,omitempty"`
			ToolCalls []openAIStreamToolDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
}

// openAIStreamToolDelta represents an incremental tool call delta in a streaming response.
type openAIStreamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// openAIStreamRequest is the request body for streaming chat completions.
type openAIStreamRequest struct {
	Model     string            `json:"model"`
	Messages  []json.RawMessage `json:"messages"`
	Tools     []openAIToolDef   `json:"tools,omitempty"`
	MaxTokens int               `json:"max_tokens,omitempty"`
	Stream    bool              `json:"stream"`
}

// NewOpenAICompatProvider creates a new OpenAI-compatible LLM provider
func NewOpenAICompatProvider(cfg llm.Config) (*OpenAICompatProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.BaseURL == "" {
		if cfg.Provider == llm.ProviderLiteLLM {
			return nil, fmt.Errorf("base_url is required for LiteLLM provider")
		}
		cfg.BaseURL = llm.DefaultOpenAIBaseURL
	}

	// Ensure base URL doesn't have trailing slash
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")

	if cfg.Model == "" {
		switch cfg.Provider {
		case llm.ProviderLiteLLM:
			cfg.Model = llm.DefaultLiteLLMModel
		default:
			cfg.Model = llm.DefaultOpenAIModel
		}
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = llm.DefaultMaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = llm.DefaultTimeout
	}

	providerName := cfg.Provider
	if providerName == "" {
		providerName = llm.ProviderOpenAI
	}

	return &OpenAICompatProvider{
		baseURL:      cfg.BaseURL,
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		maxTokens:    cfg.MaxTokens,
		timeout:      time.Duration(cfg.Timeout) * time.Second,
		client:       &http.Client{},
		providerName: providerName,
	}, nil
}

// Complete sends a message to the OpenAI-compatible API and returns the response
func (p *OpenAICompatProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	reqBody := openAIChatRequest{
		Model: p.model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: p.maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIChatResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// CompleteWithTools sends a multi-turn conversation with tool
// definitions to the OpenAI-compatible API.
func (p *OpenAICompatProvider) CompleteWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return p.StreamWithTools(ctx, messages, tools, nil)
}

// StreamWithTools is like CompleteWithTools but calls onEvent with
// incremental text deltas as they arrive. If onEvent is nil, events
// are silently discarded. Returns the fully accumulated Response.
func (p *OpenAICompatProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	onEvent func(llm.StreamEvent)) (*llm.Response, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Convert messages to OpenAI format
	var openaiMsgs []json.RawMessage
	for _, msg := range messages {
		var raw []byte
		var err error

		switch msg.Role {
		case "system", "user":
			raw, err = json.Marshal(map[string]string{
				"role":    msg.Role,
				"content": msg.Content,
			})
		case "assistant":
			assistantMsg := map[string]any{
				"role": "assistant",
			}
			if msg.Content != "" {
				assistantMsg["content"] = msg.Content
			}
			if len(msg.ToolCalls) > 0 {
				var tcs []openAIToolCall
				for _, tc := range msg.ToolCalls {
					argsJSON, marshalErr := json.Marshal(tc.Args)
					if marshalErr != nil {
						return nil, fmt.Errorf("failed to marshal tool args: %w", marshalErr)
					}
					tcs = append(tcs, openAIToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      tc.Name,
							Arguments: string(argsJSON),
						},
					})
				}
				assistantMsg["tool_calls"] = tcs
			}
			raw, err = json.Marshal(assistantMsg)
		case "tool":
			for _, tr := range msg.ToolResults {
				toolMsg, marshalErr := json.Marshal(map[string]string{
					"role":         "tool",
					"tool_call_id": tr.ToolUseID,
					"content":      tr.Output,
				})
				if marshalErr != nil {
					return nil, fmt.Errorf("failed to marshal tool result: %w", marshalErr)
				}
				openaiMsgs = append(openaiMsgs, toolMsg)
			}
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("failed to marshal message: %w", err)
		}
		openaiMsgs = append(openaiMsgs, raw)
	}

	// Convert tool definitions
	var openaiTools []openAIToolDef
	for _, td := range tools {
		def := openAIToolDef{Type: "function"}
		def.Function.Name = td.Name
		def.Function.Description = td.Description
		def.Function.Parameters = td.Parameters
		openaiTools = append(openaiTools, def)
	}

	reqBody := openAIStreamRequest{
		Model:     p.model,
		Messages:  openaiMsgs,
		Tools:     openaiTools,
		MaxTokens: p.maxTokens,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	httpResp, err := p.client.Do(req)
	if err != nil {
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: fmt.Sprintf("API request failed: %v", err),
			})
		}
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		errMsg := fmt.Sprintf("API returned status %d: %s",
			httpResp.StatusCode, string(body))
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: errMsg,
			})
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Parse SSE stream
	var contentBuf strings.Builder
	var finishReason string
	var usage *openAIUsage

	// Track accumulated tool calls by index
	type pendingToolCall struct {
		ID       string
		Name     string
		ArgsJSON strings.Builder
	}
	toolCalls := make(map[int]*pendingToolCall)

	scanner := bufio.NewScanner(httpResp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines (SSE separators)
		if line == "" {
			continue
		}

		// End of stream
		if line == "data: [DONE]" {
			break
		}

		// Only process data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if len(chunk.Choices) == 0 {
			// May be a usage-only chunk
			if chunk.Usage != nil {
				usage = chunk.Usage
			}
			continue
		}

		choice := chunk.Choices[0]

		// Accumulate text content
		if choice.Delta.Content != "" {
			contentBuf.WriteString(choice.Delta.Content)
			if onEvent != nil {
				onEvent(llm.StreamEvent{
					Type:    llm.StreamEventTextDelta,
					Content: choice.Delta.Content,
				})
			}
		}

		// Accumulate tool call deltas
		for _, tcd := range choice.Delta.ToolCalls {
			tc, ok := toolCalls[tcd.Index]
			if !ok {
				tc = &pendingToolCall{}
				toolCalls[tcd.Index] = tc
			}
			if tcd.ID != "" {
				tc.ID = tcd.ID
			}
			if tcd.Function.Name != "" {
				tc.Name = tcd.Function.Name
			}
			if tcd.Function.Arguments != "" {
				tc.ArgsJSON.WriteString(tcd.Function.Arguments)
			}
		}

		// Capture finish reason
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}

		// Capture usage if present in this chunk
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if err := scanner.Err(); err != nil {
		if onEvent != nil {
			onEvent(llm.StreamEvent{
				Type:    llm.StreamEventError,
				Content: fmt.Sprintf("stream read error: %v", err),
			})
		}
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	// Build response
	resp := &llm.Response{
		Content: contentBuf.String(),
	}

	if finishReason == "tool_calls" {
		resp.StopReason = llm.StopReasonToolUse
	} else {
		resp.StopReason = llm.StopReasonEndTurn
	}

	if usage != nil {
		resp.Usage = llm.Usage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		}
	}

	// Convert accumulated tool calls to response
	for i := 0; i < len(toolCalls); i++ {
		tc, ok := toolCalls[i]
		if !ok {
			continue
		}
		var args map[string]any
		rawArgs := tc.ArgsJSON.String()
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			args = map[string]any{"_raw": rawArgs}
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: args,
		})
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
func (p *OpenAICompatProvider) Model() string {
	return p.model
}

// ProviderName returns the name of this provider
func (p *OpenAICompatProvider) ProviderName() string {
	return p.providerName
}

// Ensure OpenAICompatProvider implements Provider interface
var _ llm.Provider = (*OpenAICompatProvider)(nil)
