package llm

import "fmt"

// Provider constructor functions, registered by internal packages via init().
var (
	newAnthropicProvider    func(Config) (Provider, error)
	newOpenAICompatProvider func(Config) (Provider, error)
)

// RegisterAnthropicProvider registers the Anthropic provider constructor.
func RegisterAnthropicProvider(fn func(Config) (Provider, error)) {
	newAnthropicProvider = fn
}

// RegisterOpenAICompatProvider registers the OpenAI-compatible provider constructor.
func RegisterOpenAICompatProvider(fn func(Config) (Provider, error)) {
	newOpenAICompatProvider = fn
}

// NewProvider creates a Provider based on the config.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case ProviderAnthropic:
		if newAnthropicProvider == nil {
			return nil, fmt.Errorf("anthropic provider not registered (import _ \"github.com/redhat-et/docsclaw/internal/anthropic\")")
		}
		return newAnthropicProvider(cfg)
	case ProviderOpenAI, ProviderLiteLLM, "":
		if newOpenAICompatProvider == nil {
			return nil, fmt.Errorf("openai provider not registered (import _ \"github.com/redhat-et/docsclaw/internal/openai\")")
		}
		return newOpenAICompatProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
}
