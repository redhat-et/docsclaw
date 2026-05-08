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

type SQLiteStore struct {
	db  *sql.DB
	ttl time.Duration
	mu  sync.Mutex
}

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
			_ = db.Close()
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
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteStore{db: db, ttl: ttl}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Len() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	return count
}

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
	defer func() { _ = tx.Rollback() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = tx.Rollback() }()

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
		slog.Info("session expired", "count", count)
	}
}

var _ SessionStore = (*SQLiteStore)(nil)
