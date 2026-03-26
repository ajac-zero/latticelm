package users

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// sharedDB is a single Postgres instance shared across all tests in this package.
var sharedDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic(err)
	}

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	sharedDB, err = sql.Open("pgx", connStr)
	if err != nil {
		panic(err)
	}

	if _, err := Migrate(ctx, sharedDB, "pgx"); err != nil {
		panic(err)
	}

	code := m.Run()

	sharedDB.Close()
	_ = pgCtr.Terminate(ctx)
	os.Exit(code)
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(sharedDB, "pgx")
}

func makeUser(iss, sub, email, name string) *User {
	return &User{
		OIDCIss: iss,
		OIDCSub: sub,
		Email:   email,
		Name:    name,
	}
}

func TestMigrate(t *testing.T) {
	// Idempotent second run.
	version, err := Migrate(context.Background(), sharedDB, "pgx")
	require.NoError(t, err)
	assert.Equal(t, expectedSchemaVersion, version)
}

func TestCheckSchemaVersion(t *testing.T) {
	require.NoError(t, CheckSchemaVersion(context.Background(), sharedDB))
}

func TestStore_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-1", "alice@example.com", "Alice")
	require.NoError(t, s.Create(ctx, u))
	assert.NotEmpty(t, u.ID)
	assert.False(t, u.CreatedAt.IsZero())

	got, err := s.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, RoleUser, got.Role)
	assert.Equal(t, StatusActive, got.Status)
}

func TestStore_GetByID_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByID(context.Background(), "nonexistent-id")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_GetByOIDC(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-oidc", "bob@example.com", "Bob")
	require.NoError(t, s.Create(ctx, u))

	got, err := s.GetByOIDC(ctx, "https://issuer.example", "sub-oidc")
	require.NoError(t, err)
	assert.Equal(t, "bob@example.com", got.Email)
}

func TestStore_GetByOIDC_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByOIDC(context.Background(), "https://issuer.example", "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_GetBySub(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-unique", "carol@example.com", "Carol")
	require.NoError(t, s.Create(ctx, u))

	got, err := s.GetBySub(ctx, "sub-unique")
	require.NoError(t, err)
	assert.Equal(t, "carol@example.com", got.Email)
}

func TestStore_GetByEmail(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-email", "dave@example.com", "Dave")
	require.NoError(t, s.Create(ctx, u))

	got, err := s.GetByEmail(ctx, "dave@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestStore_Update(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-update", "eve@example.com", "Eve")
	require.NoError(t, s.Create(ctx, u))

	u.Name = "Eve Updated"
	u.Role = RoleAdmin
	require.NoError(t, s.Update(ctx, u))

	got, err := s.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Eve Updated", got.Name)
	assert.Equal(t, RoleAdmin, got.Role)
}

func TestStore_Update_NotFound(t *testing.T) {
	s := newStore(t)
	u := &User{ID: "nonexistent", Email: "x@x.com", Name: "X", Role: RoleUser, Status: StatusActive}
	err := s.Update(context.Background(), u)
	assert.Error(t, err)
}

func TestStore_UpdateRole(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-role", "frank@example.com", "Frank")
	require.NoError(t, s.Create(ctx, u))

	require.NoError(t, s.UpdateRole(ctx, u.ID, RoleAdmin))

	got, err := s.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, got.Role)
}

func TestStore_UpdateRole_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.UpdateRole(context.Background(), "nonexistent", RoleAdmin)
	assert.Error(t, err)
}

func TestStore_List(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	for i, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		u := makeUser("https://issuer.example", "sub-list-"+string(rune('a'+i)), email, "User")
		require.NoError(t, s.Create(ctx, u))
	}

	all, err := s.List(ctx, "")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(all), 3)

	active, err := s.List(ctx, StatusActive)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(active), 3)
}

func TestStore_ListWithOptions(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	admin := makeUser("https://issuer.example", "sub-admin-list", "admin@x.com", "Admin User")
	admin.Role = RoleAdmin
	require.NoError(t, s.Create(ctx, admin))

	regular := makeUser("https://issuer.example", "sub-regular-list", "regular@x.com", "Regular User")
	require.NoError(t, s.Create(ctx, regular))

	t.Run("filter by role", func(t *testing.T) {
		result, err := s.ListWithOptions(ctx, ListOptions{Role: RoleAdmin})
		require.NoError(t, err)
		for _, u := range result.Users {
			assert.Equal(t, RoleAdmin, u.Role)
		}
	})

	t.Run("search by name", func(t *testing.T) {
		result, err := s.ListWithOptions(ctx, ListOptions{Search: "Admin User"})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.Total, 1)
	})

	t.Run("pagination", func(t *testing.T) {
		result, err := s.ListWithOptions(ctx, ListOptions{Page: 1, Limit: 1})
		require.NoError(t, err)
		assert.Len(t, result.Users, 1)
	})

	t.Run("sort by name asc", func(t *testing.T) {
		result, err := s.ListWithOptions(ctx, ListOptions{SortBy: "name", SortDir: "asc"})
		require.NoError(t, err)
		assert.NotEmpty(t, result.Users)
	})
}

func TestStore_BulkUpdate(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u1 := makeUser("https://issuer.example", "sub-bulk-1", "bulk1@x.com", "Bulk1")
	u2 := makeUser("https://issuer.example", "sub-bulk-2", "bulk2@x.com", "Bulk2")
	require.NoError(t, s.Create(ctx, u1))
	require.NoError(t, s.Create(ctx, u2))

	err := s.BulkUpdate(ctx, BulkUpdateOptions{
		IDs:    []string{u1.ID, u2.ID},
		Status: StatusSuspended,
	})
	require.NoError(t, err)

	for _, id := range []string{u1.ID, u2.ID} {
		got, err := s.GetByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, StatusSuspended, got.Status)
	}
}

func TestStore_BulkUpdate_Empty(t *testing.T) {
	s := newStore(t)
	err := s.BulkUpdate(context.Background(), BulkUpdateOptions{IDs: []string{}})
	require.NoError(t, err)
}

func TestStore_BulkUpdate_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.BulkUpdate(context.Background(), BulkUpdateOptions{
		IDs:  []string{"nonexistent"},
		Role: RoleAdmin,
	})
	assert.Error(t, err)
}

func TestStore_GetOrCreate_NewUser(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u, err := s.GetOrCreate(ctx, "https://issuer.example", "sub-new", "new@x.com", "New User")
	require.NoError(t, err)
	assert.NotEmpty(t, u.ID)
	assert.Equal(t, "new@x.com", u.Email)
	assert.Equal(t, RoleUser, u.Role)
}

func TestStore_GetOrCreate_ExistingByOIDC(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u, err := s.GetOrCreate(ctx, "https://issuer.example", "sub-existing", "existing@x.com", "Existing")
	require.NoError(t, err)

	// Second call returns same user (updates name).
	u2, err := s.GetOrCreate(ctx, "https://issuer.example", "sub-existing", "existing@x.com", "Existing Updated")
	require.NoError(t, err)
	assert.Equal(t, u.ID, u2.ID)
	assert.Equal(t, "Existing Updated", u2.Name)
}

func TestStore_GetOrCreate_ExistingByEmail(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Create user with one OIDC identity.
	existing := makeUser("https://issuer1.example", "sub-a", "shared@x.com", "Shared")
	require.NoError(t, s.Create(ctx, existing))

	// GetOrCreate with different OIDC identity but same email returns existing.
	u, err := s.GetOrCreate(ctx, "https://issuer2.example", "sub-b", "shared@x.com", "Shared")
	require.NoError(t, err)
	assert.Equal(t, existing.ID, u.ID)
}

func TestStore_ResolveUserSub(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-resolve", "resolve@x.com", "Resolve User")
	require.NoError(t, s.Create(ctx, u))

	name, email, err := s.ResolveUserSub(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Resolve User", name)
	assert.Equal(t, "resolve@x.com", email)
}

func TestUser_IsAdmin(t *testing.T) {
	assert.True(t, (&User{Role: RoleAdmin}).IsAdmin())
	assert.False(t, (&User{Role: RoleUser}).IsAdmin())
}

func TestUser_IsActive(t *testing.T) {
	assert.True(t, (&User{Status: StatusActive}).IsActive())
	assert.False(t, (&User{Status: StatusSuspended}).IsActive())
	assert.False(t, (&User{Status: StatusDeleted}).IsActive())
}

func TestStore_Create_DefaultsRoleAndStatus(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := &User{OIDCIss: "https://issuer.example", OIDCSub: "sub-defaults", Email: "defaults@x.com", Name: "Defaults"}
	require.NoError(t, s.Create(ctx, u))
	assert.Equal(t, RoleUser, u.Role)
	assert.Equal(t, StatusActive, u.Status)
}

func TestStore_Create_PreservesExplicitID(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := &User{
		ID:      "explicit-id-123",
		OIDCIss: "https://issuer.example",
		OIDCSub: "sub-explicit",
		Email:   "explicit@x.com",
		Name:    "Explicit",
	}
	require.NoError(t, s.Create(ctx, u))
	assert.Equal(t, "explicit-id-123", u.ID)
}

func TestStore_List_Suspended(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	u := makeUser("https://issuer.example", "sub-suspended", "suspended@x.com", "Suspended")
	require.NoError(t, s.Create(ctx, u))
	u.Status = StatusSuspended
	require.NoError(t, s.Update(ctx, u))

	suspended, err := s.List(ctx, StatusSuspended)
	require.NoError(t, err)
	found := false
	for _, su := range suspended {
		if su.ID == u.ID {
			found = true
		}
	}
	assert.True(t, found)
}

// TestStore_Create_AutoTimestamps verifies CreatedAt/UpdatedAt are set within a reasonable range.
func TestStore_Create_AutoTimestamps(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	before := time.Now().Add(-time.Second)
	u := makeUser("https://issuer.example", "sub-ts", "ts@x.com", "Timestamps")
	require.NoError(t, s.Create(ctx, u))
	after := time.Now().Add(time.Second)

	assert.True(t, u.CreatedAt.After(before))
	assert.True(t, u.CreatedAt.Before(after))
}
