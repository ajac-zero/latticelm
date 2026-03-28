package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
			getByID:    `SELECT id, model, messages, request, owner_iss, owner_sub, tenant_id, created_at, updated_at FROM conversations WHERE id = $1`,
			upsert:     `INSERT INTO conversations (id, model, messages, request, owner_iss, owner_sub, tenant_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET model = EXCLUDED.model, messages = EXCLUDED.messages, request = EXCLUDED.request, updated_at = EXCLUDED.updated_at`,
			update:     `UPDATE conversations SET messages = $1, updated_at = $2 WHERE id = $3`,
			deleteByID: `DELETE FROM conversations WHERE id = $1`,
			cleanup:    `DELETE FROM conversations WHERE updated_at < $1`,
		}
	}
	return sqlDialect{
		getByID:    `SELECT id, model, messages, request, owner_iss, owner_sub, tenant_id, created_at, updated_at FROM conversations WHERE id = ?`,
		upsert:     `REPLACE INTO conversations (id, model, messages, request, owner_iss, owner_sub, tenant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		update:     `UPDATE conversations SET messages = ?, updated_at = ? WHERE id = ?`,
		deleteByID: `DELETE FROM conversations WHERE id = ?`,
		cleanup:    `DELETE FROM conversations WHERE updated_at < ?`,
	}
}

// SQLStore manages conversation history in a SQL database with automatic expiration.
type SQLStore struct {
	db      *sql.DB
	driver  string // "sqlite", "pgx", or "postgres"
	ttl     time.Duration
	dialect sqlDialect
	done    chan struct{}
}

// NewSQLStore creates a SQL-backed conversation store. It runs all pending
// schema migrations, verifies the schema version, and starts a background
// goroutine to remove expired rows.
func NewSQLStore(db *sql.DB, driver string, ttl time.Duration) (*SQLStore, error) {
	ctx := context.Background()

	if _, err := Migrate(ctx, db, driver); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	if err := CheckSchemaVersion(ctx, db); err != nil {
		return nil, err
	}

	s := &SQLStore{
		db:      db,
		driver:  driver,
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
	row := s.db.QueryRowContext(ctx, s.dialect.getByID, id) // #nosec G701

	var conv Conversation
	var msgJSON string
	var requestJSON string
	err := row.Scan(&conv.ID, &conv.Model, &msgJSON, &requestJSON, &conv.OwnerIss, &conv.OwnerSub, &conv.TenantID, &conv.CreatedAt, &conv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(msgJSON), &conv.Messages); err != nil {
		return nil, err
	}
	if requestJSON != "" && requestJSON != "null" {
		var req api.ResponseRequest
		if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
			return nil, err
		}
		conv.Request = &req
	}

	return &conv, nil
}

func (s *SQLStore) Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo, request *api.ResponseRequest) (*Conversation, error) {
	now := time.Now()
	msgJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	if _, err := s.db.ExecContext(ctx, s.dialect.upsert, id, model, string(msgJSON), string(requestJSON), owner.OwnerIss, owner.OwnerSub, owner.TenantID, now, now); err != nil {
		return nil, err
	}

	return &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		Request:   copyRequest(request),
		OwnerIss:  owner.OwnerIss,
		OwnerSub:  owner.OwnerSub,
		TenantID:  owner.TenantID,
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
	_, err := s.db.ExecContext(ctx, s.dialect.deleteByID, id) // #nosec G701
	return err
}

func (s *SQLStore) Size() int {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count)
	return count
}

// List returns a paginated list of conversations with optional filters.
func (s *SQLStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	// Set defaults
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Limit < 1 {
		opts.Limit = 20
	}

	// Build WHERE clause
	var whereClauses []string
	var args []interface{}
	argNum := 1

	if opts.OwnerIss != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("owner_iss = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "owner_iss = ?")
		}
		args = append(args, opts.OwnerIss)
		argNum++
	}

	if opts.OwnerSub != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("owner_sub = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "owner_sub = ?")
		}
		args = append(args, opts.OwnerSub)
		argNum++
	}

	if opts.TenantID != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "tenant_id = ?")
		}
		args = append(args, opts.TenantID)
		argNum++
	}

	if opts.Model != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("model = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "model = ?")
		}
		args = append(args, opts.Model)
		argNum++
	}

	if opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("id ILIKE $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "id LIKE ?")
		}
		args = append(args, searchPattern)
		argNum++
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereClause += " AND " + whereClauses[i]
		}
	}

	// Count total matching records
	countQuery := "SELECT COUNT(*) FROM conversations " + whereClause
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	// Fetch paginated results (without full messages, just metadata)
	offset := (opts.Page - 1) * opts.Limit

	// SQLite uses json_array_length, PostgreSQL uses jsonb_array_length
	var messageCountExpr string
	if s.driver == "pgx" || s.driver == "postgres" {
		messageCountExpr = "jsonb_array_length(messages::jsonb)"
	} else {
		messageCountExpr = "json_array_length(messages)"
	}

	// #nosec G201
	dataQuery := fmt.Sprintf(`
		SELECT id, model, owner_iss, owner_sub, tenant_id, created_at, updated_at, %s as message_count
		FROM conversations
		%s
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`, messageCountExpr, whereClause)

	if s.driver == "pgx" || s.driver == "postgres" {
		dataQuery = fmt.Sprintf(`
			SELECT id, model, owner_iss, owner_sub, tenant_id, created_at, updated_at, %s as message_count
			FROM conversations
			%s
			ORDER BY updated_at DESC
			LIMIT $%d OFFSET $%d
		`, messageCountExpr, whereClause, argNum, argNum+1)
	}

	listArgs := append(args, opts.Limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*Conversation
	for rows.Next() {
		var conv Conversation
		var messageCount int
		err := rows.Scan(
			&conv.ID, &conv.Model, &conv.OwnerIss, &conv.OwnerSub, &conv.TenantID,
			&conv.CreatedAt, &conv.UpdatedAt, &messageCount,
		)
		if err != nil {
			return nil, err
		}
		// Store message count as a slice of that length (API will use len())
		conv.Messages = make([]api.Message, messageCount)
		conversations = append(conversations, &conv)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ListResult{
		Conversations: conversations,
		Total:         total,
	}, nil
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
