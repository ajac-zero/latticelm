# Admin Web UI

The LLM Gateway includes a built-in admin web interface for monitoring and managing the gateway.

## Features

### System Information
- Version and build details
- Platform information (OS, architecture)
- Go version
- Server uptime
- Git commit hash

### Health Status
- Overall system health
- Individual health checks:
  - Server status
  - Provider availability
  - Conversation store connectivity

### Provider Management
- View all configured providers
- See provider types (OpenAI, Anthropic, Google, etc.)
- List models available for each provider
- Monitor provider status

### Configuration Viewing
- View current gateway configuration
- Secrets are automatically masked for security
- Collapsible JSON view
- Shows all config sections:
  - Server settings
  - Providers
  - Models
  - Authentication
  - Conversations
  - Logging
  - Rate limiting
  - Observability

## Setup

### Production Build

1. **Enable admin UI in config:**
```yaml
admin:
  enabled: true
```

2. **Build frontend and backend together:**
```bash
make build-all
```

This command:
- Builds the Vue 3 frontend
- Copies frontend assets to `internal/ui/dist`
- Embeds assets into the Go binary using `embed.FS`
- Compiles the gateway with embedded admin UI

3. **Run the gateway:**
```bash
./bin/llm-gateway --config config.yaml
```

4. **Access the admin UI:**
Navigate to `http://localhost:8080/admin/`

### Development Mode

For faster frontend development with hot reload:

**Terminal 1 - Backend:**
```bash
make dev-backend
# or
go run ./cmd/gateway --config config.yaml
```

**Terminal 2 - Frontend:**
```bash
make dev-frontend
# or
cd ui && npm run dev
```

The frontend dev server runs on `http://localhost:5173` and automatically proxies API requests to the backend on `http://localhost:8080`.

## Architecture

### Backend Components

**Package:** `internal/ui/`

- `server.go` - Server struct and initialization
- `handlers.go` - API endpoint handlers
- `routes.go` - Route registration
- `response.go` - JSON response helpers
- `static.go` - Embedded frontend asset serving

### API Endpoints

All admin API endpoints are under `/admin/api/v1/`:

- `GET /admin/api/v1/system/info` - System information
- `GET /admin/api/v1/system/health` - Health checks
- `GET /admin/api/v1/config` - Configuration (secrets masked)
- `GET /admin/api/v1/providers` - Provider list and status

### Frontend Components

**Framework:** Vue 3 + TypeScript + Vite

**Directory:** `ui/`

```
ui/
├── src/
│   ├── main.ts              # App entry point
│   ├── App.vue              # Root component
│   ├── router.ts            # Vue Router config
│   ├── api/
│   │   ├── client.ts        # Axios HTTP client
│   │   ├── system.ts        # System API calls
│   │   ├── config.ts        # Config API calls
│   │   └── providers.ts     # Providers API calls
│   ├── components/          # Reusable components
│   ├── views/
│   │   └── Dashboard.vue    # Main dashboard view
│   └── types/
│       └── api.ts           # TypeScript type definitions
├── index.html
├── package.json
├── vite.config.ts
└── tsconfig.json
```

## Security Features

### Secret Masking

All sensitive data is automatically masked in API responses:

- API keys show only first 4 and last 4 characters
- Database connection strings are partially hidden
- OAuth secrets are masked

Example:
```json
{
  "api_key": "sk-p...xyz"
}
```

### Authentication

In MVP version, the admin UI inherits the gateway's existing authentication:

- If `auth.enabled: true`, admin UI requires valid JWT token
- If `auth.enabled: false`, admin UI is publicly accessible

**Note:** Production deployments should always enable authentication.

## Auto-Refresh

The dashboard automatically refreshes data every 30 seconds to keep information current.

## Browser Support

The admin UI works in all modern browsers:
- Chrome/Edge (recommended)
- Firefox
- Safari

## Build Process

### Frontend Build

```bash
cd ui
npm install
npm run build
```

Output: `ui/dist/`

### Embedding in Go Binary

The `internal/ui/static.go` file uses Go's `embed` directive:

```go
//go:embed all:dist
var frontendAssets embed.FS
```

This embeds all files from the `dist` directory into the compiled binary, creating a single-file deployment artifact.

### SPA Routing

The admin UI is a Single Page Application (SPA). The static file server implements fallback to `index.html` for client-side routing, allowing Vue Router to handle navigation.

## Troubleshooting

### Admin UI shows 404

- Ensure `admin.enabled: true` in config
- Rebuild with `make build-all` to embed frontend assets
- Check that `internal/ui/dist/` exists and contains built assets

### API calls fail

- Check that backend is running on port 8080
- Verify CORS is not blocking requests (should not be an issue as UI is served from same origin)
- Check browser console for errors

### Frontend won't build

- Ensure Node.js 18+ is installed: `node --version`
- Install dependencies: `cd ui && npm install`
- Check for npm errors in build output

### Assets not loading

- Verify Vite config has correct `base: '/admin/'`
- Check that asset paths in `index.html` are correct
- Ensure Go's embed is finding the dist folder

## Future Enhancements

Planned features for future releases:

- [ ] RBAC with admin/viewer roles
- [ ] Audit logging for all admin actions
- [ ] Configuration editing (hot reload)
- [ ] Provider management (add/edit/delete)
- [ ] Model management
- [ ] Circuit breaker reset controls
- [ ] Real-time metrics and charts
- [ ] Request/response inspection
- [ ] Rate limit management
