package conversation

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/yourusername/go-llm-gateway/internal/api"
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

	s := &SQLStore{db: db, ttl: ttl, dialect: newDialect(driver)}
	if ttl > 0 {
		go s.cleanup()
	}
	return s, nil
}

func (s *SQLStore) Get(id string) (*Conversation, bool) {
	row := s.db.QueryRow(s.dialect.getByID, id)

	var conv Conversation
	var msgJSON string
	err := row.Scan(&conv.ID, &conv.Model, &msgJSON, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return nil, false
	}

	if err := json.Unmarshal([]byte(msgJSON), &conv.Messages); err != nil {
		return nil, false
	}

	return &conv, true
}

func (s *SQLStore) Create(id string, model string, messages []api.Message) *Conversation {
	now := time.Now()
	msgJSON, _ := json.Marshal(messages)

	_, _ = s.db.Exec(s.dialect.upsert, id, model, string(msgJSON), now, now)

	return &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *SQLStore) Append(id string, messages ...api.Message) (*Conversation, bool) {
	conv, ok := s.Get(id)
	if !ok {
		return nil, false
	}

	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()

	msgJSON, _ := json.Marshal(conv.Messages)
	_, _ = s.db.Exec(s.dialect.update, string(msgJSON), conv.UpdatedAt, id)

	return conv, true
}

func (s *SQLStore) Delete(id string) {
	_, _ = s.db.Exec(s.dialect.deleteByID, id)
}

func (s *SQLStore) Size() int {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count)
	return count
}

func (s *SQLStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-s.ttl)
		_, _ = s.db.Exec(s.dialect.cleanup, cutoff)
	}
}
