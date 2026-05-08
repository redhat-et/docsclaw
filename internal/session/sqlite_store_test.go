package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestSQLiteStoreNew(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	if store.Len() != 0 {
		t.Fatalf("expected 0 sessions, got %d", store.Len())
	}
}

func TestSQLiteStoreGetOrCreate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

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
	defer func() { _ = store.Close() }()

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
	defer func() { _ = store.Close() }()

	sess, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if sess != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestSQLiteStoreAppend(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

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
	defer func() { _ = store.Close() }()

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
	defer func() { _ = store.Close() }()

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
	defer func() { _ = store.Close() }()

	msgs, err := store.AppendAndSnapshot("nonexistent", llm.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("AppendAndSnapshot on nonexistent should not error: %v", err)
	}
	if msgs != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestSQLiteStoreReaper(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

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

func TestSQLiteStoreToolCallRoundtrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

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

func TestSQLiteStorePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

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
	_ = store.Close()

	store2, err := NewSQLiteStore(dbPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer func() { _ = store2.Close() }()

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
