# Authentication & Authorization

## Overview

The UI enforces proper **authentication** (via OIDC) and **authorization** (via database) when auth is enabled.

- **Authentication**: OIDC providers (Clerk, Auth0, Okta) prove WHO you are
- **Authorization**: Our database controls WHAT you can do

## Architecture

### Authentication Flow (OIDC)

1. **When auth is disabled**: All routes are accessible without authentication
2. **When auth is enabled**:
   - All visitors must authenticate before accessing any part of the UI
   - Unauthenticated visitors are redirected to `/auth/login`
   - OIDC provider handles login (Clerk, Auth0, etc.)
   - Our app receives ID token with user identity

### Authorization Flow (Database)

1. **First Login** (Auto-Provisioning):
   - Extract `iss` and `sub` from OIDC ID token
   - Check database: `SELECT * FROM users WHERE oidc_iss = ? AND oidc_sub = ?`
   - If user doesn't exist, create with `role = 'user'`
   - Store user's database ID in session

2. **Subsequent Logins**:
   - Look up existing user by OIDC identity
   - Load `role` from database (not from OIDC claims)
   - Update email/name if changed in OIDC provider

## Role-Based Access Control

The system uses the `users.role` database column to determine access levels:

### Admin Users (`is_admin: true`)
- Can access `/dashboard` (system configuration, health, providers)
- Can access `/chat` (LLM playground)
- Home link redirects to `/dashboard`

### Regular Users (`is_admin: false`)
- Can access `/chat` (LLM playground)
- Cannot access `/dashboard` (redirected to `/chat` if attempted)
- Dashboard link hidden in sidebar
- Home link redirects to `/chat`

## Database Schema

Users are stored in the `users` table:

```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,           -- UUID (our ID)
    oidc_iss   TEXT NOT NULL,              -- OIDC issuer URL
    oidc_sub   TEXT NOT NULL,              -- OIDC subject (unique per provider)
    email      TEXT NOT NULL,              -- User email
    name       TEXT NOT NULL,              -- Display name
    role       TEXT NOT NULL DEFAULT 'user', -- 'admin' or 'user'
    status     TEXT NOT NULL DEFAULT 'active', -- 'active', 'suspended', 'deleted'
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(oidc_iss, oidc_sub)
);
```

## OIDC Configuration

The OIDC provider only handles **authentication**. It provides:

- `iss` (issuer) - Identifies the OIDC provider
- `sub` (subject) - Unique user ID from the provider
- `email` - User's email address
- `name` - User's display name

**Roles are NOT read from OIDC claims.** They are managed in our database.

### Example OIDC ID Token

```json
{
  "iss": "https://adapted-beetle-23.clerk.accounts.dev",
  "sub": "user_abc123xyz",
  "email": "user@example.com",
  "name": "John Doe",
  "aud": "your-client-id",
  "exp": 1234567890
}
```

After authentication, we:
1. Look up or create user in database using `oidc_iss` + `oidc_sub`
2. Load the user's `role` from our database
3. Store user session with database-managed role

## JWT Token Authentication (API Access)

For direct API access with JWT tokens (non-OIDC), the old claim-based approach still works:

```json
{
  "sub": "user-789",
  "email": "admin@example.com",
  "role": "admin",  // This grants admin access for API calls
  "iss": "your-jwt-issuer",
  "aud": "your-audience",
  "exp": 1234567890
}
```

**Note**: JWT token auth is separate from UI OIDC auth. It's used for API-to-API communication.

## Security Features

1. **No fail-open**: If auth is enabled and the session check fails, users are blocked (not allowed through)
2. **Route guards**: Both `requireAuth` and `requireAdmin` guards prevent unauthorized access
3. **UI hiding**: Admin-only features are hidden from the UI for non-admin users
4. **Root-level enforcement**: The app root checks auth status and redirects before rendering any routes
5. **Session-based auth**: Uses HttpOnly cookies for OIDC to prevent XSS attacks

## Backend Configuration Example

```yaml
auth:
  enabled: true
  issuer: "https://adapted-beetle-23.clerk.accounts.dev"
  audience: "your-client-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/api/auth/callback"
  admin_email: "admin@example.com"  # Auto-promote this email to admin on first login

conversations:
  enabled: true
  store: "sql"  # Required for user management
  driver: "pgx"  # or "postgres", "mysql", "sqlite3"
  dsn: "postgresql://user:pass@host/db"

ui:
  enabled: true
  # Old claim/allowed_values config is deprecated
  # Roles are now managed in the database
```

## Bootstrapping First Admin

You have three options to promote your first admin user:

### Option 1: Config File (Recommended) ⭐

Set `admin_email` in your `config.yaml`:

```yaml
auth:
  enabled: true
  issuer: "https://your-oidc-provider.com"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/api/auth/callback"
  admin_email: "your-email@example.com"  # Auto-promote on first login
```

**How it works:**
- When the user with this email logs in for the first time, they are automatically promoted to admin
- Only happens if the user currently has `role='user'` (won't affect existing admins)
- Server logs the promotion:
  ```
  INFO auto-promoting user to admin email=your@example.com user_id=...
  INFO user promoted to admin
  ```
- Config field is explicit, documented, and version-controlled

### Option 2: Manual SQL

```sql
-- View all users
SELECT id, email, name, role, status FROM users;

-- Promote to admin
UPDATE users
SET role = 'admin', updated_at = NOW()
WHERE email = 'your-email@example.com';
```

### Option 3: Using the Bootstrap Script

```bash
./scripts/bootstrap_admin.sh your-email@example.com
```

## Testing

To test role-based access:

1. **First login**: Login via OIDC → User is auto-created with `role = 'user'`
2. **Check database**: `SELECT * FROM users;` → Should see new user
3. **Try accessing dashboard**: As regular user → Redirected to `/chat`
4. **Promote to admin**: Run bootstrap script or manual SQL
5. **Logout and login**: New session loads `role = 'admin'` from database
6. **Access dashboard**: Now works! Dashboard link appears in sidebar
