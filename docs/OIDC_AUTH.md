# OIDC Authentication for UI

This document describes the OIDC authentication implementation for the gateway UI.

## Overview

The gateway UI now supports OIDC authentication with role-based access control (RBAC):

- **Chat page** (`/chat`) - Available to all authenticated users
- **Dashboard page** (`/`) - Available only to admin users

## Architecture

### Backend

The backend implements the OAuth 2.0 Authorization Code Flow with PKCE:

1. User visits UI → redirected to `/auth/login`
2. `/auth/login` generates state and PKCE verifier, redirects to OIDC provider
3. User authenticates with provider
4. Provider redirects to `/auth/callback` with authorization code
5. Backend exchanges code for tokens
6. Backend creates session and sets secure HTTP-only cookie
7. User is redirected to UI

**Components:**

- `internal/auth/oidc_client.go` - OIDC client implementation
- `internal/auth/session.go` - Session management
- Session middleware - Validates session cookies and redirects to login if invalid

### Frontend

The frontend uses Vue Router guards to protect routes:

- `ui/src/auth.ts` - Auth utilities (getCurrentUser, login, logout)
- `ui/src/router.ts` - Route guards that check authentication and admin status
- `ui/src/App.vue` - Navigation bar with user info and logout button

## Configuration

### 1. Enable OIDC in config.yaml

```yaml
auth:
  enabled: true
  issuer: "https://accounts.google.com"
  audience: "your-client-id.apps.googleusercontent.com"
  # OIDC client configuration for UI login
  client_id: "your-client-id.apps.googleusercontent.com"
  client_secret: "your-client-secret"  # Optional for public clients
  redirect_uri: "http://localhost:8080/auth/callback"

ui:
  enabled: true
  # Optional: specify claim for admin authorization
  # claim: "role"
  # allowed_values:
  #   - "admin"
```

### 2. OIDC Provider Setup

#### Google

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create/select a project
3. Navigate to "APIs & Services" > "Credentials"
4. Create OAuth 2.0 Client ID (Web application)
5. Add authorized redirect URI: `http://localhost:8080/auth/callback`
6. Copy Client ID and Client Secret

Configuration:
```yaml
auth:
  issuer: "https://accounts.google.com"
  audience: "your-client-id.apps.googleusercontent.com"
  client_id: "your-client-id.apps.googleusercontent.com"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/auth/callback"
```

To set admin role, add `role: admin` to JWT claims (requires custom claim configuration).

#### Auth0

1. Go to [Auth0 Dashboard](https://manage.auth0.com/)
2. Create an application (Regular Web Application)
3. Configure:
   - Allowed Callback URLs: `http://localhost:8080/auth/callback`
   - Allowed Logout URLs: `http://localhost:8080/auth/login`
4. Copy Domain, Client ID, and Client Secret

Configuration:
```yaml
auth:
  issuer: "https://your-tenant.auth0.com/"
  audience: "your-client-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/auth/callback"
```

Admin authorization in Auth0:
1. Create a "role" claim in user metadata
2. Add custom claim to ID token via Auth0 Rules/Actions
3. Configure gateway to check for admin role:

```yaml
ui:
  claim: "role"
  allowed_values:
    - "admin"
```

#### Okta

1. Go to [Okta Admin Console](https://your-domain.okta.com/admin)
2. Applications > Create App Integration > OIDC > Web Application
3. Configure:
   - Sign-in redirect URIs: `http://localhost:8080/auth/callback`
   - Sign-out redirect URIs: `http://localhost:8080/auth/login`
4. Copy Client ID and Client Secret

Configuration:
```yaml
auth:
  issuer: "https://your-domain.okta.com/"
  audience: "your-client-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  redirect_uri: "http://localhost:8080/auth/callback"
```

## Admin Authorization

The gateway checks JWT claims to determine if a user is an admin. By default, it looks for:

1. Custom claim (if specified in `ui.claim`)
2. `role` claim
3. `roles` claim (array)
4. `groups` claim (array)

If any of these contains "admin" (or values in `ui.allowed_values`), the user is granted admin access.

### Example JWT Claims

**Google (custom claim via GCP IAM):**
```json
{
  "email": "user@example.com",
  "role": "admin"
}
```

**Auth0:**
```json
{
  "email": "user@example.com",
  "https://your-app.com/roles": ["admin"]
}
```

**Okta:**
```json
{
  "email": "user@example.com",
  "groups": ["admin", "users"]
}
```

## API Endpoints

The following auth endpoints are available:

- `GET /auth/login` - Initiate OIDC login flow
- `GET /auth/callback` - OIDC callback handler
- `GET /auth/logout` - Logout and clear session
- `GET /auth/user` - Get current user info (JSON)

## Security Features

### Backend

- **PKCE (Proof Key for Code Exchange)** - Prevents authorization code interception
- **State parameter** - CSRF protection
- **Secure cookies** - HttpOnly, SameSite=Lax
- **Session expiration** - 24 hour default TTL
- **Automatic cleanup** - Expired sessions removed periodically

### Frontend

- **Route guards** - Prevent unauthorized access
- **Auth caching** - Reduces redundant API calls
- **Secure token storage** - Tokens stored server-side, not in localStorage

## Troubleshooting

### "Invalid state" error

- Ensure redirect_uri in config matches exactly what's registered with OIDC provider
- Check for clock skew between server and provider
- Verify state cookie is being set/read correctly (check SameSite, Secure attributes)

### "OIDC discovery failed"

- Verify issuer URL is correct and accessible
- Check firewall/network rules allow outbound HTTPS to provider
- Ensure well-known endpoint exists: `{issuer}/.well-known/openid-configuration`

### User not recognized as admin

- Check JWT claims in `/auth/user` response
- Verify claim name matches `ui.claim` configuration
- Ensure allowed_values includes the claim value
- For custom claims, verify they're added to ID token (not just access token)

### Session expires too quickly

- Sessions default to 24 hours
- Set `SESSION_TTL` environment variable to customize (e.g., `SESSION_TTL=48h`)
- Consider implementing refresh token rotation for longer sessions

### Multiple replicas / distributed deployment

- By default, sessions are stored in-memory (single instance only)
- For multi-instance deployments, configure Redis-backed sessions:
  ```bash
  SESSION_REDIS_URL=redis://redis:6379/2
  ```
- Redis-backed sessions allow all replicas to share session state
- Use a dedicated Redis database (different from rate limiting) to avoid key conflicts

## Migration from JWT Bearer Tokens

If you're currently using JWT Bearer token authentication for the API:

1. **API authentication** still uses JWT Bearer tokens (unchanged)
2. **UI authentication** now uses session cookies (OIDC flow)
3. Both can coexist:
   - API clients: Continue using `Authorization: Bearer <token>`
   - UI users: Use OIDC login flow

To force JWT auth for UI (disable OIDC):
- Don't set `client_id` in auth config
- Gateway falls back to JWT Bearer token validation

## Future Enhancements

Potential improvements:

- [x] Redis-backed sessions for multi-instance deployments
- [ ] Refresh token rotation
- [ ] Remember me (extended sessions)
- [ ] Session revocation API
- [ ] Audit logging for authentication events
- [ ] SSO integration with corporate identity providers
