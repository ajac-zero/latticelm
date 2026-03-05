# latticelm

## Overview

A lightweight LLM proxy gateway written in Go that provides a unified API interface for multiple LLM providers. Similar to LiteLLM, but built natively in Go using each provider's official SDK.

## Purpose

Simplify LLM integration by exposing a single, consistent API that routes requests to different providers:
- **OpenAI** (GPT models)
- **Azure OpenAI** (Azure-deployed models)
- **Anthropic** (Claude)
- **Google Generative AI** (Gemini)
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

- **Single API interface** for multiple LLM providers
- **Native Go SDKs** for optimal performance and type safety
- **Provider abstraction** - switch providers without changing client code
- **Lightweight** - minimal overhead, fast routing
- **Easy configuration** - manage API keys and provider settings centrally

## Use Cases

- Applications that need multi-provider LLM support
- Cost optimization (route to cheapest provider for specific tasks)
- Failover and redundancy (fallback to alternative providers)
- A/B testing across different models
- Centralized LLM access for microservices

## 🎉 Status: **WORKING!**

✅ **All providers integrated with official Go SDKs:**
- OpenAI → `github.com/openai/openai-go/v3`
- Azure OpenAI → `github.com/openai/openai-go/v3` (with Azure auth)
- Anthropic → `github.com/anthropics/anthropic-sdk-go`
- Google → `google.golang.org/genai`
- Vertex AI → `google.golang.org/genai` (with GCP auth)

✅ **Compiles successfully** (36MB binary)
✅ **Provider auto-selection** (gpt→Azure/OpenAI, claude→Anthropic, gemini→Google)
✅ **Configuration system** (YAML with env var support)
✅ **Streaming support** (Server-Sent Events for all providers)
✅ **OAuth2/OIDC authentication** (Google, Auth0, any OIDC provider)
✅ **Terminal chat client** (Python with Rich UI, PEP 723)
✅ **Conversation tracking** (previous_response_id for efficient context)
✅ **Rate limiting** (Per-IP token bucket with configurable limits)
✅ **Health & readiness endpoints** (Kubernetes-compatible health checks)
✅ **Admin Web UI** (Dashboard with system info, health checks, provider status)

## Quick Start

```bash
# 1. Set API keys
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"
export GOOGLE_API_KEY="your-key"

# 2. Build (includes Admin UI)
cd latticelm
make build-all

# 3. Run
./bin/llm-gateway

# 4. Test (non-streaming)
curl -X POST http://localhost:8080/v1/chat/completions \
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

# 5. Test streaming
curl -X POST http://localhost:8080/v1/chat/completions \
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

## Tech Stack

- **Language:** Go
- **API Specification:** [Open Responses](https://www.openresponses.org)
- **SDKs:**
  - `google.golang.org/genai` (Google Generative AI)
  - Anthropic Go SDK
  - OpenAI Go SDK
- **Transport:** RESTful HTTP (potentially gRPC in the future)

## Status

🚧 **In Development** - Project specification and initial setup phase.

## Getting Started

1. **Copy the example config** and fill in provider API keys:

   ```bash
   cp config.example.yaml config.yaml
   ```

   You can also override API keys via environment variables (`GOOGLE_API_KEY`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`).

2. **Run the gateway** using the default configuration path:

   ```bash
   go run ./cmd/gateway --config config.yaml
   ```

   The server listens on the address configured under `server.address` (defaults to `:8080`).

3. **Call the Open Responses endpoint**:

   ```bash
   curl -X POST http://localhost:8080/v1/responses \
     -H 'Content-Type: application/json' \
     -d '{
           "model": "gpt-4o-mini",
           "input": [
             {"role": "user", "content": [{"type": "input_text", "text": "Hello!"}]}
           ]
         }'
   ```

   Include `"provider": "anthropic"` (or `google`, `openai`) to pin a provider; otherwise the gateway infers it from the model name.

## Project Structure

- `cmd/gateway`: Entry point that loads configuration, wires providers, and starts the HTTP server.
- `internal/config`: YAML configuration loader with environment overrides for API keys.
- `internal/api`: Open Responses request/response types and validation helpers.
- `internal/server`: HTTP handlers that expose `/v1/responses`.
- `internal/providers`: Provider abstractions plus provider-specific scaffolding in `google`, `anthropic`, and `openai` subpackages.

## Chat Client

Interactive terminal chat interface with beautiful Rich UI:

```bash
# Basic usage
uv run chat.py

# With authentication
uv run chat.py --token "$(gcloud auth print-identity-token)"

# Switch models on the fly
You> /model claude
You> /models  # List all available models
```

The chat client automatically uses `previous_response_id` to reduce token usage by only sending new messages instead of the full conversation history.

See **[CHAT_CLIENT.md](./CHAT_CLIENT.md)** for full documentation.

## Conversation Management

The gateway implements conversation tracking using `previous_response_id` from the Open Responses spec:

- 📉 **Reduced token usage** - Only send new messages
- ⚡ **Smaller requests** - Less bandwidth
- 🧠 **Server-side context** - Gateway maintains history
- ⏰ **Auto-expire** - Conversations expire after 1 hour

See **[CONVERSATIONS.md](./CONVERSATIONS.md)** for details.

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

The `provider_model_id` field lets you map a friendly model name to the actual provider identifier (e.g., an Azure deployment name). If omitted, the model `name` is used directly. See **[AZURE_OPENAI.md](./AZURE_OPENAI.md)** for complete setup guide.

## Admin Web UI

The gateway includes a built-in admin web interface for monitoring and configuration.

### Features

- **System Information** - View version, uptime, platform details
- **Health Checks** - Monitor server, providers, and conversation store status
- **Provider Status** - View configured providers and their models
- **Configuration** - View current configuration (with secrets masked)

### Accessing the Admin UI

1. Enable in config:
```yaml
admin:
  enabled: true
```

2. Build with frontend assets:
```bash
make build-all
```

3. Access at: `http://localhost:8080/admin/`

### Development Mode

Run backend and frontend separately for development:

```bash
# Terminal 1: Run backend
make dev-backend

# Terminal 2: Run frontend dev server
make dev-frontend
```

Frontend dev server runs on `http://localhost:5173` and proxies API requests to backend.

## Authentication

The gateway supports OAuth2/OIDC authentication. See **[AUTH.md](./AUTH.md)** for setup instructions.

**Quick example with Google OAuth:**

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

## Next Steps

- ✅ ~~Implement streaming responses~~
- ✅ ~~Add OAuth2/OIDC authentication~~
- ✅ ~~Implement conversation tracking with previous_response_id~~
- ⬜ Add structured logging, tracing, and request-level metrics
- ⬜ Support tool/function calling
- ⬜ Persistent conversation storage (Redis/database)
- ⬜ Expand configuration to support routing policies (cost, latency, failover)
