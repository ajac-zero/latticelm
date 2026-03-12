# Admin Email Auto-Promotion Feature

## Overview

The `admin_email` config field provides an explicit, documented way to bootstrap your first admin user without manual database intervention.

## Configuration

Add to your `config.yaml`:

```yaml
auth:
  enabled: true
  issuer: "https://your-oidc-provider.com"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/api/auth/callback"
  admin_email: "your-email@example.com"  # Auto-promote on first login
```

## How It Works

1. **User Logs In**: User authenticates via OIDC (Clerk, Auth0, etc.)
2. **Auto-Provision**: User is created in database if they don't exist
3. **Email Check**: If `user.email == config.auth.admin_email`
4. **Role Check**: AND `user.role == 'user'` (not already admin)
5. **Auto-Promote**: Update user role to 'admin' in database
6. **Session**: User session includes `IsAdmin: true`

## Server Logs

When auto-promotion happens, you'll see:

```
INFO auto-promoting user to admin email=your@example.com user_id=550e8400-...
INFO user promoted to admin
INFO user authenticated user_id=550e8400-... email=your@example.com is_admin=true
```

## Safety Features

### Only Promotes Regular Users
- Checks `user.Role == users.RoleUser` before promoting
- Won't demote existing admins or affect their role
- Idempotent: safe to leave in config after first login

### Only on First Login
- Promotion happens during OIDC callback (login flow)
- Not checked on every request (performance)
- If user is already admin, no database update

### Doesn't Fail Login on Error
```go
if err := c.userStore.UpdateRole(ctx, user.ID, users.RoleAdmin); err != nil {
    c.logger.Error("failed to promote user to admin", slog.String("error", err.Error()))
    // Don't fail the login, just log the error
}
```

User still gets logged in as regular user if promotion fails (can be fixed manually).

## Benefits Over Alternatives

### vs. Environment Variable
- ✅ **Documented**: Visible in config file, can add comments
- ✅ **Version Controlled**: Part of your infrastructure as code
- ✅ **Discoverable**: Appears in `config.example.yaml`
- ✅ **IDE Support**: YAML schema can provide autocomplete/validation

### vs. Manual SQL
- ✅ **No SQL Knowledge Required**: Just edit config
- ✅ **No Database Access Needed**: Don't need psql/mysql client
- ✅ **Automated**: Happens on first login automatically
- ✅ **No Timing Issues**: Can't forget to promote after deployment

### vs. Bootstrap Script
- ✅ **No Extra Step**: Happens during normal login flow
- ✅ **Cross-Platform**: Works on Windows, Linux, macOS
- ✅ **No Script Dependencies**: Doesn't require bash/psql

## Security Considerations

### Accidental Exposure
**Risk**: Someone else logs in with the admin email before you

**Mitigations**:
1. Set `admin_email` to YOUR email (the one you'll use to login)
2. Deploy in private environment first (not public internet)
3. Login immediately after deployment
4. Remove `admin_email` from config after first login (optional)

### Config Leaks
**Risk**: Config file with admin_email is committed to public repo

**Mitigations**:
1. Use environment-specific configs (dev vs prod)
2. `.gitignore` your production config
3. Use secrets management (Vault, k8s secrets) for production
4. Admin email is not a secret - it's the user's actual email

### Email Spoofing
**Risk**: Attacker spoofs email in OIDC claims

**Mitigations**:
1. Email comes from trusted OIDC provider (Clerk, Auth0)
2. OIDC ID token is cryptographically signed
3. We verify token signature before trusting claims
4. Attacker would need to compromise your OIDC provider

## Recommended Workflow

### Development
```yaml
auth:
  admin_email: "dev@example.com"  # Your development email
```

### Production - Option 1 (Permanent)
```yaml
auth:
  admin_email: "admin@yourcompany.com"  # Keep in config
```

Safe if you control the deployment and login first.

### Production - Option 2 (Temporary)
```yaml
auth:
  admin_email: "admin@yourcompany.com"  # Remove after first login
```

1. Deploy with `admin_email` set
2. Login immediately
3. Edit config, remove `admin_email`
4. Redeploy or restart server

### Production - Option 3 (External Secrets)
```yaml
auth:
  admin_email: "${ADMIN_EMAIL}"  # From secrets manager
```

Use environment variables or secrets injection.

## Example: Complete Setup

### 1. Configure

```yaml
# config.yaml
auth:
  enabled: true
  issuer: "https://your-clerk-instance.clerk.accounts.dev"
  audience: "your-client-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "https://yourapp.com/api/auth/callback"
  admin_email: "you@yourcompany.com"

conversations:
  enabled: true
  store: "sql"
  driver: "pgx"
  dsn: "postgresql://user:pass@host/db"

ui:
  enabled: true
```

### 2. Deploy

```bash
./gateway
```

**Server Output:**
```
INFO user schema ready version=1
INFO OIDC client enabled for UI authentication
INFO auto-promotion configured for admin email email=you@yourcompany.com
INFO server listening address=:8080
```

### 3. First Login

Visit `https://yourapp.com`, click "Login"

**Server Output:**
```
INFO request started path=/api/auth/oidc/login
INFO request started path=/api/auth/callback
INFO auto-promoting user to admin email=you@yourcompany.com user_id=550e8400-...
INFO user promoted to admin
INFO user authenticated email=you@yourcompany.com is_admin=true
```

### 4. Verify

Navigate to `https://yourapp.com/debug/claims`

**Expected:**
```json
{
  "mode": "oidc",
  "is_admin": true,
  "database_user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "you@yourcompany.com",
    "role": "admin",
    "status": "active"
  }
}
```

### 5. (Optional) Remove Config

```yaml
auth:
  # admin_email: "you@yourcompany.com"  # Commented out after first login
```

## Troubleshooting

### "auto-promotion configured" but user not promoted

**Check logs for:**
```
INFO user authenticated email=different@email.com is_admin=false
```

**Cause**: Email mismatch between config and OIDC provider

**Fix**: Update `admin_email` to match the actual email from OIDC

### "failed to promote user to admin"

**Check logs for:**
```
ERROR failed to promote user to admin error=...
```

**Possible causes:**
- Database connection issue
- User doesn't exist (shouldn't happen, created just before)
- Permission issue on users table

**Fix**: Check database connectivity, then manually promote via SQL

### User promoted but still sees `/chat`

**Cause**: Old session in browser

**Fix**:
1. Logout completely
2. Clear browser cookies for your domain
3. Login again (new session will have `is_admin: true`)

## Implementation Details

### Code Location

**Config Struct** (`internal/config/config.go:128-136`):
```go
type AuthConfig struct {
    // ...
    AdminEmail string `yaml:"admin_email"`
}
```

**OIDC Client Config** (`internal/auth/oidc_client.go:26-32`):
```go
type OIDCClientConfig struct {
    // ...
    AdminEmail string
}
```

**Auto-Promotion Logic** (`internal/auth/oidc_client.go:271-300`):
```go
// Auto-provision or get existing user
user, _ := c.userStore.GetOrCreate(ctx, issuer, subject, email, name)

// Auto-promote to admin if email matches
if c.cfg.AdminEmail != "" &&
   user.Email == c.cfg.AdminEmail &&
   user.Role == users.RoleUser {

    c.logger.Info("auto-promoting user to admin", ...)
    c.userStore.UpdateRole(ctx, user.ID, users.RoleAdmin)
    user.Role = users.RoleAdmin
    c.logger.Info("user promoted to admin", ...)
}
```

### Database Query

```sql
UPDATE users
SET role = 'admin', updated_at = NOW()
WHERE id = $1
```

Only executed when:
1. `admin_email` is configured
2. User's email matches config
3. User's current role is 'user'

## Alternatives

If you prefer NOT to use `admin_email`, you can still:

1. **Manual SQL**: Connect to database, run `UPDATE users SET role='admin' ...`
2. **Bootstrap Script**: Run `./scripts/bootstrap_admin.sh your@email.com`
3. **Admin API**: Create an admin endpoint to promote users (future feature)

All three methods work fine. `admin_email` is just the most convenient for initial setup.
