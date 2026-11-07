package cmd

import (
	"context"
	"sync"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

type completeCall struct {
	SystemPrompt string
	UserPrompt   string
}

// fakeProvider is a scripted llm.Provider for tests. It records every
// Complete call and serves queued responses from CompleteWithTools. When
// the queue is empty it returns a plain end-turn response containing
// "answer".
type fakeProvider struct {
	mu            sync.Mutex
	completeCalls []completeCall
	toolMessages  [][]llm.Message
	responses     []*llm.Response
	err           error
}

func (p *fakeProvider) Complete(_ context.Context, systemPrompt, userPrompt string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completeCalls = append(p.completeCalls, completeCall{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if p.err != nil {
		return "", p.err
	}
	return "answer", nil
}

func (p *fakeProvider) CompleteWithTools(_ context.Context,
	messages []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {

	p.mu.Lock()
	defer p.mu.Unlock()
	p.toolMessages = append(p.toolMessages, append([]llm.Message(nil), messages...))
	if p.err != nil {
		return nil, p.err
	}
	if len(p.responses) == 0 {
		return &llm.Response{StopReason: llm.StopReasonEndTurn, Content: "answer"}, nil
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func (p *fakeProvider) StreamWithTools(ctx context.Context,
	messages []llm.Message, tools []llm.ToolDefinition,
	_ func(llm.StreamEvent)) (*llm.Response, error) {

	return p.CompleteWithTools(ctx, messages, tools)
}

func (p *fakeProvider) Model() string        { return "fake-model" }
func (p *fakeProvider) ProviderName() string { return "fake" }
