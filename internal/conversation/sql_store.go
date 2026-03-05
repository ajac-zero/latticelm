package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
)

// sqlDialect holds driver-specific SQL statements.
type sqlDialect struct {
	getByID    string
	upsert     string
	update     string
	deleteByID string
	cleanup    string
}

func newDialect(driver string) sqlDialect {
	if driver == "pgx" || driver == "postgres" {
		return sqlDialect{
			getByID:    `SELECT id, model, messages, created_at, updated_at FROM conversations WHERE id = $1`,
			upsert:     `INSERT INTO conversations (id, model, messages, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET model = EXCLUDED.model, messages = EXCLUDED.messages, updated_at = EXCLUDED.updated_at`,
			update:     `UPDATE conversations SET messages = $1, updated_at = $2 WHERE id = $3`,
			deleteByID: `DELETE FROM conversations WHERE id = $1`,
			cleanup:    `DELETE FROM conversations WHERE updated_at < $1`,
		}
	}
	return sqlDialect{
		getByID:    `SELECT id, model, messages, created_at, updated_at FROM conversations WHERE id = ?`,
		upsert:     `REPLACE INTO conversations (id, model, messages, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		update:     `UPDATE conversations SET messages = ?, updated_at = ? WHERE id = ?`,
		deleteByID: `DELETE FROM conversations WHERE id = ?`,
		cleanup:    `DELETE FROM conversations WHERE updated_at < ?`,
	}
}

// SQLStore manages conversation history in a SQL database with automatic expiration.
type SQLStore struct {
	db      *sql.DB
	ttl     time.Duration
	dialect sqlDialect
	done    chan struct{}
}

// NewSQLStore creates a SQL-backed conversation store. It creates the
// conversations table if it does not already exist and starts a background
// goroutine to remove expired rows.
func NewSQLStore(db *sql.DB, driver string, ttl time.Duration) (*SQLStore, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS conversations (
		id         TEXT PRIMARY KEY,
		model      TEXT NOT NULL,
		messages   TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return nil, err
	}

	s := &SQLStore{
		db:      db,
		ttl:     ttl,
		dialect: newDialect(driver),
		done:    make(chan struct{}),
	}
	if ttl > 0 {
		go s.cleanup()
	}
	return s, nil
}

func (s *SQLStore) Get(ctx context.Context, id string) (*Conversation, error) {
	row := s.db.QueryRowContext(ctx, s.dialect.getByID, id)

	var conv Conversation
	var msgJSON string
	err := row.Scan(&conv.ID, &conv.Model, &msgJSON, &conv.CreatedAt, &conv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(msgJSON), &conv.Messages); err != nil {
		return nil, err
	}

	return &conv, nil
}

func (s *SQLStore) Create(ctx context.Context, id string, model string, messages []api.Message) (*Conversation, error) {
	now := time.Now()
	msgJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	if _, err := s.db.ExecContext(ctx, s.dialect.upsert, id, model, string(msgJSON), now, now); err != nil {
		return nil, err
	}

	return &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *SQLStore) Append(ctx context.Context, id string, messages ...api.Message) (*Conversation, error) {
	conv, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, nil
	}

	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()

	msgJSON, err := json.Marshal(conv.Messages)
	if err != nil {
		return nil, err
	}

	if _, err := s.db.ExecContext(ctx, s.dialect.update, string(msgJSON), conv.UpdatedAt, id); err != nil {
		return nil, err
	}

	return conv, nil
}

func (s *SQLStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.dialect.deleteByID, id)
	return err
}

func (s *SQLStore) Size() int {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count)
	return count
}

func (s *SQLStore) cleanup() {
	// Calculate cleanup interval as 10% of TTL, with sensible bounds
	interval := s.ttl / 10

	// Cap maximum interval at 1 minute for production
	if interval > 1*time.Minute {
		interval = 1 * time.Minute
	}

	// Allow small intervals for testing (as low as 10ms)
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-s.ttl)
			_, _ = s.db.Exec(s.dialect.cleanup, cutoff)
		case <-s.done:
			return
		}
	}
}

// Close stops the cleanup goroutine and closes the database connection.
func (s *SQLStore) Close() error {
	close(s.done)
	return s.db.Close()
}
