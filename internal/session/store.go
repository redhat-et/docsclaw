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

// Store manages in-memory sessions with TTL-based expiry.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewStore creates a session store with the given idle TTL.
func NewStore(ttl time.Duration) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// GetOrCreate returns an existing session or creates one with the
// system prompt as the first message.
func (s *Store) GetOrCreate(id, systemPrompt string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		return sess
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
	return sess
}

// Get returns the session or nil if not found.
func (s *Store) Get(id string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

// Append adds a message to the session and updates LastActive.
func (s *Store) Append(id string, msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return
	}
	sess.Messages = append(sess.Messages, msg)
	sess.LastActive = time.Now()
}

// Len returns the number of active sessions.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// StartReaper runs a background goroutine that removes sessions
// idle beyond the TTL. It stops when ctx is cancelled.
func (s *Store) StartReaper(ctx context.Context) {
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

func (s *Store) reap() {
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
