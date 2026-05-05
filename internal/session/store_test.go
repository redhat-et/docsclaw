package session

import (
	"context"
	"testing"
	"time"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestStoreGetOrCreate(t *testing.T) {
	s := NewStore(30 * time.Minute)
	sess := s.GetOrCreate("task-1", "You are a helpful assistant.")

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

func TestStoreGetOrCreateExisting(t *testing.T) {
	s := NewStore(30 * time.Minute)
	sess1 := s.GetOrCreate("task-1", "prompt-1")
	s.Append("task-1", llm.Message{Role: "user", Content: "hello"})

	sess2 := s.GetOrCreate("task-1", "prompt-2")

	if len(sess2.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess2.Messages))
	}
	if sess2.Messages[0].Content != "prompt-1" {
		t.Fatal("existing session should keep original system prompt")
	}
	if sess1 != sess2 {
		t.Fatal("expected same session pointer")
	}
}

func TestStoreAppend(t *testing.T) {
	s := NewStore(30 * time.Minute)
	s.GetOrCreate("task-1", "system")

	before := s.Get("task-1").LastActive
	time.Sleep(time.Millisecond)

	s.Append("task-1", llm.Message{Role: "user", Content: "hello"})
	s.Append("task-1", llm.Message{Role: "assistant", Content: "hi"})

	sess := s.Get("task-1")
	if len(sess.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(sess.Messages))
	}
	if !sess.LastActive.After(before) {
		t.Fatal("expected LastActive to be updated")
	}
}

func TestStoreAppendNonexistent(t *testing.T) {
	s := NewStore(30 * time.Minute)
	s.Append("nonexistent", llm.Message{Role: "user", Content: "hello"})
	if s.Len() != 0 {
		t.Fatal("append to nonexistent session should not create it")
	}
}

func TestStoreGetNonexistent(t *testing.T) {
	s := NewStore(30 * time.Minute)
	if s.Get("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestStoreReaper(t *testing.T) {
	s := NewStore(10 * time.Millisecond)
	s.GetOrCreate("expire-me", "system")
	s.GetOrCreate("keep-me", "system")

	time.Sleep(20 * time.Millisecond)

	// Touch keep-me so it survives
	s.Append("keep-me", llm.Message{Role: "user", Content: "still here"})

	s.reap()

	if s.Get("expire-me") != nil {
		t.Fatal("expected expire-me to be reaped")
	}
	if s.Get("keep-me") == nil {
		t.Fatal("expected keep-me to survive")
	}
}

func TestStoreStartReaperCancellation(t *testing.T) {
	s := NewStore(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.StartReaper(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StartReaper did not stop after context cancellation")
	}
}
