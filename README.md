# latticelm

> A production-ready LLM proxy gateway written in Go with enterprise features

## Table of Contents

- [Overview](#overview)
- [Supported Providers](#supported-providers)
- [Key Features](#key-features)
- [Status](#status)
- [Use Cases](#use-cases)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [API Standard](#api-standard)
- [API Reference](#api-reference)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Chat Client](#chat-client)
- [Conversation Management](#conversation-management)
- [Observability](#observability)
- [Circuit Breakers](#circuit-breakers)
- [Azure OpenAI](#azure-openai)
- [Azure Anthropic](#azure-anthropic-microsoft-foundry)
- [Admin Web UI](#admin-web-ui)
- [Deployment](#deployment)
- [Authentication](#authentication)
- [Production Features](#production-features)
- [Roadmap](#roadmap)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Overview

A production-ready LLM proxy gateway written in Go that provides a unified API interface for multiple LLM providers. Similar to LiteLLM, but built natively in Go using each provider's official SDK with enterprise features including rate limiting, circuit breakers, observability, and authentication.

## Supported Providers

- **OpenAI** (GPT models)
- **Azure OpenAI** (Azure-deployed OpenAI models)
- **Anthropic** (Claude models)
- **Azure Anthropic** (Microsoft Foundry-hosted Claude models)
- **Google Generative AI** (Gemini models)
- **Vertex AI** (Google Cloud-hosted Gemini models)

Instead of managing multiple SDK integrations in your application, call one endpoint and let the gateway handle provider-specific implementations.

## Architecture

```
Client Request
    ↓
latticelm (unified API)
    ↓
├─→ OpenAI SDK
├─→ Azure OpenAI (OpenAI SDK + Azure auth)
├─→ Anthropic SDK
├─→ Google Gen AI SDK
└─→ Vertex AI (Google Gen AI SDK + GCP auth)
```

## Key Features

### Core Functionality
- **Single API interface** for multiple LLM providers
- **Native Go SDKs** for optimal performance and type safety
- **Provider abstraction** - switch providers without changing client code
- **Streaming support** - Server-Sent Events for all providers
- **Conversation tracking** - Efficient context management with `previous_response_id`

### Production Features
- **Circuit breakers** - Automatic failure detection and recovery per provider
- **Rate limiting** - Per-IP token bucket algorithm with configurable limits
- **OAuth2/OIDC authentication** - Support for Google, Auth0, and any OIDC provider
- **Observability** - Prometheus metrics and OpenTelemetry tracing
- **Health checks** - Kubernetes-compatible liveness and readiness endpoints
- **Admin Web UI** - Built-in dashboard for monitoring and configuration

### Configuration
- **Easy setup** - YAML configuration with environment variable overrides
- **Flexible storage** - In-memory, SQLite, MySQL, PostgreSQL, or Redis for conversations

## Use Cases

- Applications that need multi-provider LLM support
- Cost optimization (route to cheapest provider for specific tasks)
- Failover and redundancy (fallback to alternative providers)
- A/B testing across different models
- Centralized LLM access for microservices

## Status

**Production Ready** - All core features implemented and tested.

### Provider Integration
✅ All providers use official Go SDKs:
- OpenAI → `github.com/openai/openai-go/v3`
- Azure OpenAI → `github.com/openai/openai-go/v3` (with Azure auth)
- Anthropic → `github.com/anthropics/anthropic-sdk-go`
- Azure Anthropic → `github.com/anthropics/anthropic-sdk-go` (with Azure auth)
- Google Gen AI → `google.golang.org/genai`
- Vertex AI → `google.golang.org/genai` (with GCP auth)

### Features
✅ Provider auto-selection (gpt→OpenAI, claude→Anthropic, gemini→Google)
✅ Streaming responses (Server-Sent Events)
✅ Conversation tracking with `previous_response_id`
✅ OAuth2/OIDC authentication
✅ Rate limiting with token bucket algorithm
✅ Circuit breakers for fault tolerance
✅ Observability (Prometheus metrics + OpenTelemetry tracing)
✅ Health & readiness endpoints
✅ Admin Web UI dashboard
✅ Terminal chat client (Python with Rich UI)

## Quick Start

### Prerequisites

- Go 1.21+ (for building from source)
- Docker (optional, for containerized deployment)
- Node.js 18+ (optional, for Web UI development)

### Running Locally

```bash
# 1. Clone the repository
git clone https://github.com/yourusername/latticelm.git
cd latticelm

# 2. Set API keys
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"
export GOOGLE_API_KEY="your-key"

# 3. Copy and configure settings (optional)
cp config.example.yaml config.yaml
# Edit config.yaml to customize settings

# 4. Build (includes Web UI)
make build-all

# 5. Run
./bin/llm-gateway

# Gateway starts on http://localhost:8080
# Web UI available at http://localhost:8080/
```

### Testing the API

**Non-streaming request:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": [
      {
        "role": "user",
        "content": [{"type": "input_text", "text": "Hello!"}]
      }
    ]
  }'
```

**Streaming request:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -N \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "stream": true,
    "input": [
      {
        "role": "user",
        "content": [{"type": "input_text", "text": "Write a haiku about Go"}]
      }
    ]
  }'
```

### Development Mode

Run backend and frontend separately for live reloading:

```bash
# Terminal 1: Backend with auto-reload
make dev-backend

# Terminal 2: Frontend dev server
make dev-frontend
```

Frontend runs on `http://localhost:5173` with hot module replacement.

## API Standard

This gateway implements the **[Open Responses](https://www.openresponses.org)** specification — an open-source, multi-provider API standard for LLM interfaces based on OpenAI's Responses API.

**Why Open Responses:**
- **Multi-provider by default** - one schema that maps cleanly across providers
- **Agentic workflow support** - consistent streaming events, tool invocation patterns, and "items" as atomic units
- **Extensible** - stable core with room for provider-specific features

By following the Open Responses spec, this gateway ensures:
- Interoperability across different LLM providers
- Standard request/response formats (messages, tool calls, streaming)
- Compatibility with existing Open Responses tooling and ecosystem

For full specification details, see: **https://www.openresponses.org**

## API Reference

### Core Endpoints

#### POST /v1/responses
Create a chat completion response (streaming or non-streaming).

**Request body:**
```json
{
  "model": "gpt-4o-mini",
  "stream": false,
  "input": [
    {
      "role": "user",
      "content": [{"type": "input_text", "text": "Hello!"}]
    }
  ],
  "previous_response_id": "optional-conversation-id",
  "provider": "optional-explicit-provider"
}
```

**Response (non-streaming):**
```json
{
  "id": "resp_abc123",
  "object": "response",
  "model": "gpt-4o-mini",
  "provider": "openai",
  "output": [
    {
      "role": "assistant",
      "content": [{"type": "text", "text": "Hello! How can I help you?"}]
    }
  ],
  "usage": {
    "input_tokens": 10,
    "output_tokens": 8
  }
}
```

**Response (streaming):**
Server-Sent Events with `data: {...}` lines containing deltas.

#### GET /v1/models
List available models.

**Response:**
```json
{
  "object": "list",
  "data": [
    {"id": "gpt-4o-mini", "provider": "openai"},
    {"id": "claude-3-5-sonnet", "provider": "anthropic"},
    {"id": "gemini-1.5-flash", "provider": "google"}
  ]
}
```

### Health Endpoints

#### GET /health
Liveness probe (always returns 200 if server is running).

**Response:**
```json
{
  "status": "healthy",
  "timestamp": 1709438400
}
```

#### GET /ready
Readiness probe (checks conversation store and providers).

**Response:**
```json
{
  "status": "ready",
  "timestamp": 1709438400,
  "checks": {
    "conversation_store": "healthy",
    "providers": "healthy"
  }
}
```

Returns 503 if any check fails.

### Admin Endpoints

#### GET /
Web dashboard (when web UI is enabled).

#### GET /api/info
System information.

#### GET /api/health
Detailed health status.

#### GET /api/config
Current configuration (secrets masked).

### Observability Endpoints

#### GET /metrics
Prometheus metrics (when observability is enabled).

## Tech Stack

- **Language:** Go
- **API Specification:** [Open Responses](https://www.openresponses.org)
- **Official SDKs:**
  - `google.golang.org/genai` (Google Generative AI & Vertex AI)
  - `github.com/anthropics/anthropic-sdk-go` (Anthropic & Azure Anthropic)
  - `github.com/openai/openai-go/v3` (OpenAI & Azure OpenAI)
- **Observability:**
  - Prometheus for metrics
  - OpenTelemetry for distributed tracing
- **Resilience:**
  - Circuit breakers via `github.com/sony/gobreaker`
  - Token bucket rate limiting
- **Transport:** RESTful HTTP with Server-Sent Events for streaming

## Project Structure

```
latticelm/
├── cmd/gateway/          # Main application entry point
├── internal/
│   ├── admin/            # Web UI backend and embedded frontend
│   ├── api/              # Open Responses types and validation
│   ├── auth/             # OAuth2/OIDC authentication
│   ├── config/           # YAML configuration loader
│   ├── conversation/     # Conversation tracking and storage
│   ├── logger/           # Structured logging setup
│   ├── metrics/          # Prometheus metrics
│   ├── providers/        # Provider implementations
│   │   ├── anthropic/
│   │   ├── azureanthropic/
│   │   ├── azureopenai/
│   │   ├── google/
│   │   ├── openai/
│   │   └── vertexai/
│   ├── ratelimit/        # Rate limiting implementation
│   ├── server/           # HTTP server and handlers
│   └── tracing/          # OpenTelemetry tracing
├── ui/                   # Vue.js Web UI
├── k8s/                  # Kubernetes manifests
├── tests/                # Integration tests
├── config.example.yaml   # Example configuration
├── Makefile              # Build and development tasks
└── README.md
```

## Configuration

The gateway uses a YAML configuration file with support for environment variable overrides.

### Basic Configuration

```yaml
server:
  address: ":8080"
  max_request_body_size: 10485760  # 10MB

logging:
  format: "json"  # or "text" for development
  level: "info"   # debug, info, warn, error

# Configure providers (API keys can use ${ENV_VAR} syntax)
providers:
  openai:
    type: "openai"
    api_key: "${OPENAI_API_KEY}"
  anthropic:
    type: "anthropic"
    api_key: "${ANTHROPIC_API_KEY}"
  google:
    type: "google"
    api_key: "${GOOGLE_API_KEY}"

# Map model names to providers
models:
  - name: "gpt-4o-mini"
    provider: "openai"
  - name: "claude-3-5-sonnet"
    provider: "anthropic"
  - name: "gemini-1.5-flash"
    provider: "google"
```

### Advanced Configuration

```yaml
# Rate limiting
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 20

# Authentication
auth:
  enabled: true
  issuer: "https://accounts.google.com"
  audience: "your-client-id.apps.googleusercontent.com"

# Observability
observability:
  enabled: true
  metrics:
    enabled: true
    path: "/metrics"
  tracing:
    enabled: true
    service_name: "llm-gateway"
    exporter:
      type: "otlp"
      endpoint: "localhost:4317"

# Conversation storage
conversations:
  store: "sql"  # memory, sql, or redis
  ttl: "1h"
  driver: "sqlite3"
  dsn: "conversations.db"

# Web UI
ui:
  enabled: true
```

See `config.example.yaml` for complete configuration options with detailed comments.

## Chat Client

Interactive terminal chat interface with beautiful Rich UI powered by Python and the Rich library:

```bash
# Basic usage
uv run chat.py

# With authentication
uv run chat.py --token "$(gcloud auth print-identity-token)"

# Switch models on the fly
You> /model claude
You> /models  # List all available models
```

Features:
- **Syntax highlighting** for code blocks
- **Markdown rendering** for formatted responses
- **Model switching** on the fly with `/model` command
- **Conversation history** with automatic `previous_response_id` tracking
- **Streaming responses** with real-time display

The chat client uses [PEP 723](https://peps.python.org/pep-0723/) inline script metadata, so `uv run` automatically installs dependencies.

## Conversation Management

The gateway implements efficient conversation tracking using `previous_response_id` from the Open Responses spec:

- 📉 **Reduced token usage** - Only send new messages, not full history
- ⚡ **Smaller requests** - Less bandwidth and faster responses
- 🧠 **Server-side context** - Gateway maintains conversation state
- ⏰ **Auto-expire** - Conversations expire after configurable TTL (default: 1 hour)

### Storage Options

Choose from multiple storage backends:

```yaml
conversations:
  store: "memory"  # "memory", "sql", or "redis"
  ttl: "1h"        # Conversation expiration

  # SQLite (default for sql)
  driver: "sqlite3"
  dsn: "conversations.db"

  # MySQL
  # driver: "mysql"
  # dsn: "user:password@tcp(localhost:3306)/dbname?parseTime=true"

  # PostgreSQL
  # driver: "pgx"
  # dsn: "postgres://user:password@localhost:5432/dbname?sslmode=disable"

  # Redis
  # store: "redis"
  # dsn: "redis://:password@localhost:6379/0"
```

## Observability

The gateway provides comprehensive observability through Prometheus metrics and OpenTelemetry tracing.

### Metrics

Enable Prometheus metrics to monitor gateway performance:

```yaml
observability:
  enabled: true
  metrics:
    enabled: true
    path: "/metrics"  # Default endpoint
```

Available metrics include:
- Request counts and latencies per provider and model
- Error rates and types
- Circuit breaker state changes
- Rate limit hits
- Conversation store operations

Access metrics at `http://localhost:8080/metrics` (Prometheus scrape format).

### Tracing

Enable OpenTelemetry tracing for distributed request tracking:

```yaml
observability:
  enabled: true
  tracing:
    enabled: true
    service_name: "llm-gateway"
    sampler:
      type: "probability"  # "always", "never", or "probability"
      rate: 0.1  # Sample 10% of requests
    exporter:
      type: "otlp"  # Send to OpenTelemetry Collector
      endpoint: "localhost:4317"  # gRPC endpoint
      insecure: true  # Use TLS in production
```

Traces include:
- End-to-end request flow
- Provider API calls
- Conversation store lookups
- Circuit breaker operations
- Authentication checks

Use with Jaeger, Zipkin, or any OpenTelemetry-compatible backend.

## Circuit Breakers

The gateway automatically wraps each provider with a circuit breaker for fault tolerance. When a provider experiences failures, the circuit breaker:

1. **Closed state** - Normal operation, requests pass through
2. **Open state** - Fast-fail after threshold reached, returns errors immediately
3. **Half-open state** - Allows test requests to check if provider recovered

Default configuration (per provider):
- **Max requests in half-open**: 3
- **Interval**: 60 seconds (resets failure count)
- **Timeout**: 30 seconds (open → half-open transition)
- **Failure ratio**: 0.5 (50% failures trips circuit)

Circuit breaker state changes are logged and exposed via metrics.

## Azure OpenAI

The gateway supports Azure OpenAI with the same interface as standard OpenAI:

```yaml
providers:
  azureopenai:
    type: "azureopenai"
    api_key: "${AZURE_OPENAI_API_KEY}"
    endpoint: "https://your-resource.openai.azure.com"

models:
  - name: "gpt-4o"
    provider: "azureopenai"
    provider_model_id: "my-gpt4o-deployment"  # optional: defaults to name
```

```bash
export AZURE_OPENAI_API_KEY="..."
export AZURE_OPENAI_ENDPOINT="https://your-resource.openai.azure.com"

./gateway
```

The `provider_model_id` field lets you map a friendly model name to the actual provider identifier (e.g., an Azure deployment name). If omitted, the model `name` is used directly.

## Azure Anthropic (Microsoft Foundry)

The gateway supports Azure-hosted Anthropic models through Microsoft's AI Foundry:

```yaml
providers:
  azureanthropic:
    type: "azureanthropic"
    api_key: "${AZURE_ANTHROPIC_API_KEY}"
    endpoint: "https://your-resource.services.ai.azure.com/anthropic"

models:
  - name: "claude-sonnet-4-5"
    provider: "azureanthropic"
    provider_model_id: "claude-sonnet-4-5-20250514"  # optional
```

```bash
export AZURE_ANTHROPIC_API_KEY="..."
export AZURE_ANTHROPIC_ENDPOINT="https://your-resource.services.ai.azure.com/anthropic"

./gateway
```

Azure Anthropic provides Claude models with Azure's compliance, security, and regional deployment options.

## Admin Web UI

The gateway includes a built-in admin web interface for monitoring and configuration.

### Features

- **System Information** - View version, uptime, platform details
- **Health Checks** - Monitor server, providers, and conversation store status
- **Provider Status** - View configured providers and their models
- **Configuration** - View current configuration (with secrets masked)

### Accessing the Web UI

1. Enable in config:
```yaml
ui:
  enabled: true
```

2. Build with frontend assets:
```bash
make build-all
```

3. Access at: `http://localhost:8080/`

### Development Mode

Run backend and frontend separately for development:

```bash
# Terminal 1: Run backend
make dev-backend

# Terminal 2: Run frontend dev server
make dev-frontend
```

Frontend dev server runs on `http://localhost:5173` and proxies API requests to backend.

## Deployment

### Docker

**See the [Docker Deployment Guide](./docs/DOCKER_DEPLOYMENT.md)** for complete instructions on using pre-built images.

Build and run with Docker:

```bash
# Build Docker image (includes Web UI automatically)
docker build -t llm-gateway:latest .

# Run container
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -e GOOGLE_API_KEY="your-key" \
  -e ANTHROPIC_API_KEY="your-key" \
  -e OPENAI_API_KEY="your-key" \
  llm-gateway:latest

# Check status
docker logs llm-gateway
```

The Docker build uses a multi-stage process that automatically builds the frontend, so you don't need Node.js installed locally.

**Using Docker Compose:**

```yaml
version: '3.8'
services:
  llm-gateway:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - GOOGLE_API_KEY=${GOOGLE_API_KEY}
    restart: unless-stopped
```

```bash
docker-compose up -d
```

The Docker image:
- Uses 3-stage build (frontend → backend → runtime) for minimal size (~50MB)
- Automatically builds and embeds the Web UI
- Runs as non-root user (UID 1000) for security
- Includes health checks for orchestration
- No need for Node.js or Go installed locally

### Kubernetes

Production-ready Kubernetes manifests are available in the `k8s/` directory:

```bash
# Deploy to Kubernetes
kubectl apply -k k8s/

# Or deploy individual manifests
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml
```

Features included:
- **High availability** - 3+ replicas with pod anti-affinity
- **Auto-scaling** - HorizontalPodAutoscaler (3-20 replicas)
- **Security** - Non-root, read-only filesystem, network policies
- **Monitoring** - ServiceMonitor and PrometheusRule for Prometheus Operator
- **Storage** - Redis StatefulSet for conversation persistence
- **Ingress** - TLS with cert-manager integration

See **[k8s/README.md](./k8s/README.md)** for complete deployment guide including:
- Cloud-specific configurations (AWS EKS, GCP GKE, Azure AKS)
- Secrets management (External Secrets Operator, Sealed Secrets)
- Monitoring and alerting setup
- Troubleshooting guide

## Authentication

The gateway supports OAuth2/OIDC authentication for securing API access.

### Configuration

```yaml
auth:
  enabled: true
  issuer: "https://accounts.google.com"
  audience: "YOUR-CLIENT-ID.apps.googleusercontent.com"
```

```bash
# Get token
TOKEN=$(gcloud auth print-identity-token)

# Make authenticated request
curl -X POST http://localhost:8080/v1/responses \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-2.0-flash-exp", ...}'
```

## Production Features

### Rate Limiting

Per-IP rate limiting using token bucket algorithm to prevent abuse and manage load:

```yaml
rate_limit:
  enabled: true
  requests_per_second: 10  # Max requests per second per IP
  burst: 20                # Maximum burst size
```

Features:
- **Token bucket algorithm** for smooth rate limiting
- **Per-IP limiting** with support for X-Forwarded-For headers
- **Configurable limits** for requests per second and burst size
- **Automatic cleanup** of stale rate limiters to prevent memory leaks
- **429 responses** with Retry-After header when limits exceeded

### Health & Readiness Endpoints

Kubernetes-compatible health check endpoints for orchestration and load balancers:

**Liveness endpoint** (`/health`):
```bash
curl http://localhost:8080/health
# {"status":"healthy","timestamp":1709438400}
```

**Readiness endpoint** (`/ready`):
```bash
curl http://localhost:8080/ready
# {
#   "status":"ready",
#   "timestamp":1709438400,
#   "checks":{
#     "conversation_store":"healthy",
#     "providers":"healthy"
#   }
# }
```

The readiness endpoint verifies:
- Conversation store connectivity
- At least one provider is configured
- Returns 503 if any check fails

## Roadmap

### Completed ✅
- ✅ Streaming responses (Server-Sent Events)
- ✅ OAuth2/OIDC authentication
- ✅ Conversation tracking with `previous_response_id`
- ✅ Persistent conversation storage (SQL and Redis)
- ✅ Circuit breakers for fault tolerance
- ✅ Rate limiting
- ✅ Observability (Prometheus metrics and OpenTelemetry tracing)
- ✅ Admin Web UI
- ✅ Health and readiness endpoints

### In Progress 🚧
- ⬜ Tool/function calling support across providers
- ⬜ Request-level cost tracking and budgets
- ⬜ Advanced routing policies (cost optimization, latency-based, failover)
- ⬜ Multi-tenancy with per-tenant rate limits and quotas
- ⬜ Request caching for identical prompts
- ⬜ Webhook notifications for events (failures, circuit breaker changes)

## Documentation

Comprehensive guides and documentation are available in the `/docs` directory:

- **[Docker Deployment Guide](./docs/DOCKER_DEPLOYMENT.md)** - Deploy with pre-built images or build from source
- **[Kubernetes Deployment Guide](./k8s/README.md)** - Production deployment with Kubernetes
- **[Web UI Documentation](./docs/ADMIN_UI.md)** - Using the web dashboard
- **[Configuration Reference](./config.example.yaml)** - All configuration options explained

See the **[docs directory README](./docs/README.md)** for a complete documentation index.

## Contributing

Contributions are welcome! Here's how you can help:

### Reporting Issues

- **Bug reports**: Include steps to reproduce, expected vs actual behavior, and environment details
- **Feature requests**: Describe the use case and why it would be valuable
- **Security issues**: Email security concerns privately (don't open public issues)

### Development Workflow

1. **Fork and clone** the repository
2. **Create a branch** for your feature: `git checkout -b feature/your-feature-name`
3. **Make your changes** with clear, atomic commits
4. **Add tests** for new functionality
5. **Run tests**: `make test`
6. **Run linter**: `make lint`
7. **Update documentation** if needed
8. **Submit a pull request** with a clear description

### Code Standards

- Follow Go best practices and idioms
- Write tests for new features and bug fixes
- Keep functions small and focused
- Use meaningful variable names
- Add comments for complex logic
- Run `go fmt` before committing

### Testing

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/providers/...

# Run with coverage
make test-coverage

# Run integration tests (requires API keys)
make test-integration
```

### Adding a New Provider

1. Create provider implementation in `internal/providers/yourprovider/`
2. Implement the `Provider` interface
3. Add provider registration in `internal/providers/providers.go`
4. Add configuration support in `internal/config/`
5. Add tests and update documentation

## License

MIT License - see the repository for details.

## Acknowledgments

- Built with official SDKs from OpenAI, Anthropic, and Google
- Inspired by [LiteLLM](https://github.com/BerriAI/litellm)
- Implements the [Open Responses](https://www.openresponses.org) specification
- Uses [gobreaker](https://github.com/sony/gobreaker) for circuit breaker functionality

## Support

- **Documentation**: Check this README and the files in `/docs`
- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions and community support

---

**Made with ❤️ in Go**
