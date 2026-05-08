# SQLite Session Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SQLite-backed persistent session storage so conversations survive server restarts.

**Architecture:** Extract a `SessionStore` interface from the concrete in-memory store. The existing store becomes `MemoryStore`; a new `SQLiteStore` implements the same interface using `modernc.org/sqlite`. Selection is controlled by a `--session-db` flag.

**Tech Stack:** Go 1.25+, `modernc.org/sqlite` (pure Go), `database/sql`

**Spec:** `docs/superpowers/specs/2026-05-08-sqlite-session-persistence-design.md`

---

### Task 1: Extract SessionStore Interface and Rename to MemoryStore

**Files:**
- Modify: `internal/session/store.go`
- Modify: `internal/session/store_test.go`

- [ ] **Step 1: Write a compilation test for the interface**

Add to the bottom of `internal/session/store.go`:

```go
// Compile-time interface check.
var _ SessionStore = (*MemoryStore)(nil)
```

This will fail to compile until the interface and rename are done.

- [ ] **Step 2: Add the SessionStore interface and rename Store to MemoryStore**

Replace the contents of `internal/session/store.go` with:

```go
package session

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

// Session holds the conversation state for a single A2A task.
type Session struct {
	ID         string
	Messages   []llm.Message
	CreatedAt  time.Time
	LastActive time.Time
}

// SessionStore is the interface for session persistence backends.
type SessionStore interface {
	GetOrCreate(id, systemPrompt string) (*Session, error)
	Get(id string) (*Session, error)
	Append(id string, msg llm.Message) error
	AppendAndSnapshot(id string, msg llm.Message) ([]llm.Message, error)
	Len() int
	StartReaper(ctx context.Context)
	Close() error
}

// MemoryStore manages in-memory sessions with TTL-based expiry.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewMemoryStore creates an in-memory session store with the given idle TTL.
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

func (s *MemoryStore) GetOrCreate(id, systemPrompt string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}

	now := time.Now()
	sess := &Session{
		ID: id,
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
		},
		CreatedAt:  now,
		LastActive: now,
	}
	s.sessions[id] = sess
	slog.Info("session created", "session_id", id)
	return sess, nil
}

func (s *MemoryStore) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id], nil
}

func (s *MemoryStore) Append(id string, msg llm.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil
	}
	sess.Messages = append(sess.Messages, msg)
	sess.LastActive = time.Now()
	return nil
}

func (s *MemoryStore) AppendAndSnapshot(id string, msg llm.Message) ([]llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, nil
	}
	sess.Messages = append(sess.Messages, msg)
	sess.LastActive = time.Now()

	snapshot := make([]llm.Message, len(sess.Messages))
	copy(snapshot, sess.Messages)
	return snapshot, nil
}

func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *MemoryStore) StartReaper(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reap()
		}
	}
}

func (s *MemoryStore) Close() error {
	return nil
}

func (s *MemoryStore) reap() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, sess := range s.sessions {
		if now.Sub(sess.LastActive) > s.ttl {
			delete(s.sessions, id)
			slog.Info("session expired",
				"session_id", id,
				"message_count", len(sess.Messages))
		}
	}
}

// Compile-time interface check.
var _ SessionStore = (*MemoryStore)(nil)
```

- [ ] **Step 3: Update tests for renamed constructor**

In `internal/session/store_test.go`, replace all calls to `NewStore(` with `NewMemoryStore(`. There are 9 occurrences.

Also update the `reap()` call in `TestStoreReaper` — since `reap` is unexported, call it via a helper or make the test use `StartReaper` with a short ticker. Since `reap()` is on `*MemoryStore` (not the interface), the direct call still works.

- [ ] **Step 4: Verify tests pass**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -v`

Expected: all 9 tests pass.

- [ ] **Step 5: Verify the full build compiles**

Run: `cd /Users/panni/work/docsclaw && go build ./...`

Expected: compilation errors in `internal/bridge/executor.go` and `internal/cmd/serve.go` because they reference the old `Store` type and old method signatures. This is expected — we fix those in Task 3.

- [ ] **Step 6: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/store.go internal/session/store_test.go
git commit -s -m "refactor: extract SessionStore interface, rename Store to MemoryStore"
```

---

### Task 2: Add SQLite Dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the modernc.org/sqlite dependency**

Run:

```bash
cd /Users/panni/work/docsclaw && go get modernc.org/sqlite
```

- [ ] **Step 2: Tidy**

Run:

```bash
cd /Users/panni/work/docsclaw && go mod tidy
```

- [ ] **Step 3: Commit**

```bash
cd /Users/panni/work/docsclaw && git add go.mod go.sum
git commit -s -m "deps: add modernc.org/sqlite for session persistence"
```

---

### Task 3: Update Executor and Serve Command for Interface

**Files:**
- Modify: `internal/bridge/executor.go`
- Modify: `internal/cmd/serve.go`

- [ ] **Step 1: Update executor to use SessionStore interface**

In `internal/bridge/executor.go`, change the `Sessions` field and add error handling.

Replace:

```go
	Sessions       *session.Store     // optional: server-side conversation state
```

With:

```go
	Sessions       session.SessionStore // optional: server-side conversation state
```

Replace the session usage block (lines 102-108) from:

```go
			if e.Sessions != nil && sessionID != "" {
				e.Sessions.GetOrCreate(sessionID, e.SystemPrompt)
				messages = e.Sessions.AppendAndSnapshot(sessionID,
					llm.Message{Role: "user", Content: userText})
				e.Log.Info("Processing free-form message via agentic loop",
					"session_id", sessionID,
					"message_count", len(messages))
```

To:

```go
			if e.Sessions != nil && sessionID != "" {
				if _, err := e.Sessions.GetOrCreate(sessionID, e.SystemPrompt); err != nil {
					e.Log.Error("Session creation failed", "error", err)
					yield(e.failedEvent(execCtx, "Session error: "+err.Error()), nil)
					return
				}
				var err error
				messages, err = e.Sessions.AppendAndSnapshot(sessionID,
					llm.Message{Role: "user", Content: userText})
				if err != nil {
					e.Log.Error("Session append failed", "error", err)
					yield(e.failedEvent(execCtx, "Session error: "+err.Error()), nil)
					return
				}
				e.Log.Info("Processing free-form message via agentic loop",
					"session_id", sessionID,
					"message_count", len(messages))
```

Replace the post-processing session append (lines 125-127) from:

```go
			if e.Sessions != nil && sessionID != "" {
				e.Sessions.Append(sessionID,
					llm.Message{Role: "assistant", Content: result})
			}
```

To:

```go
			if e.Sessions != nil && sessionID != "" {
				if err := e.Sessions.Append(sessionID,
					llm.Message{Role: "assistant", Content: result}); err != nil {
					e.Log.Error("Failed to save assistant response to session",
						"session_id", sessionID, "error", err)
				}
			}
```

Also remove the now-unused direct import of `"github.com/redhat-et/docsclaw/internal/session"` — replace it with the interface-based reference. Actually, the import is still needed since `session.SessionStore` is the type. Keep the import.

- [ ] **Step 2: Update serve.go to use NewMemoryStore**

In `internal/cmd/serve.go`, change line 435 from:

```go
		sessions := session.NewStore(30 * time.Minute)
```

To:

```go
		var sessions session.SessionStore = session.NewMemoryStore(30 * time.Minute)
```

- [ ] **Step 3: Verify full build compiles**

Run: `cd /Users/panni/work/docsclaw && go build ./...`

Expected: compiles cleanly with no errors.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/panni/work/docsclaw && go test ./... 2>&1 | tail -20`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/bridge/executor.go internal/cmd/serve.go
git commit -s -m "refactor: update executor and serve to use SessionStore interface"
```

---

### Task 4: Implement SQLiteStore — Constructor and Schema

**Files:**
- Create: `internal/session/sqlite_store.go`

- [ ] **Step 1: Write the failing test for SQLiteStore construction**

Create `internal/session/sqlite_store_test.go`:

```go
package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreNew(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if store.Len() != 0 {
		t.Fatalf("expected 0 sessions, got %d", store.Len())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStoreNew -v`

Expected: FAIL — `NewSQLiteStore` not defined.

- [ ] **Step 3: Implement SQLiteStore constructor and schema**

Create `internal/session/sqlite_store.go`:

```go
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
	_ "modernc.org/sqlite"
)

// SQLiteStore persists sessions to a SQLite database.
type SQLiteStore struct {
	db  *sql.DB
	ttl time.Duration
	mu  sync.Mutex
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath
// and initializes the schema.
func NewSQLiteStore(dbPath string, ttl time.Duration) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	schema := `
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
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteStore{db: db, ttl: ttl}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Len returns the number of active sessions.
func (s *SQLiteStore) Len() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	return count
}

// Placeholder methods — implemented in subsequent tasks.

func (s *SQLiteStore) GetOrCreate(id, systemPrompt string) (*Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *SQLiteStore) Get(id string) (*Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *SQLiteStore) Append(id string, msg llm.Message) error {
	return fmt.Errorf("not implemented")
}

func (s *SQLiteStore) AppendAndSnapshot(id string, msg llm.Message) ([]llm.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *SQLiteStore) StartReaper(ctx context.Context) {}

// Compile-time interface check.
var _ SessionStore = (*SQLiteStore)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStoreNew -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/sqlite_store.go internal/session/sqlite_store_test.go
git commit -s -m "feat: add SQLiteStore skeleton with schema initialization"
```

---

### Task 5: Implement GetOrCreate and Get

**Files:**
- Modify: `internal/session/sqlite_store.go`
- Modify: `internal/session/sqlite_store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/session/sqlite_store_test.go`:

```go
func TestSQLiteStoreGetOrCreate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	sess, err := store.GetOrCreate("task-1", "You are a helpful assistant.")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	if sess.ID != "task-1" {
		t.Fatalf("expected ID task-1, got %q", sess.ID)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 message (system), got %d", len(sess.Messages))
	}
	if sess.Messages[0].Role != "system" {
		t.Fatalf("expected system role, got %q", sess.Messages[0].Role)
	}
	if sess.Messages[0].Content != "You are a helpful assistant." {
		t.Fatalf("unexpected system prompt: %q", sess.Messages[0].Content)
	}
}

func TestSQLiteStoreGetOrCreateExisting(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrCreate("task-1", "prompt-1")
	if err != nil {
		t.Fatalf("first GetOrCreate failed: %v", err)
	}

	sess, err := store.GetOrCreate("task-1", "prompt-2")
	if err != nil {
		t.Fatalf("second GetOrCreate failed: %v", err)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "prompt-1" {
		t.Fatal("existing session should keep original system prompt")
	}
}

func TestSQLiteStoreGetNonexistent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	sess, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sess != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStore -v`

Expected: FAIL — `GetOrCreate` returns "not implemented".

- [ ] **Step 3: Implement GetOrCreate and Get**

In `internal/session/sqlite_store.go`, replace the placeholder `GetOrCreate` and `Get` methods:

```go
func (s *SQLiteStore) GetOrCreate(id, systemPrompt string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.loadSession(id)
	if err != nil {
		return nil, err
	}
	if sess != nil {
		return sess, nil
	}

	now := time.Now()
	nowStr := now.Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO sessions (id, system_prompt, created_at, last_active) VALUES (?, ?, ?, ?)",
		id, systemPrompt, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	_, err = tx.Exec(
		"INSERT INTO messages (session_id, seq, role, content) VALUES (?, 0, 'system', ?)",
		id, systemPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert system message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	slog.Info("session created", "session_id", id)
	return &Session{
		ID:         id,
		Messages:   []llm.Message{{Role: "system", Content: systemPrompt}},
		CreatedAt:  now,
		LastActive: now,
	}, nil
}

func (s *SQLiteStore) Get(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSession(id)
}

func (s *SQLiteStore) loadSession(id string) (*Session, error) {
	var systemPrompt, createdStr, activeStr string
	err := s.db.QueryRow(
		"SELECT system_prompt, created_at, last_active FROM sessions WHERE id = ?", id,
	).Scan(&systemPrompt, &createdStr, &activeStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, createdStr)
	lastActive, _ := time.Parse(time.RFC3339Nano, activeStr)

	rows, err := s.db.Query(
		"SELECT role, content, tool_calls, tool_results FROM messages WHERE session_id = ? ORDER BY seq",
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []llm.Message
	for rows.Next() {
		var role string
		var content, toolCallsJSON, toolResultsJSON sql.NullString

		if err := rows.Scan(&role, &content, &toolCallsJSON, &toolResultsJSON); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		msg := llm.Message{
			Role:    role,
			Content: content.String,
		}

		if toolCallsJSON.Valid && toolCallsJSON.String != "" {
			if err := json.Unmarshal([]byte(toolCallsJSON.String), &msg.ToolCalls); err != nil {
				return nil, fmt.Errorf("unmarshal tool_calls: %w", err)
			}
		}
		if toolResultsJSON.Valid && toolResultsJSON.String != "" {
			if err := json.Unmarshal([]byte(toolResultsJSON.String), &msg.ToolResults); err != nil {
				return nil, fmt.Errorf("unmarshal tool_results: %w", err)
			}
		}

		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return &Session{
		ID:         id,
		Messages:   messages,
		CreatedAt:  createdAt,
		LastActive: lastActive,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStore -v`

Expected: PASS for all three new tests.

- [ ] **Step 5: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/sqlite_store.go internal/session/sqlite_store_test.go
git commit -s -m "feat: implement SQLiteStore GetOrCreate and Get"
```

---

### Task 6: Implement Append and AppendAndSnapshot

**Files:**
- Modify: `internal/session/sqlite_store.go`
- Modify: `internal/session/sqlite_store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/session/sqlite_store_test.go`:

```go
import (
	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestSQLiteStoreAppend(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrCreate("task-1", "system")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	err = store.Append("task-1", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	err = store.Append("task-1", llm.Message{Role: "assistant", Content: "hi"})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	sess, err := store.Get("task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(sess.Messages))
	}
}

func TestSQLiteStoreAppendNonexistent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	err = store.Append("nonexistent", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("Append to nonexistent should not error: %v", err)
	}
	if store.Len() != 0 {
		t.Fatal("append to nonexistent session should not create it")
	}
}

func TestSQLiteStoreAppendAndSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrCreate("task-1", "system")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	msgs, err := store.AppendAndSnapshot("task-1", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("AppendAndSnapshot failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestSQLiteStoreAppendAndSnapshotNonexistent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	msgs, err := store.AppendAndSnapshot("nonexistent", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("AppendAndSnapshot on nonexistent should not error: %v", err)
	}
	if msgs != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run "TestSQLiteStoreAppend" -v`

Expected: FAIL — `Append` returns "not implemented".

- [ ] **Step 3: Implement Append and AppendAndSnapshot**

In `internal/session/sqlite_store.go`, replace the placeholder methods:

```go
func (s *SQLiteStore) Append(id string, msg llm.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendMessage(id, msg)
}

func (s *SQLiteStore) AppendAndSnapshot(id string, msg llm.Message) ([]llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendMessage(id, msg); err != nil {
		return nil, err
	}

	sess, err := s.loadSession(id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}
	return sess.Messages, nil
}

func (s *SQLiteStore) appendMessage(id string, msg llm.Message) error {
	var exists bool
	err := s.db.QueryRow("SELECT 1 FROM sessions WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}

	var seq int
	err = s.db.QueryRow(
		"SELECT COALESCE(MAX(seq), -1) + 1 FROM messages WHERE session_id = ?", id,
	).Scan(&seq)
	if err != nil {
		return fmt.Errorf("get next seq: %w", err)
	}

	var toolCallsJSON, toolResultsJSON *string
	if len(msg.ToolCalls) > 0 {
		b, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool_calls: %w", err)
		}
		s := string(b)
		toolCallsJSON = &s
	}
	if len(msg.ToolResults) > 0 {
		b, err := json.Marshal(msg.ToolResults)
		if err != nil {
			return fmt.Errorf("marshal tool_results: %w", err)
		}
		s := string(b)
		toolResultsJSON = &s
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO messages (session_id, seq, role, content, tool_calls, tool_results) VALUES (?, ?, ?, ?, ?, ?)",
		id, seq, msg.Role, msg.Content, toolCallsJSON, toolResultsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	_, err = tx.Exec(
		"UPDATE sessions SET last_active = ? WHERE id = ?",
		time.Now().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("update last_active: %w", err)
	}

	return tx.Commit()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run "TestSQLiteStoreAppend" -v`

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/sqlite_store.go internal/session/sqlite_store_test.go
git commit -s -m "feat: implement SQLiteStore Append and AppendAndSnapshot"
```

---

### Task 7: Implement Reaper

**Files:**
- Modify: `internal/session/sqlite_store.go`
- Modify: `internal/session/sqlite_store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/session/sqlite_store_test.go`:

```go
func TestSQLiteStoreReaper(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrCreate("expire-me", "system")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	_, err = store.GetOrCreate("keep-me", "system")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	err = store.Append("keep-me", llm.Message{Role: "user", Content: "still here"})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	store.reap()

	sess, err := store.Get("expire-me")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sess != nil {
		t.Fatal("expected expire-me to be reaped")
	}

	sess, err = store.Get("keep-me")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sess == nil {
		t.Fatal("expected keep-me to survive")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStoreReaper -v`

Expected: FAIL — `reap` method not defined on SQLiteStore.

- [ ] **Step 3: Implement reap and StartReaper**

In `internal/session/sqlite_store.go`, replace the placeholder `StartReaper` and add `reap`:

```go
func (s *SQLiteStore) StartReaper(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reap()
		}
	}
}

func (s *SQLiteStore) reap() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.ttl).Format(time.RFC3339Nano)
	result, err := s.db.Exec("DELETE FROM sessions WHERE last_active < ?", cutoff)
	if err != nil {
		slog.Error("session reap failed", "error", err)
		return
	}
	if count, _ := result.RowsAffected(); count > 0 {
		slog.Info("sessions reaped", "count", count)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run TestSQLiteStoreReaper -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/sqlite_store.go internal/session/sqlite_store_test.go
git commit -s -m "feat: implement SQLiteStore reaper for TTL-based session expiry"
```

---

### Task 8: Tool Call Serialization Roundtrip and Persistence Test

**Files:**
- Modify: `internal/session/sqlite_store_test.go`

- [ ] **Step 1: Write the tool call roundtrip test**

Add to `internal/session/sqlite_store_test.go`:

```go
func TestSQLiteStoreToolCallRoundtrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrCreate("task-1", "system")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	err = store.Append("task-1", llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{
			{ID: "call-1", Name: "exec", Args: map[string]any{"command": "ls"}},
		},
	})
	if err != nil {
		t.Fatalf("Append tool call failed: %v", err)
	}

	err = store.Append("task-1", llm.Message{
		Role: "tool",
		ToolResults: []llm.ToolResultContent{
			{ToolUseID: "call-1", Output: "file1.txt\nfile2.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Append tool result failed: %v", err)
	}

	sess, err := store.Get("task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(sess.Messages))
	}

	assistantMsg := sess.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].Name != "exec" {
		t.Fatalf("expected tool name 'exec', got %q", assistantMsg.ToolCalls[0].Name)
	}

	toolMsg := sess.Messages[2]
	if len(toolMsg.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolMsg.ToolResults))
	}
	if toolMsg.ToolResults[0].Output != "file1.txt\nfile2.txt" {
		t.Fatalf("unexpected tool output: %q", toolMsg.ToolResults[0].Output)
	}
}
```

- [ ] **Step 2: Write the persistence across close/reopen test**

Add to `internal/session/sqlite_store_test.go`:

```go
func TestSQLiteStorePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// First open: create session and add messages.
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	_, err = store.GetOrCreate("task-1", "You are helpful.")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	err = store.Append("task-1", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	err = store.Append("task-1", llm.Message{Role: "assistant", Content: "hi there"})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	store.Close()

	// Second open: verify everything is still there.
	store2, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer store2.Close()

	sess, err := store2.Get("task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session to survive close/reopen")
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "You are helpful." {
		t.Fatalf("unexpected system prompt: %q", sess.Messages[0].Content)
	}
	if sess.Messages[1].Content != "hello" {
		t.Fatalf("unexpected user message: %q", sess.Messages[1].Content)
	}
	if sess.Messages[2].Content != "hi there" {
		t.Fatalf("unexpected assistant message: %q", sess.Messages[2].Content)
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/panni/work/docsclaw && go test ./internal/session/ -run "TestSQLiteStore(ToolCall|Persistence)" -v`

Expected: PASS — both tests pass since GetOrCreate, Append, and Get are already implemented.

- [ ] **Step 4: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/session/sqlite_store_test.go
git commit -s -m "test: add tool call roundtrip and persistence tests for SQLiteStore"
```

---

### Task 9: Wire SQLite Store into Serve Command

**Files:**
- Modify: `internal/cmd/serve.go`

- [ ] **Step 1: Add the --session-db flag**

In the `init()` function, add the new flag after the existing `llm-timeout` flag:

```go
	serveCmd.Flags().String("session-db", "",
		"Session database path (default: $XDG_DATA_HOME/docsclaw/sessions.db, use 'memory' for in-memory)")
```

And bind it:

```go
	_ = v.BindPFlag("session_db", serveCmd.Flags().Lookup("session-db"))
```

Add `SessionDB` to the `Config` struct:

```go
type Config struct {
	config.CommonConfig `mapstructure:",squash"`
	ConfigDir          string     `mapstructure:"config_dir"`
	SkillsDir          string     `mapstructure:"skills_dir"`
	DocumentServiceURL string     `mapstructure:"document_service_url"`
	LLM                llm.Config `mapstructure:"llm"`
	SessionDB          string     `mapstructure:"session_db"`
}
```

- [ ] **Step 2: Add XDG resolution helper**

Add this function near the top of `serve.go` (after the imports):

```go
func defaultSessionDBPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			dir = filepath.Join(os.TempDir(), "docsclaw")
		} else {
			dir = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(dir, "docsclaw", "sessions.db")
}
```

- [ ] **Step 3: Replace session store creation in runServe**

Replace the existing session creation block (around line 434-438):

```go
		sessions := session.NewStore(30 * time.Minute)
		reaperCtx, reaperCancel := context.WithCancel(context.Background())
		defer reaperCancel()
		go sessions.StartReaper(reaperCtx)
```

With:

```go
		sessionDB := cfg.SessionDB
		if sessionDB == "" {
			sessionDB = os.Getenv("DOCSCLAW_SESSION_DB")
		}

		var sessions session.SessionStore
		if sessionDB == "memory" {
			sessions = session.NewMemoryStore(30 * time.Minute)
			log.Info("Session store", "backend", "memory")
		} else {
			if sessionDB == "" {
				sessionDB = defaultSessionDBPath()
			}
			if err := os.MkdirAll(filepath.Dir(sessionDB), 0755); err != nil {
				return fmt.Errorf("failed to create session db directory: %w", err)
			}
			sqliteStore, err := session.NewSQLiteStore(sessionDB, 30*time.Minute)
			if err != nil {
				return fmt.Errorf("failed to open session database: %w", err)
			}
			defer sqliteStore.Close()
			sessions = sqliteStore
			log.Info("Session store", "backend", "sqlite", "path", sessionDB)
		}

		reaperCtx, reaperCancel := context.WithCancel(context.Background())
		defer reaperCancel()
		go sessions.StartReaper(reaperCtx)
```

Also update the line that was already changed in Task 3 — if it still reads `var sessions session.SessionStore = session.NewMemoryStore(...)`, replace the entire block with the code above.

- [ ] **Step 4: Verify full build compiles**

Run: `cd /Users/panni/work/docsclaw && go build ./...`

Expected: compiles cleanly.

- [ ] **Step 5: Run all tests**

Run: `cd /Users/panni/work/docsclaw && go test ./... 2>&1 | tail -20`

Expected: all tests pass.

- [ ] **Step 6: Run linter**

Run: `cd /Users/panni/work/docsclaw && make lint`

Expected: no new lint errors.

- [ ] **Step 7: Commit**

```bash
cd /Users/panni/work/docsclaw && git add internal/cmd/serve.go
git commit -s -m "feat: wire SQLite session store with --session-db flag and XDG default"
```

---

### Task 10: Update CLAUDE.md and Design Spec Status

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/superpowers/specs/2026-05-08-sqlite-session-persistence-design.md`

- [ ] **Step 1: Update project structure table in CLAUDE.md**

The `internal/session/` entry currently says "In-memory session store for multi-turn conversations". Update it to:

```
| `internal/session/` | Session store (in-memory and SQLite backends) |
```

- [ ] **Step 2: Update design spec status**

In `docs/superpowers/specs/2026-05-08-sqlite-session-persistence-design.md`, change:

```
**Status**: Draft
```

To:

```
**Status**: Implemented
```

- [ ] **Step 3: Commit**

```bash
cd /Users/panni/work/docsclaw && git add CLAUDE.md docs/superpowers/specs/2026-05-08-sqlite-session-persistence-design.md
git commit -s -m "docs: update CLAUDE.md and mark session persistence spec as implemented"
```
