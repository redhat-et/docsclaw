# OpenAI-Compatible API Endpoint

**Date**: 2026-05-05
**Issue**: #54
**Status**: Approved

## Goal

Add an OpenAI-compatible Chat Completions API to DocsClaw so users can
connect existing chat UIs (Open WebUI, LibreChat, TypingMind) instead
of building a custom web frontend. When deployed on OpenShift, admins
give users the endpoint URL and they use their preferred client.

## Approach

**Direct LLM + Tool Loop** (Approach A). The OpenAI endpoint calls
the LLM provider and tool registry directly, bypassing A2A. This
avoids double conversion (OpenAI-to-A2A-to-LLM) and maps naturally to
the chat completions request/response model.

## Package structure

New package `internal/openaiapi/` with three files:

| File | Responsibility |
|------|---------------|
| `types.go` | OpenAI request/response structs |
| `handler.go` | HTTP handlers for all three endpoints |
| `stream.go` | SSE streaming logic |

## Routes

Registered in `internal/cmd/serve.go` on the existing port 8000
alongside A2A routes:

| Method | Path | Handler |
|--------|------|---------|
| POST | `/v1/chat/completions` | `ChatCompletionHandler` |
| GET | `/v1/models` | `ModelsHandler` |
| GET | `/v1/skills` | `SkillsHandler` |

No new dependencies beyond stdlib and existing `pkg/llm`, `pkg/tools`,
`pkg/skills` packages.

## Request/response flow

### Non-streaming (`stream: false` or omitted)

```
Client POST /v1/chat/completions
  ├─ Parse ChatCompletionRequest
  │   └─ Extract messages[] (ignore "model" field)
  ├─ Convert OpenAI messages → []llm.Message
  │   ├─ System messages → appended after DocsClaw's system prompt
  │   └─ User/assistant messages → mapped directly
  ├─ Phase 2 (tools enabled)?
  │   ├─ Yes → RunToolLoop(provider, messages, registry, config)
  │   └─ No  → provider.Complete(systemPrompt, userMessage)
  └─ Convert llm.Response → ChatCompletionResponse
      └─ Return JSON {id, model, choices[], usage}
```

### Streaming (`stream: true`)

Returns SSE with `Content-Type: text/event-stream`. Each chunk:

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",
       "choices":[{"delta":{"content":"word"},"index":0}]}
```

Final events:

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",
       "choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}

data: [DONE]
```

**V1 implementation**: Simulated streaming. The full response is
obtained from the provider/tool loop, then chunked into word-sized
pieces flushed over SSE. This provides progressive rendering UX
without changes to the provider interface.

**Future**: Add `CompleteWithToolsStream` to `Provider` interface for
true token-by-token streaming.

## Design decisions

### System prompt merging

DocsClaw's configured system prompt always takes priority. If the
client sends a system message, it is appended after DocsClaw's prompt.
The agent personality is preserved; the client can add context.

### Model selection

The `model` field in requests is ignored. DocsClaw uses its configured
provider and model, set at deployment time via config/env vars. The
`/v1/models` endpoint returns a single entry. To offer multiple
models, deploy multiple DocsClaw instances with different ConfigMaps.

### Tool visibility

V1: Server-side only. Tool execution happens inside the agentic loop;
the client sees only the final answer. This is a strict subset of the
pass-through approach. Adding tool-call deltas to the SSE stream is a
follow-up that requires no structural changes.

### Session management

Client-side history (standard OpenAI behavior). The client sends all
messages each time. No server-side session state for this endpoint.

### Authentication

None in DocsClaw itself. Auth is an infrastructure concern handled by
OAuth proxy sidecar on OpenShift, or reverse proxy (nginx, Envoy)
elsewhere. This keeps DocsClaw focused on agent runtime, not auth.

### `/v1/models` response

```json
{
  "object": "list",
  "data": [
    {
      "id": "docsclaw",
      "object": "model",
      "created": 1714900000,
      "owned_by": "docsclaw",
      "description": "DocsClaw agent (backed by claude-sonnet-4-20250514)"
    }
  ]
}
```

The `id` is `"docsclaw"` (stable). The description includes the
actual backing model from `provider.Model()`. The agent name from the
agent card is used if available.

### `/v1/skills` response

```json
{
  "skills": [
    {
      "id": "summarize",
      "name": "Summarize Document",
      "description": "Summarizes technical documents into key points"
    }
  ]
}
```

Loaded from the same agent card config that
`/.well-known/agent-card.json` serves.

## Error handling

Standard OpenAI error format:

```json
{
  "error": {
    "message": "description",
    "type": "error_type",
    "code": "error_code"
  }
}
```

Error mapping:

| Scenario | HTTP | Type | Code |
|----------|------|------|------|
| Malformed JSON / missing messages | 400 | `invalid_request_error` | `invalid_request` |
| LLM provider error | 502 | `server_error` | `upstream_error` |
| Tool loop exceeds max iterations | 500 | `server_error` | `internal_error` |

Mid-stream errors: send error event then `data: [DONE]`.

## Testing

**Unit tests** (`internal/openaiapi/`):
- Message conversion (OpenAI ↔ `llm.Message`)
- Response formatting (`llm.Response` → `ChatCompletionResponse`)
- SSE chunk encoding (correct `data:` lines and `[DONE]`)
- Error formatting (correct JSON for each error type)

**Integration tests** (mock LLM provider):
- Full HTTP round-trip with mock provider
- Streaming and non-streaming paths
- Phase 1 (no tools) and phase 2 (tool loop) paths

**Manual verification**:
- Connect Open WebUI to running endpoint
- Verify model appears in selector
- Verify streaming renders progressively

## Future enhancements

- True token-by-token streaming via `Provider` interface extension
- Tool-call visibility in SSE stream (pass-through deltas)
- Responses API (`/v1/responses`) when client support matures
- API key auth (if needed outside sidecar pattern)
