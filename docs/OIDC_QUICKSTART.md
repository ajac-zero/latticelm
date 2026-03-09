# OIDC Authentication Quick Start

This guide helps you quickly set up OIDC authentication for the gateway UI.

## Overview

With OIDC enabled:
- Users must log in via your identity provider (Google, Auth0, Okta, etc.)
- Sessions are managed with secure HTTP-only cookies
- `/chat` is accessible to all authenticated users
- `/` (dashboard) is restricted to admin users

## Quick Setup with Google

### 1. Create OAuth Client

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Navigate to **APIs & Services > Credentials**
3. Click **Create Credentials > OAuth 2.0 Client ID**
4. Application type: **Web application**
5. Add authorized redirect URI: `http://localhost:8080/auth/callback`
6. Save and copy your **Client ID** and **Client Secret**

### 2. Update Configuration

Edit `config.yaml`:

```yaml
auth:
  enabled: true
  issuer: "https://accounts.google.com"
  audience: "YOUR_CLIENT_ID.apps.googleusercontent.com"
  client_id: "YOUR_CLIENT_ID.apps.googleusercontent.com"
  client_secret: "YOUR_CLIENT_SECRET"
  redirect_uri: "http://localhost:8080/auth/callback"

ui:
  enabled: true
  # For admin access, add custom claim or use default (checks 'role', 'roles', 'groups')
  # claim: "role"
  # allowed_values:
  #   - "admin"
```

### 3. Run the Gateway

```bash
make build-all
./bin/llm-gateway --config config.yaml
```

### 4. Test It Out

1. Open browser to `http://localhost:8080/`
2. You'll be redirected to Google login
3. After authentication, you'll be redirected back to the gateway
4. Non-admin users see only the Chat page
5. Admin users can access both Chat and Dashboard

## Making a User Admin

By default, the gateway checks JWT claims for admin status. For Google, you need to add custom claims:

**Option 1: Use Google Cloud Identity Platform** (requires setup)

**Option 2: Use Auth0 or Okta** (easier for role management)

**Option 3: Development/Testing** - Temporarily allow all users:
```yaml
ui:
  claim: "email_verified"
  allowed_values:
    - "true"
```

## Auth Flow Diagram

```
┌──────┐                    ┌─────────┐                 ┌──────────┐
│      │   1. Visit /       │         │   2. Redirect   │          │
│ User ├───────────────────>│ Gateway ├────────────────>│ Identity │
│      │                    │         │   to IdP login  │ Provider │
└──────┘                    └─────────┘                 └──────────┘
   ^                             ^                            │
   │                             │                            │
   │ 7. Access protected         │ 4. Token exchange          │ 3. User
   │    pages with session       │    (Authorization Code     │    authenticates
   │                             │     Flow with PKCE)        │
   │                             │                            v
   │                        ┌─────────┐   5. Create      ┌──────────┐
   └────────────────────────┤         │<────session──────┤ /callback│
      6. Set session cookie │ Gateway │                  │          │
                            └─────────┘                  └──────────┘
```

## Troubleshooting

### "redirect_uri_mismatch" error
- Ensure the redirect URI in config matches **exactly** what's in your OAuth client settings
- Include the protocol (`http://` or `https://`)
- Match the port (`:8080`)

### Can't access dashboard after login
- Check that your user has admin role in JWT claims
- View your token at `/auth/user` endpoint
- Verify `ui.claim` and `allowed_values` configuration

### "Invalid state" error
- Clear browser cookies and try again
- Check server/browser clock synchronization
- Ensure cookies are enabled

### Session expires immediately
- Check `Secure` cookie flag if not using HTTPS
- Verify `SameSite` cookie settings
- Try accessing via `localhost` instead of `127.0.0.1`

## Production Deployment

For production use:

1. **Use HTTPS**: Cookies require `Secure` flag
   ```yaml
   redirect_uri: "https://your-domain.com/auth/callback"
   ```

2. **Configure CORS** (if frontend is separate domain)

3. **Set up role/claim management** in your identity provider

4. **Enable IP allowlist** for dashboard:
   ```yaml
   ui:
     ip_allowlist:
       - "10.0.0.0/8"  # VPN range
   ```

5. **Consider Redis-backed sessions** for multi-instance deployments (future enhancement)

## Next Steps

- Read [OIDC_AUTH.md](./OIDC_AUTH.md) for detailed architecture
- Configure other providers (Auth0, Okta) - see examples in OIDC_AUTH.md
- Set up custom claims for admin authorization
- Enable rate limiting and other production features

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /auth/login` | Initiate OIDC login |
| `GET /auth/callback` | OAuth callback handler |
| `GET /auth/logout` | Clear session |
| `GET /auth/user` | Get current user (JSON) |
| `GET /` | Dashboard (admin only) |
| `GET /chat` | Chat playground (all users) |
