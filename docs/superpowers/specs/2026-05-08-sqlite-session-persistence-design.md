# SQLite Session Persistence Design

**Date**: 2026-05-08
**Status**: Draft
**Depends on**: Server-side sessions (#51, merged)

## Problem

The in-memory session store loses all conversation state when the
server restarts. For Kubernetes deployments where agents run as
lightweight pods, sessions must survive pod restarts (backed by a
PVC). This is the first step toward full agent memory.

## Approach

Extract a `SessionStore` interface from the concrete `Store` type.
The existing in-memory implementation becomes `MemoryStore`. A new
`SQLiteStore` implements the same interface with persistence via
`modernc.org/sqlite` (pure Go, no CGo).

## SessionStore Interface

```go
type SessionStore interface {
    GetOrCreate(id, systemPrompt string) (*Session, error)
    Get(id string) (*Session, error)
    Append(id string, msg llm.Message) error
    AppendAndSnapshot(id string, msg llm.Message) ([]llm.Message, error)
    Len() int
    StartReaper(ctx context.Context)
    Close() error
}
```

Methods gain `error` returns to accommodate I/O failures. The
existing `MemoryStore` returns nil errors. The `Session` struct
is unchanged.

## SQLite Schema

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    system_prompt TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    last_active   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    role         TEXT NOT NULL,
    content      TEXT,
    tool_calls   TEXT,
    tool_results TEXT,
    UNIQUE(session_id, seq)
);
```

- `tool_calls` and `tool_results` stored as JSON text columns,
  matching `llm.Message` structure
- `seq` provides explicit message ordering for restore
- Timestamps as RFC 3339 text
- `ON DELETE CASCADE` so reaping a session cleans up messages
- Database opens with `PRAGMA journal_mode=WAL` and
  `PRAGMA foreign_keys=ON`

## Write Strategy

- `GetOrCreate`: if session exists, load all messages from
  `messages` table ordered by `seq`; otherwise insert session
  row and system prompt message
- `Append` / `AppendAndSnapshot`: insert one message row and
  update `last_active`, both in one transaction
- `Reaper`: `DELETE FROM sessions WHERE last_active < ?`,
  cascade handles messages

## Resume Behavior

Seamless: restored sessions are indistinguishable from live ones.
No gap acknowledgment or special system messages on restore.

## Message Persistence

All message types are persisted: user, assistant, system, tool
calls, and tool results. No truncation or filtering.

## Configuration

New flag and env var:

```
--session-db    DOCSCLAW_SESSION_DB
```

| Value | Behavior |
|-------|----------|
| `""` (unset) | SQLite at `$XDG_DATA_HOME/docsclaw/sessions.db` |
| `"memory"` | In-memory store (current behavior) |
| Any path | SQLite at that path |

XDG resolution: `$XDG_DATA_HOME` if set, otherwise
`~/.local/share`. Directory created automatically.

For Kubernetes: set `DOCSCLAW_SESSION_DB` in the pod spec and
mount a PVC at that path.

## Wiring

In `serve.go`, replacing the current `session.NewStore()` block:

```go
var sessions session.SessionStore
if sessionDB == "memory" {
    sessions = session.NewMemoryStore(30 * time.Minute)
} else {
    store, err := session.NewSQLiteStore(sessionDB, 30 * time.Minute)
    if err != nil {
        return fmt.Errorf("failed to open session database: %w", err)
    }
    defer store.Close()
    sessions = store
}
```

The executor's `Sessions` field changes from `*session.Store` to
`session.SessionStore`. Error handling is added to `GetOrCreate`
and `AppendAndSnapshot` calls in `executor.go`.

## Files to Modify or Create

| File | Change |
|------|--------|
| `internal/session/store.go` | Extract `SessionStore` interface, rename `Store` to `MemoryStore`, adapt methods to return errors |
| `internal/session/store_test.go` | Update to use `MemoryStore` constructor |
| `internal/session/sqlite_store.go` | **New**: `SQLiteStore` implementation |
| `internal/session/sqlite_store_test.go` | **New**: SQLite tests + persistence roundtrip |
| `internal/bridge/executor.go` | Change `Sessions` to `session.SessionStore`, add error handling |
| `internal/cmd/serve.go` | Add `--session-db` flag, XDG default, store selection |
| `go.mod` | Add `modernc.org/sqlite` |

## Testing

**SQLiteStore unit tests** (`sqlite_store_test.go`):
- Same cases as memory store: GetOrCreate, Append,
  AppendAndSnapshot, Reaper, concurrent access
- Tool call/result serialization roundtrip
- Persistence across close/reopen

**Memory store tests**: updated for renamed constructor.

**Integration**: manual test with
`docsclaw serve --session-db /tmp/test.db` + `docsclaw chat`,
kill server, restart, verify conversation continues.

## Dependencies

- `modernc.org/sqlite`: pure Go SQLite driver, no CGo,
  cross-compiles for all release targets

## Out of Scope

- Context compaction / summarization
- Message count limits per session
- Redis backend for multi-replica
- Durable cross-session memory (key-value)
