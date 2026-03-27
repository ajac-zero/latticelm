package apikeys

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store provides database operations for API key management.
type Store struct {
	db *sql.DB
}

// NewStore creates a new API key store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// KeyOwner holds the joined API key + user info needed for authentication.
type KeyOwner struct {
	Key        APIKey
	OIDCIss    string
	OIDCSub    string
	UserRole   string
	UserStatus string
}

// Create inserts a new API key record. The caller is responsible for hashing.
func (s *Store) Create(ctx context.Context, k *APIKey) error {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	k.CreatedAt = time.Now()
	if k.Status == "" {
		k.Status = StatusActive
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, key_hash, key_prefix, user_id, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		k.ID, k.Name, k.KeyHash, k.KeyPrefix, k.UserID, string(k.Status), k.ExpiresAt, k.CreatedAt,
	)
	return err
}

// Authenticate looks up a key by its SHA-256 hash and joins with the users
// table so the caller can build a Principal in a single query.
func (s *Store) Authenticate(ctx context.Context, keyHash string) (*KeyOwner, error) {
	var ko KeyOwner
	var status string
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT ak.id, ak.name, ak.key_hash, ak.key_prefix, ak.user_id, ak.status,
		       ak.expires_at, ak.created_at, ak.last_used_at,
		       u.oidc_iss, u.oidc_sub, u.role, u.status
		FROM api_keys ak
		JOIN users u ON ak.user_id = u.id
		WHERE ak.key_hash = $1`, keyHash,
	).Scan(
		&ko.Key.ID, &ko.Key.Name, &ko.Key.KeyHash, &ko.Key.KeyPrefix,
		&ko.Key.UserID, &status, &expiresAt, &ko.Key.CreatedAt, &lastUsedAt,
		&ko.OIDCIss, &ko.OIDCSub, &ko.UserRole, &ko.UserStatus,
	)
	if err != nil {
		return nil, err
	}

	ko.Key.Status = Status(status)
	if expiresAt.Valid {
		ko.Key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		ko.Key.LastUsedAt = &lastUsedAt.Time
	}
	return &ko, nil
}

// ListByUser returns all keys owned by the given user.
func (s *Store) ListByUser(ctx context.Context, userID string) ([]*APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, key_prefix, user_id, status, expires_at, created_at, last_used_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var k APIKey
		var status string
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.UserID,
			&status, &expiresAt, &k.CreatedAt, &lastUsedAt); err != nil {
			return nil, err
		}
		k.Status = Status(status)
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			k.LastUsedAt = &lastUsedAt.Time
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// Revoke sets the key status to revoked.
func (s *Store) Revoke(ctx context.Context, id, ownerID string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET status = $1 WHERE id = $2 AND user_id = $3`,
		string(StatusRevoked), id, ownerID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key not found or not owned by user")
	}
	return nil
}

// Delete removes an API key record.
func (s *Store) Delete(ctx context.Context, id, ownerID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, id, ownerID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key not found or not owned by user")
	}
	return nil
}

// TouchLastUsed updates the last_used_at timestamp. Intended to be called
// asynchronously to avoid adding latency to the authentication path.
func (s *Store) TouchLastUsed(ctx context.Context, id string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`, time.Now(), id)
}
