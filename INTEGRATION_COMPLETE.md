# User System Integration - COMPLETE ✅

## What Was Done

### 1. Created User Management System

**Files Created:**
- `internal/users/user.go` - User model with roles (admin/user) and status
- `internal/users/store.go` - Database operations (GetOrCreate, UpdateRole, etc.)
- `internal/users/migrate.go` - Migration system for user schema
- `internal/users/migrations/001_create_users.sql` - Database schema

**Key Features:**
- Auto-provisioning on first OIDC login
- Role-based authorization (admin/user)
- Account status management (active/suspended/deleted)
- Linked to OIDC identity via `oidc_iss` + `oidc_sub`

### 2. Integrated with OIDC Authentication

**Files Modified:**
- `internal/auth/oidc_client.go`:
  - Added `userStore *users.Store` to struct
  - Updated `NewOIDCClient` to accept user store
  - Modified `HandleCallback` to auto-provision users
  - Check user account status before creating session
  - Set `IsAdmin` from database, not OIDC claims

**Key Changes:**
```go
// Old (claims-based):
isAdmin := hasAdminAccess(claims, c.adminCfg)

// New (database-based):
user, _ := c.userStore.GetOrCreate(ctx, issuer, subject, email, name)
sessionData.IsAdmin = user.IsAdmin()
```

### 3. Updated Server Initialization

**File Modified:**
- `cmd/gateway/main.go`:
  - Import `internal/users` package
  - Initialize user store after conversation store
  - Run user migrations on startup
  - Pass user store to `NewOIDCClient`
  - Pass user store to `NewAPI`
  - Require SQL database for OIDC auth

**Migration Flow:**
```
Server Start
  → Open Database Connection
  → Run Conversation Migrations
  → Run User Migrations ✅
  → Create User Store
  → Initialize OIDC Client (with user store)
  → Start Server
```

### 4. Enhanced Auth API

**File Modified:**
- `internal/auth/http_api.go`:
  - Added `userStore *users.Store` to API struct
  - Updated `NewAPI` to accept user store
  - Enhanced `/api/auth/debug/claims` to show database user info

**Debug Output:**
```json
{
  "mode": "oidc",
  "claims": { /* OIDC ID token claims */ },
  "is_admin": true,
  "database_user": {
    "id": "uuid",
    "email": "user@example.com",
    "role": "admin",
    "status": "active"
  }
}
```

### 5. Created Bootstrap Tools

**Files Created:**
- `scripts/bootstrap_admin.sh` - Shell script to promote admin via psql
- `scripts/promote_admin.sql` - SQL template for manual promotion

**Usage:**
```bash
./scripts/bootstrap_admin.sh your-email@example.com
```

### 6. Updated Documentation

**Files Modified:**
- `ui/AUTH.md` - Updated to reflect database-based authorization
- `AUTHORIZATION.md` - Complete architecture documentation

**Files Created:**
- `TODO_USER_SYSTEM.md` - Integration checklist (now complete)
- `INTEGRATION_COMPLETE.md` - This file

## How It Works Now

### First Login Flow

1. User clicks "Login" → Redirected to Clerk/OIDC provider
2. User authenticates with provider
3. Provider redirects back with authorization code
4. Backend exchanges code for ID token
5. **Auto-Provisioning**:
   ```go
   user, err := userStore.GetOrCreate(ctx,
       oidcIssuer,    // "https://clerk.example.com"
       oidcSubject,   // "user_abc123"
       email,         // "user@example.com"
       name,          // "John Doe"
   )
   // User created with role = 'user' (not admin)
   ```
6. Session created with `user.ID` and `user.IsAdmin()`
7. User lands on `/chat` (not admin yet)

### Promoting to Admin

```sql
UPDATE users SET role = 'admin', updated_at = NOW()
WHERE email = 'user@example.com';
```

### Subsequent Login

1. User authenticates with OIDC provider
2. Backend looks up existing user:
   ```go
   user, _ := userStore.GetByOIDC(ctx, issuer, subject)
   ```
3. Updates email/name if changed
4. Session gets `IsAdmin: user.IsAdmin()` from database
5. User now has access to `/dashboard`

## Database Schema

```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,           -- UUID
    oidc_iss   TEXT NOT NULL,              -- OIDC issuer
    oidc_sub   TEXT NOT NULL,              -- OIDC subject
    email      TEXT NOT NULL,
    name       TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'user',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(oidc_iss, oidc_sub)
);

CREATE TABLE users_schema_migrations (
    version     INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at  TIMESTAMP NOT NULL
);
```

## Testing the Integration

### 1. Build and Run

```bash
cd /Users/ajac-zero/Developer/Sandbox/latticelm

# Build
go build -o gateway cmd/gateway/main.go

# Run (migrations run automatically on startup)
./gateway
```

**Expected Output:**
```
INFO user schema ready version=1
INFO user store initialized
INFO OIDC client enabled for UI authentication
INFO server listening address=:8080
```

### 2. First Login

1. Navigate to `http://localhost:8080`
2. Click "Login" → Redirected to Clerk
3. Authenticate
4. You're back at the app, on `/chat`
5. No dashboard link in sidebar (not admin yet)

### 3. Check Database

```sql
SELECT id, email, name, role, status, created_at
FROM users;
```

**Expected:**
```
id                                   | email                | role | status
-------------------------------------|---------------------|------|--------
550e8400-e29b-41d4-a716-446655440000 | you@example.com     | user | active
```

### 4. Debug Claims

Navigate to: `http://localhost:8080/debug/claims`

**Expected Output:**
```json
{
  "mode": "oidc",
  "claims": {
    "iss": "https://adapted-beetle-23.clerk.accounts.dev",
    "sub": "user_3An8ARtd8wNDchypQyFOED4PNK3",
    "email": "you@example.com",
    "name": "Your Name"
  },
  "is_admin": false,
  "database_user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "you@example.com",
    "role": "user",
    "status": "active"
  }
}
```

### 5. Promote to Admin

```bash
./scripts/bootstrap_admin.sh you@example.com
```

Or manually:
```sql
UPDATE users SET role = 'admin', updated_at = NOW()
WHERE email = 'you@example.com';
```

### 6. Logout and Login

1. Click logout
2. Login again
3. Now you see "Dashboard" in sidebar
4. Can access `/dashboard`
5. `/debug/claims` shows `"is_admin": true`

## Key Benefits

### 1. Provider Independence
- Switch from Clerk → Auth0 → Okta without affecting user roles
- OIDC identity changes don't affect authorization

### 2. Centralized Control
- Manage roles in your database
- Audit trail of role changes (can add audit log table)
- Fine-grained control beyond just admin/user

### 3. Security
- Even if attacker compromises OIDC provider, they can't become admin
- Session hijacking doesn't grant role escalation
- HttpOnly cookies prevent XSS attacks

### 4. Flexibility
- Can add more roles: 'moderator', 'viewer', 'api_user'
- Can add team/organization support
- Can add fine-grained permissions later

## Configuration

### Required Config

```yaml
auth:
  enabled: true
  issuer: "https://your-clerk-domain.clerk.accounts.dev"
  audience: "your-client-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/api/auth/callback"

conversations:
  enabled: true
  store: "sql"  # REQUIRED for user management
  driver: "pgx"
  dsn: "postgresql://user:pass@host/db"

ui:
  enabled: true
```

### Deprecated Config

```yaml
ui:
  # These are no longer used for OIDC auth:
  # claim: "role"
  # allowed_values:
  #   - "admin"
```

**Note**: `claim` and `allowed_values` are still used for JWT token auth (API access), but not for OIDC UI authentication.

## Troubleshooting

### "OIDC authentication requires a SQL database"

**Cause:** OIDC auth is enabled but no SQL database configured.

**Fix:**
```yaml
conversations:
  enabled: true
  store: "sql"
  dsn: "your-database-connection-string"
```

### "User migration failed"

**Cause:** Database connection issue or migration error.

**Fix:**
1. Check database is running: `psql $DATABASE_DSN`
2. Check migrations exist: `ls internal/users/migrations/`
3. Check logs for specific error

### "Not authenticated" on /debug/claims

**Cause:** Session expired or not logged in.

**Fix:**
1. Logout completely
2. Login again
3. Try /debug/claims again

### User still not admin after promotion

**Cause:** Session still has old role.

**Fix:**
1. Logout
2. Login again (forces session refresh from database)

## Future Enhancements

- [ ] User management API endpoints (list, update, delete users)
- [ ] User management UI in admin dashboard
- [ ] Audit log for role changes
- [ ] Fine-grained permissions (beyond admin/user)
- [ ] Team/organization support
- [ ] API keys per user
- [ ] User invitation system
- [ ] Bootstrap admin via environment variable
