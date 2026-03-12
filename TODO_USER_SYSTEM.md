# User System Integration - TODO

## Completed ✅

- [x] Created `internal/users` package with User model
- [x] Created database migrations for `users` table
- [x] Created `Store` with auto-provisioning (`GetOrCreate`)
- [x] Reverted OIDC claim-based authorization
- [x] Documentation (AUTHORIZATION.md)

## Remaining Work

### 1. Integrate User Store with OIDC Client

**File**: `internal/auth/oidc_client.go`

Update the `OIDCClient` struct to include user store:

```go
type OIDCClient struct {
    cfg          OIDCClientConfig
    client       *http.Client
    logger       *slog.Logger
    sessionStore *SessionStore
    userStore    *users.Store  // ADD THIS
    adminCfg     AdminConfig   // REMOVE THIS (no longer needed)
    // ...
}
```

Update `NewOIDCClient`:

```go
func NewOIDCClient(
    cfg OIDCClientConfig,
    sessionStore *SessionStore,
    userStore *users.Store,  // ADD THIS
    logger *slog.Logger,
) (*OIDCClient, error) {
    // ...
}
```

Update `HandleCallback` (around line 269-280):

```go
// Parse ID token to get user info
claims, err := c.parseIDToken(tokens.IDToken)
if err != nil {
    c.logger.Error("failed to parse ID token", slog.String("error", err.Error()))
    http.Error(w, "Authentication failed", http.StatusInternalServerError)
    return
}

// AUTO-PROVISION: Get or create user in database
user, err := c.userStore.GetOrCreate(ctx,
    getClaimString(claims, "iss"),    // OIDC issuer
    getClaimString(claims, "sub"),    // OIDC subject
    getClaimString(claims, "email"),  // Email from OIDC
    getClaimString(claims, "name"),   // Name from OIDC
)
if err != nil {
    c.logger.Error("failed to provision user", slog.String("error", err.Error()))
    http.Error(w, "Authentication failed", http.StatusInternalServerError)
    return
}

// Create session with application user data
sessionData := &SessionData{
    UserID:       user.ID,           // Our database ID
    Email:        user.Email,
    Name:         user.Name,
    IDToken:      tokens.IDToken,
    AccessToken:  tokens.AccessToken,
    RefreshToken: tokens.RefreshToken,
    IsAdmin:      user.IsAdmin(),    // From database, not OIDC claims
}
```

### 2. Update Server Initialization

**File**: `cmd/gateway/main.go` or `internal/server/server.go`

Initialize user store and run migrations:

```go
import (
    "latticelm/internal/users"
    // ...
)

// In main() or server setup:

// Run user migrations
userSchemaVersion, err := users.Migrate(ctx, db, driver)
if err != nil {
    log.Fatal("user migration failed", slog.String("error", err.Error()))
}
log.Info("user schema ready", slog.Int("version", userSchemaVersion))

// Create user store
userStore := users.NewStore(db, driver)

// Update OIDC client creation
oidcClient, err := auth.NewOIDCClient(
    oidcCfg,
    sessionStore,
    userStore,    // Pass user store
    logger,
)
```

### 3. Update SessionData Structure

**File**: `internal/auth/session.go`

The `UserID` field should now store our database ID (not OIDC sub):

```go
type SessionData struct {
    UserID       string // Our database user ID (UUID)
    Email        string
    Name         string
    IDToken      string
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    IsAdmin      bool   // From database users.role column
}
```

### 4. Clean Up Old Admin Config

**Files to update**:
- `internal/config/config.go` - Keep `UIConfig` but document that `claim`/`allowed_values` are deprecated
- `config.yaml` - Remove or comment out `ui.claim` and `ui.allowed_values`

### 5. Bootstrap First Admin User

After deployment, manually promote first user:

```sql
-- Option 1: Promote by email
UPDATE users
SET role = 'admin', updated_at = NOW()
WHERE email = 'your-email@example.com';

-- Option 2: Promote by OIDC subject
UPDATE users
SET role = 'admin', updated_at = NOW()
WHERE oidc_sub = 'user_abc123';
```

Or add an environment variable for bootstrap admin:

```go
// In server initialization:
if bootstrapAdminEmail := os.Getenv("BOOTSTRAP_ADMIN_EMAIL"); bootstrapAdminEmail != "" {
    // Find user and promote to admin
}
```

### 6. Update Auth API Debug Endpoint

**File**: `internal/auth/http_api.go`

Update `HandleDebugClaims` to show database user info:

```go
func (a *API) HandleDebugClaims(w http.ResponseWriter, r *http.Request) {
    // ... existing OIDC session check ...

    // ADD: Show database user info
    if session.UserID != "" && a.userStore != nil {
        dbUser, err := a.userStore.GetByID(r.Context(), session.UserID)
        if err == nil {
            response["database_user"] = map[string]interface{}{
                "id":      dbUser.ID,
                "email":   dbUser.Email,
                "role":    dbUser.Role,
                "status":  dbUser.Status,
            }
        }
    }

    writeJSONSuccess(w, response)
}
```

### 7. Update Frontend Types

**File**: `ui/src/lib/api/types.ts`

Update the `User` type to include database ID:

```typescript
export interface User {
  id: string        // Our database UUID
  email: string
  name: string
  is_admin: boolean // From users.role column
}
```

## Testing Checklist

- [ ] Migrations run successfully
- [ ] First OIDC login creates user with `role='user'`
- [ ] Subsequent logins reuse existing user record
- [ ] Email/name updates from OIDC provider sync to database
- [ ] Regular users cannot access `/dashboard`
- [ ] Promoting user to admin in DB grants dashboard access
- [ ] `/debug/claims` shows database user info
- [ ] Session persists user's role across requests

## Database Verification

```sql
-- Check users table exists
SELECT * FROM users LIMIT 1;

-- Check schema version
SELECT * FROM users_schema_migrations;

-- List all users
SELECT id, email, name, role, status FROM users;

-- Promote user to admin
UPDATE users SET role = 'admin' WHERE email = 'your-email@example.com';
```
