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
