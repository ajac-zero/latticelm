# Authorization Architecture

## Overview

This application separates **authentication** from **authorization**:

- **Authentication**: Handled by OIDC (Clerk, Auth0, Okta, etc.) - proves WHO you are
- **Authorization**: Handled by our application database - controls WHAT you can do

## Design Principles

1. **OIDC for Authentication Only**: OIDC providers tell us who the user is (identity)
2. **Application Manages Roles**: We store and manage user roles in our own database
3. **Auto-Provisioning**: Users are automatically created on first login from OIDC
4. **Decoupled from Provider**: Changing OIDC providers doesn't affect user roles

## Database Schema

### Users Table

```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,           -- UUID (our ID)
    oidc_iss   TEXT NOT NULL,              -- OIDC issuer
    oidc_sub   TEXT NOT NULL,              -- OIDC subject
    email      TEXT NOT NULL,              -- User email
    name       TEXT NOT NULL,              -- Display name
    role       TEXT NOT NULL DEFAULT 'user', -- 'admin' or 'user'
    status     TEXT NOT NULL DEFAULT 'active', -- 'active', 'suspended', 'deleted'
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(oidc_iss, oidc_sub)
);
```

## User Flow

### First Login

1. User clicks "Login" → Redirected to OIDC provider (Clerk)
2. User authenticates with Clerk
3. Clerk redirects back with authorization code
4. Our app exchanges code for ID token
5. **Auto-Provisioning**:
   - Extract `iss` (issuer) and `sub` (subject) from ID token
   - Check if user exists: `SELECT * FROM users WHERE oidc_iss = ? AND oidc_sub = ?`
   - If not exists, create new user with `role = 'user'`
   - Store user ID in session
6. User is logged in as regular user (not admin)

### Subsequent Logins

1. Same OAuth flow
2. User already exists in database
3. Update email/name if changed in OIDC provider
4. Load user role from database (not from OIDC claims)
5. User session includes application-managed role

### Promoting to Admin

An existing admin can promote a user via:

1. Admin API endpoint: `POST /api/admin/users/:id/role` with `{"role": "admin"}`
2. Direct database update: `UPDATE users SET role = 'admin' WHERE id = '...'`
3. Bootstrap: On first deployment, manually set first user to admin

## Implementation Details

### User Store (`internal/users/store.go`)

- `GetOrCreate()`: Auto-provision users on first OIDC login
- `GetByOIDC()`: Look up user by OIDC identity
- `UpdateRole()`: Change user's role (admin operation)
- `List()`: List all users (for admin panel)

### OIDC Integration (`internal/auth/oidc_client.go`)

In `HandleCallback()`:

```go
// After parsing ID token...
claims, _ := parseIDToken(tokens.IDToken)

// Auto-provision or get existing user
user, err := userStore.GetOrCreate(ctx,
    getClaimString(claims, "iss"),  // Issuer
    getClaimString(claims, "sub"),  // Subject
    getClaimString(claims, "email"),
    getClaimString(claims, "name"),
)

// Create session with application user
sessionData := &SessionData{
    UserID:  user.ID,      // Our database ID
    Email:   user.Email,
    Name:    user.Name,
    IsAdmin: user.IsAdmin(), // From database, not OIDC
    // ...
}
```

### Auth API (`internal/auth/http_api.go`)

Session endpoint returns user info from database:

```go
{
    "authenticated": true,
    "user": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "email": "user@example.com",
        "name": "John Doe",
        "is_admin": false  // From users.role column
    }
}
```

## Migration Guide

### Step 1: Run User Migrations

```bash
# Migrations run automatically on server start
# Or manually:
cd /Users/ajac-zero/Developer/Sandbox/latticelm
go run cmd/migrate/main.go
```

### Step 2: Bootstrap First Admin

After deploying, manually set your first admin:

```sql
-- Find your user ID after first login
SELECT id, email, role FROM users;

-- Promote to admin
UPDATE users SET role = 'admin', updated_at = NOW()
WHERE email = 'your-email@example.com';
```

Or via API (if you already have an admin token):

```bash
curl -X PATCH http://localhost:8080/api/admin/users/USER_ID \
  -H "Content-Type: application/json" \
  -d '{"role": "admin"}'
```

### Step 3: Remove Old Admin Config

The old `ui.claim` and `ui.allowed_values` config is no longer used.
You can remove it from `config.yaml`:

```yaml
ui:
  enabled: true
  # These are no longer needed:
  # claim: "role"
  # allowed_values:
  #   - "admin"
```

## Security Considerations

### Advantages

1. **Provider Independence**: Switching from Clerk to Auth0 doesn't affect user roles
2. **Audit Trail**: Role changes are tracked in database (can add audit log)
3. **Granular Control**: Can add more roles (e.g., 'moderator', 'viewer') without OIDC changes
4. **Consistent**: Same authorization logic regardless of auth method (OIDC, JWT, etc.)

### Protecting Against Attacks

1. **Session Hijacking**: Use HttpOnly, Secure, SameSite cookies
2. **OIDC Provider Compromise**: Even if attacker controls OIDC, they can't become admin without database access
3. **Role Escalation**: Only admins can change roles (enforced by API middleware)

## API Endpoints (Future)

Admin-only endpoints to be added:

- `GET /api/admin/users` - List all users
- `GET /api/admin/users/:id` - Get user details
- `PATCH /api/admin/users/:id` - Update user (role, status)
- `DELETE /api/admin/users/:id` - Soft delete user

## Testing

### Test Auto-Provisioning

1. Clear database: `DELETE FROM users;`
2. Login via OIDC
3. Check database: `SELECT * FROM users;`
4. Should see new user with `role = 'user'`

### Test Role-Based Access

1. As regular user, try to access `/dashboard` → Redirected to `/chat`
2. Promote user to admin in database
3. Logout and login again
4. Should now see `/dashboard` link and have access

## Future Enhancements

- [ ] User management UI in admin dashboard
- [ ] Audit log for role changes
- [ ] Fine-grained permissions (beyond just admin/user)
- [ ] Team/organization support
- [ ] API keys per user
