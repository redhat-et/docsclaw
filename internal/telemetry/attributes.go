package telemetry

import "go.opentelemetry.io/otel/attribute"

// Span attribute keys for DocsClaw traces.
// Grouped by domain; prefixed to avoid collision with OTel semconv.

// LLM attributes (on "llm.call" and iteration spans).
var (
	AttrLLMProvider        = attribute.Key("llm.provider")
	AttrLLMModel           = attribute.Key("llm.model")
	AttrLLMInputTokens     = attribute.Key("llm.usage.input_tokens")
	AttrLLMOutputTokens    = attribute.Key("llm.usage.output_tokens")
	AttrLLMTotalTokens     = attribute.Key("llm.usage.total_tokens")
	AttrLLMCacheReadTokens = attribute.Key("llm.usage.cache_read_tokens")
	AttrLLMCacheWriteTokens = attribute.Key("llm.usage.cache_write_tokens")
	AttrLLMStopReason      = attribute.Key("llm.stop_reason")
)

// Agentic loop attributes (on "agent.loop" spans).
var (
	AttrLoopIteration     = attribute.Key("agent.loop.iteration")
	AttrLoopMaxIterations = attribute.Key("agent.loop.max_iterations")
	AttrLoopToolCount     = attribute.Key("agent.loop.tool_count")
)

// Tool attributes (on "tool.execute" spans).
var (
	AttrToolName       = attribute.Key("tool.name")
	AttrToolError      = attribute.Key("tool.error")
	AttrToolDenied     = attribute.Key("tool.denied")
	AttrToolDenyReason = attribute.Key("tool.deny_reason")
)

// Request/session attributes.
var (
	AttrSessionID = attribute.Key("session.id")
	AttrAgentName = attribute.Key("agent.name")
)
