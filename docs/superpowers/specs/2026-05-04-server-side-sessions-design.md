# Server-Side Session Management Design

**Date**: 2026-05-04
**Issue**: #5 (Phase 1)
**Status**: Draft

## Problem

DocsClaw's server is stateless. The chat TUI works around this by
prepending the full conversation as plaintext into every A2A request.
This wastes tokens (the entire history is re-tokenized each turn),
loses message structure (roles become text labels), and doesn't
persist across client restarts.

## Design

### Session Store (`internal/session/store.go`)

In-memory store with TTL-based expiry:

```go
type Session struct {
    ID         string
    Messages   []llm.Message
    CreatedAt  time.Time
    LastActive time.Time
}

type Store struct {
    mu       sync.RWMutex
    sessions map[string]*Session
    ttl      time.Duration
}
```

**API**:
- `NewStore(ttl)` — constructor, default TTL 30 minutes
- `GetOrCreate(id, systemPrompt)` — returns existing session or
  creates one with the system prompt as the first message
- `Append(id, msg)` — adds a message and updates LastActive
- `Get(id)` — returns session or nil
- `StartReaper(ctx)` — background goroutine that deletes sessions
  idle beyond TTL, runs every minute

The store is safe for concurrent use (RWMutex).

### Executor changes (`internal/bridge/executor.go`)

The `MessageProcessor` type changes to accept full history:

```go
// Before
type MessageProcessor func(ctx context.Context, userMessage string) (string, error)

// After
type MessageProcessor func(ctx context.Context, messages []llm.Message) (string, error)
```

The executor gains a `Sessions` field (`*session.Store`). In
`Execute`:

1. Create or resume task — `execCtx.StoredTask` is nil on first
   call, non-nil on subsequent calls to the same taskId
2. Extract user text from `execCtx.Message`
3. Get session via `store.GetOrCreate(taskId, systemPrompt)`
4. Append user message to session
5. Call `ProcessMessage(ctx, session.Messages)`
6. Append assistant response to session
7. Return result as artifact

The executor also gains a `SystemPrompt` field so it can
initialize new sessions with the correct prompt.

### Serve command changes (`internal/cmd/serve.go`)

- Create session store: `sessions := session.NewStore(30 * time.Minute)`
- Start reaper: `sessions.StartReaper(ctx)`
- Set `executor.Sessions = sessions`
- Set `executor.SystemPrompt = systemPrompt + skillsSummary`
- Update `ProcessMessage` closure to accept `[]llm.Message`
  instead of building them from scratch:

```go
executor.ProcessMessage = func(ctx context.Context,
    messages []llm.Message) (string, error) {
    return tools.RunToolLoop(ctx, llmProvider, messages,
        toolRegistry, loopCfg)
}
```

### Client changes (`internal/bridge/client.go`, `internal/chat/model.go`)

**InvokeRequest** gains `TaskID string` field. When set, the A2A
client sends `message/send` with the existing task ID instead of
creating a new task.

**InvokeResult** gains `TaskID string` field, populated from the
A2A response task.

**Chat TUI** (`internal/chat/model.go`):
- Store `taskID` from first response
- Pass `taskID` in subsequent `InvokeRequest`
- When `taskID` is set, `buildMessageWithHistory` returns the
  raw text without prepending history (server has it)
- Display still shows local messages for scroll buffer

### Session lifecycle

```
TUI: "hello"  →  Server: no taskId → create session S1
                  S1.Messages = [system, user("hello")]
                  LLM call with S1.Messages
                  S1.Messages += [assistant("Hi!")]
                  Response includes taskId=S1

TUI: "how?"   →  Server: taskId=S1 → load session S1
                  S1.Messages += [user("how?")]
                  LLM call with S1.Messages
                  S1.Messages += [assistant("...")]
                  Response includes taskId=S1

(30 min idle) →  Reaper deletes S1
```

### Enablement

Sessions are always enabled in phase 2 (tool-use) mode. No
additional configuration needed.

## Files to modify or create

| File | Change |
|------|--------|
| `internal/session/store.go` | **New**: session store |
| `internal/session/store_test.go` | **New**: store tests |
| `internal/bridge/executor.go` | Add Sessions + SystemPrompt fields, session management in Execute |
| `internal/bridge/client.go` | Add TaskID to InvokeRequest/InvokeResult, pass to A2A |
| `internal/cmd/serve.go` | Create store, wire to executor, update ProcessMessage signature |
| `internal/chat/model.go` | Store taskID, skip history prepending when set |

## Testing

- `TestStoreGetOrCreate`: new session gets system prompt
- `TestStoreAppend`: messages accumulate, LastActive updates
- `TestStoreReaper`: expired sessions are deleted
- `TestStoreGetOrCreateExisting`: second call returns same session
- Integration: manual test with `docsclaw serve` + `docsclaw chat`
  verifying multi-turn conversation works without history prepending

## Future work

- Redis backend for multi-replica deployments
- SQLite for persistence across restarts
- Max message count per session (bounded memory)
- Context compaction integration (#5 Phase 3)
