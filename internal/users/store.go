package users

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store provides database operations for user management.
type Store struct {
	db     *sql.DB
	driver string
}

// NewStore creates a new user store with the given database connection.
func NewStore(db *sql.DB, driver string) *Store {
	return &Store{
		db:     db,
		driver: driver,
	}
}

// GetByOIDC retrieves a user by their OIDC identity (issuer + subject).
// Returns sql.ErrNoRows if the user does not exist.
func (s *Store) GetByOIDC(ctx context.Context, issuer, subject string) (*User, error) {
	var u User
	var role, status string

	query := `
		SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
		FROM users
		WHERE oidc_iss = ? AND oidc_sub = ?
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			WHERE oidc_iss = $1 AND oidc_sub = $2
		`
	}

	err := s.db.QueryRowContext(ctx, query, issuer, subject).Scan(
		&u.ID, &u.OIDCIss, &u.OIDCSub, &u.Email, &u.Name,
		&role, &status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.Role = Role(role)
	u.Status = Status(status)
	return &u, nil
}

// GetByID retrieves a user by their application ID.
// Returns sql.ErrNoRows if the user does not exist.
func (s *Store) GetByID(ctx context.Context, id string) (*User, error) {
	var u User
	var role, status string

	query := `
		SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
		FROM users
		WHERE id = ?
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			WHERE id = $1
		`
	}

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.OIDCIss, &u.OIDCSub, &u.Email, &u.Name,
		&role, &status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.Role = Role(role)
	u.Status = Status(status)
	return &u, nil
}

// Create creates a new user record. The user ID is auto-generated.
func (s *Store) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	// Set defaults
	if u.Role == "" {
		u.Role = RoleUser
	}
	if u.Status == "" {
		u.Status = StatusActive
	}

	query := `
		INSERT INTO users (id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			INSERT INTO users (id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`
	}

	_, err := s.db.ExecContext(ctx, query,
		u.ID, u.OIDCIss, u.OIDCSub, u.Email, u.Name,
		string(u.Role), string(u.Status), u.CreatedAt, u.UpdatedAt,
	)
	return err
}

// Update updates an existing user record.
func (s *Store) Update(ctx context.Context, u *User) error {
	u.UpdatedAt = time.Now()

	query := `
		UPDATE users
		SET email = ?, name = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			UPDATE users
			SET email = $1, name = $2, role = $3, status = $4, updated_at = $5
			WHERE id = $6
		`
	}

	result, err := s.db.ExecContext(ctx, query,
		u.Email, u.Name, string(u.Role), string(u.Status), u.UpdatedAt, u.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", u.ID)
	}

	return nil
}

// UpdateRole updates only the user's role.
func (s *Store) UpdateRole(ctx context.Context, userID string, role Role) error {
	query := `
		UPDATE users
		SET role = ?, updated_at = ?
		WHERE id = ?
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			UPDATE users
			SET role = $1, updated_at = $2
			WHERE id = $3
		`
	}

	result, err := s.db.ExecContext(ctx, query, string(role), time.Now(), userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", userID)
	}

	return nil
}

// ListOptions contains filters and pagination for listing users.
type ListOptions struct {
	Page    int
	Limit   int
	Role    Role
	Status  Status
	Search  string
	SortBy  string
	SortDir string // "asc" or "desc"
}

// validSortColumns is the allowlist for sort_by query parameter.
var validSortColumns = map[string]string{
	"name":       "name",
	"email":      "email",
	"role":       "role",
	"status":     "status",
	"created_at": "created_at",
}

// ListResult contains paginated user list results.
type ListResult struct {
	Users []*User
	Total int
}

// List retrieves all users, optionally filtered by status.
func (s *Store) List(ctx context.Context, status Status) ([]*User, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			WHERE status = ?
			ORDER BY created_at DESC
		`
		if s.driver == "pgx" || s.driver == "postgres" {
			query = `
				SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
				FROM users
				WHERE status = $1
				ORDER BY created_at DESC
			`
		}
		args = append(args, string(status))
	} else {
		query = `
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			ORDER BY created_at DESC
		`
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var role, status string

		err := rows.Scan(
			&u.ID, &u.OIDCIss, &u.OIDCSub, &u.Email, &u.Name,
			&role, &status, &u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		u.Role = Role(role)
		u.Status = Status(status)
		users = append(users, &u)
	}

	return users, rows.Err()
}

// ListWithOptions retrieves users with pagination and filtering support.
func (s *Store) ListWithOptions(ctx context.Context, opts ListOptions) (*ListResult, error) {
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

	if opts.Role != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("role = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "role = ?")
		}
		args = append(args, string(opts.Role))
		argNum++
	}

	if opts.Status != "" {
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argNum))
		} else {
			whereClauses = append(whereClauses, "status = ?")
		}
		args = append(args, string(opts.Status))
		argNum++
	}

	if opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		if s.driver == "pgx" || s.driver == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("(email ILIKE $%d OR name ILIKE $%d)", argNum, argNum))
		} else {
			whereClauses = append(whereClauses, "(email LIKE ? OR name LIKE ?)")
			args = append(args, searchPattern)
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
	countQuery := "SELECT COUNT(*) FROM users " + whereClause
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	// Build ORDER BY clause
	sortCol := "created_at"
	if col, ok := validSortColumns[opts.SortBy]; ok {
		sortCol = col
	}
	sortDir := "DESC"
	if opts.SortDir == "asc" {
		sortDir = "ASC"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir)

	// Fetch paginated results
	offset := (opts.Page - 1) * opts.Limit
	listArgs := append(args, opts.Limit, offset)

	// #nosec G201 - sortCol is validated against allowlist, sortDir only accepts asc/desc
	dataQuery := fmt.Sprintf(`
		SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
		FROM users
		%s
		%s
		LIMIT ? OFFSET ?
	`, whereClause, orderClause)

	if s.driver == "pgx" || s.driver == "postgres" {
		// #nosec G201 - sortCol is validated against allowlist, sortDir only accepts asc/desc
		dataQuery = fmt.Sprintf(`
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			%s
			%s
			LIMIT $%d OFFSET $%d
		`, whereClause, orderClause, argNum, argNum+1)
	}

	rows, err := s.db.QueryContext(ctx, dataQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var role, status string

		err := rows.Scan(
			&u.ID, &u.OIDCIss, &u.OIDCSub, &u.Email, &u.Name,
			&role, &status, &u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		u.Role = Role(role)
		u.Status = Status(status)
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ListResult{
		Users: users,
		Total: total,
	}, nil
}

// BulkUpdateOptions holds the fields to update for a set of users.
type BulkUpdateOptions struct {
	IDs    []string
	Role   Role
	Status Status
}

// BulkUpdate updates role and/or status for a list of user IDs.
func (s *Store) BulkUpdate(ctx context.Context, opts BulkUpdateOptions) error {
	if len(opts.IDs) == 0 {
		return nil
	}

	now := time.Now()

	for _, id := range opts.IDs {
		u, err := s.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("user not found: %s", id)
		}
		if opts.Role != "" {
			u.Role = opts.Role
		}
		if opts.Status != "" {
			u.Status = opts.Status
		}
		u.UpdatedAt = now

		query := `UPDATE users SET role = ?, status = ?, updated_at = ? WHERE id = ?`
		if s.driver == "pgx" || s.driver == "postgres" {
			query = `UPDATE users SET role = $1, status = $2, updated_at = $3 WHERE id = $4`
		}
		if _, err := s.db.ExecContext(ctx, query, string(u.Role), string(u.Status), now, id); err != nil {
			return fmt.Errorf("failed to update user %s: %w", id, err)
		}
	}

	return nil
}

// GetByEmail retrieves a user by their email address.
// Returns sql.ErrNoRows if the user does not exist.
func (s *Store) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	var role, status string

	query := `
		SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
		FROM users
		WHERE email = ?
		LIMIT 1
	`
	if s.driver == "pgx" || s.driver == "postgres" {
		query = `
			SELECT id, oidc_iss, oidc_sub, email, name, role, status, created_at, updated_at
			FROM users
			WHERE email = $1
			LIMIT 1
		`
	}

	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.OIDCIss, &u.OIDCSub, &u.Email, &u.Name,
		&role, &status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.Role = Role(role)
	u.Status = Status(status)
	return &u, nil
}

// GetOrCreate retrieves a user by OIDC identity, creating them if they don't exist.
// If no user exists for the given OIDC identity but one exists with the same email,
// the existing user is returned to prevent duplicate accounts across providers.
// This is used for auto-provisioning on first login.
func (s *Store) GetOrCreate(ctx context.Context, issuer, subject, email, name string) (*User, error) {
	// Try to get existing user by OIDC identity (fast path)
	user, err := s.GetByOIDC(ctx, issuer, subject)
	if err == nil {
		// User exists, update their email/name in case it changed
		if user.Email != email || user.Name != name {
			user.Email = email
			user.Name = name
			if err := s.Update(ctx, user); err != nil {
				return nil, fmt.Errorf("update user: %w", err)
			}
		}
		return user, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("get user by OIDC: %w", err)
	}

	// No OIDC match — check if a user with this email already exists (different provider)
	user, err = s.GetByEmail(ctx, email)
	if err == nil {
		return user, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	// No existing user at all, create them
	user = &User{
		OIDCIss: issuer,
		OIDCSub: subject,
		Email:   email,
		Name:    name,
		Role:    RoleUser,
		Status:  StatusActive,
	}

	if err := s.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}
