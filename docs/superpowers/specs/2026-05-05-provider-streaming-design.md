# LLM Provider Streaming

**Date**: 2026-05-05
**Issue**: #44
**Status**: Approved

## Goal

Add streaming support to the `llm.Provider` interface so LLM
responses are delivered token-by-token instead of waiting for the
full response. Wire real streaming into the OpenAI-compatible API
endpoint for immediate UX improvement in Open WebUI and other
clients.

## Scope

This iteration covers:
- `StreamWithTools` method on the `Provider` interface
- Anthropic provider streaming (SDK's `Messages.NewStreaming`)
- OpenAI provider streaming (SSE parsing)
- OpenAI API endpoint real streaming (phase 1 only)

Out of scope (follow-ups):
- Chat TUI incremental rendering
- A2A server streaming events
- Agentic tool loop streaming (#56)

## StreamEvent type

New types in `pkg/llm/types.go`:

```go
type StreamEventType string

const (
    StreamEventTextDelta StreamEventType = "text_delta"
    StreamEventDone      StreamEventType = "done"
    StreamEventError     StreamEventType = "error"
)

type StreamEvent struct {
    Type    StreamEventType
    Content string // text chunk for TextDelta, error message for Error
    Usage   Usage  // populated only for Done events
}
```

## Provider interface change

New method in `pkg/llm/provider.go`:

```go
type Provider interface {
    Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
    CompleteWithTools(ctx context.Context, messages []Message,
        tools []ToolDefinition) (*Response, error)
    StreamWithTools(ctx context.Context, messages []Message,
        tools []ToolDefinition, onEvent func(StreamEvent)) (*Response, error)
    Model() string
    ProviderName() string
}
```

### Contract

- Calls `onEvent` with `TextDelta` events as tokens arrive from
  the API.
- Calls `onEvent` with `Done` (including final `Usage`) when the
  response is complete.
- Calls `onEvent` with `Error` if an error occurs mid-stream.
- Returns the accumulated `*Response` — identical to what
  `CompleteWithTools` returns. Contains `Content`, `StopReason`,
  `ToolCalls`, and `Usage`.
- If `onEvent` is nil, events are silently discarded (no-op).
- Tool call arguments are accumulated internally. Only complete
  tool calls appear in the returned `*Response`. No tool-call
  events are emitted via the callback in v1.

### CompleteWithTools simplification

After implementing `StreamWithTools`, both providers delegate
`CompleteWithTools` to it:

```go
func (p *Provider) CompleteWithTools(ctx context.Context,
    messages []Message, tools []ToolDefinition) (*Response, error) {
    return p.StreamWithTools(ctx, messages, tools, nil)
}
```

This eliminates duplicate request-building and response-parsing
code in each provider.

## Anthropic provider

In `internal/anthropic/anthropic.go`:

- Use `p.client.Messages.NewStreaming(ctx, params)` instead of
  `Messages.New()`.
- Iterate over the stream. Map SDK events to `StreamEvent`:
  - `ContentBlockDelta` with text → `onEvent(TextDelta)`
  - `ContentBlockDelta` with tool input JSON → accumulate
  - `MessageStop` → `onEvent(Done)` with usage
- Accumulate text content and tool calls into `*Response`.
- Check `ctx.Err()` between events for early cancellation.

## OpenAI provider

In `internal/openai/openai.go`:

- Add `"stream": true` to the request JSON body.
- Instead of `io.ReadAll(resp.Body)`, read the response body
  line-by-line with `bufio.Scanner`.
- Parse SSE format:
  - Lines starting with `data: ` contain JSON chunks.
  - `data: [DONE]` signals end of stream.
  - Each chunk's `choices[0].delta.content` → `onEvent(TextDelta)`.
  - Tool call deltas → accumulate by tool call index.
- After `[DONE]`: `onEvent(Done)` with usage (if provided by
  the backend).
- Accumulate into `*Response` and return.
- Some OpenAI-compatible backends (vLLM, LiteLLM) may not include
  usage in streaming chunks. Usage fields will be zero in that
  case — this is acceptable.

## OpenAI API endpoint

In `internal/openaiapi/handler.go`:

**Phase 1 (no tools)** — real streaming:
- When `req.Stream` is true and no tool registry is configured,
  call `provider.StreamWithTools` with a callback that writes each
  `TextDelta` as an SSE chunk via `writeChunk()`.
- The callback also handles the initial role chunk and final
  `finish_reason` chunk.

**Phase 2 (tools)** — simulated streaming (unchanged):
- The agentic loop runs to completion via `RunToolLoop` (which
  uses `CompleteWithTools` internally).
- The final response is streamed via the existing `StreamResponse`
  function (word-by-word tokenization).
- Real phase 2 streaming is tracked in #56.

**Non-streaming requests** — no change:
- Call `StreamWithTools` with nil callback, return JSON response.

## Error handling

**Provider errors mid-stream:**
- Provider calls `onEvent(StreamEvent{Type: Error, Content: msg})`
  and returns the error.
- OpenAI handler: if SSE headers already sent, writes `StreamError`
  followed by `[DONE]`. If headers not yet sent, returns JSON error.

**Client disconnect:**
- Request context is cancelled. Provider's streaming loop checks
  `ctx.Err()` and exits early. Standard Go HTTP behavior.

**Nil callback:**
- Both providers treat nil as no-op. This enables the
  `CompleteWithTools` delegation pattern.

## Testing

**Unit tests** (`pkg/llm/`):
- StreamEvent type and constants exist.

**OpenAI provider** (`internal/openai/`):
- `httptest.Server` returning SSE responses. Verify
  `StreamWithTools` calls `onEvent` with correct `TextDelta`
  events and returns accumulated `*Response`.

**OpenAI API handler** (`internal/openaiapi/`):
- Existing streaming tests continue to pass (format unchanged).
- New test with mock provider verifying phase 1 real streaming
  pipes provider events to SSE output.

**Anthropic provider** (`internal/anthropic/`):
- Manual integration testing with real API key (same as existing
  `CompleteWithTools` — no unit tests for SDK interaction).

**Manual verification:**
- Start server, connect Open WebUI, verify tokens appear
  one-by-one in phase 1 instead of word-sized bursts.
