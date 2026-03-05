# Admin Web UI Specification

**Project:** go-llm-gateway (latticelm)
**Feature:** Admin Web UI
**Version:** 1.0
**Status:** Draft
**Date:** 2026-03-05

---

## Table of Contents

1. [Overview](#overview)
2. [Goals and Objectives](#goals-and-objectives)
3. [Requirements](#requirements)
4. [Architecture](#architecture)
5. [API Specification](#api-specification)
6. [UI Design](#ui-design)
7. [Security](#security)
8. [Implementation Phases](#implementation-phases)
9. [Testing Strategy](#testing-strategy)
10. [Deployment](#deployment)
11. [Future Enhancements](#future-enhancements)

---

## Overview

The Admin Web UI provides a browser-based interface for managing and monitoring the go-llm-gateway service. It enables operators to configure providers, manage models, monitor system health, and perform administrative tasks without directly editing configuration files or using CLI tools.

### Problem Statement

Currently, configuring and operating go-llm-gateway requires:
- Manual editing of `config.yaml` files
- Restarting the service for configuration changes
- Using external tools (Grafana, Prometheus) for monitoring
- Command-line access for operational tasks
- No centralized view of system health and configuration

### Solution

A web-based administration interface that provides:
- Real-time system status and metrics visualization
- Configuration management with validation
- Provider and model management
- Conversation store administration
- Integrated monitoring and diagnostics

---

## Goals and Objectives

### Primary Goals

1. **Simplify Configuration Management**
   - Reduce time to configure providers from minutes to seconds
   - Eliminate configuration syntax errors through UI validation
   - Provide immediate feedback on configuration changes

2. **Improve Operational Visibility**
   - Centralized dashboard for system health
   - Real-time metrics and performance monitoring
   - Provider connection status and circuit breaker states

3. **Enhance Developer Experience**
   - Intuitive interface requiring no YAML knowledge
   - Self-documenting configuration options
   - Quick testing of provider configurations

### Non-Goals

- **Not a replacement for Grafana/Prometheus** - Focus on operational tasks, not deep metrics analysis
- **Not a user-facing API explorer** - Admin-only, not for end users of the gateway
- **Not a conversation UI** - Management only, not for interactive LLM chat
- **Not a multi-tenancy admin** - Single instance management only

---

## Requirements

### Functional Requirements

#### FR1: Dashboard and Overview
- **FR1.1**: Display system status (uptime, version, build info)
- **FR1.2**: Show current configuration summary
- **FR1.3**: Display provider health status with circuit breaker states
- **FR1.4**: Show key metrics (requests/sec, error rate, latency percentiles)
- **FR1.5**: Display recent logs/events (last 100 entries)

#### FR2: Provider Management
- **FR2.1**: List all configured providers with status indicators
- **FR2.2**: Add new provider configurations (OpenAI, Azure, Anthropic, Google, Vertex AI)
- **FR2.3**: Edit existing provider settings (API keys, endpoints, parameters)
- **FR2.4**: Delete provider configurations with confirmation
- **FR2.5**: Test provider connectivity with sample request
- **FR2.6**: View provider-specific metrics (request count, error rate, latency)
- **FR2.7**: Reset circuit breaker state for providers

#### FR3: Model Management
- **FR3.1**: List all configured model mappings
- **FR3.2**: Add new model mappings (name → provider + model ID)
- **FR3.3**: Edit model mappings
- **FR3.4**: Delete model mappings with confirmation
- **FR3.5**: View model usage statistics (request count per model)
- **FR3.6**: Test model availability with sample request

#### FR4: Configuration Management
- **FR4.1**: View current configuration (all sections)
- **FR4.2**: Edit server settings (address, body size limits)
- **FR4.3**: Edit logging configuration (format, level)
- **FR4.4**: Edit rate limiting settings (enabled, requests/sec, burst)
- **FR4.5**: Edit authentication settings (OIDC issuer, audience)
- **FR4.6**: Edit observability settings (metrics, tracing)
- **FR4.7**: Validate configuration before applying
- **FR4.8**: Export current configuration as YAML
- **FR4.9**: Preview configuration diff before applying changes
- **FR4.10**: Apply configuration with hot-reload or restart prompt

#### FR5: Conversation Store Management
- **FR5.1**: View conversation store type and connection status
- **FR5.2**: Browse conversations (paginated list)
- **FR5.3**: Search conversations by ID or metadata
- **FR5.4**: View conversation details (messages, metadata, timestamps)
- **FR5.5**: Delete individual conversations
- **FR5.6**: Bulk delete conversations (by age, by criteria)
- **FR5.7**: View conversation statistics (total count, storage size)

#### FR6: Monitoring and Metrics
- **FR6.1**: Display request rate (current, 1m, 5m, 15m averages)
- **FR6.2**: Display error rate by provider and model
- **FR6.3**: Display latency percentiles (p50, p90, p95, p99)
- **FR6.4**: Display provider-specific metrics
- **FR6.5**: Display circuit breaker state changes (timeline)
- **FR6.6**: Export metrics in Prometheus format

#### FR7: Logs and Diagnostics
- **FR7.1**: View recent application logs (tail -f style)
- **FR7.2**: Filter logs by level (debug, info, warn, error)
- **FR7.3**: Search logs by keyword
- **FR7.4**: Download log exports
- **FR7.5**: View OpenTelemetry trace samples (if enabled)

#### FR8: System Operations
- **FR8.1**: View health check status (/health, /ready)
- **FR8.2**: Trigger graceful restart (with countdown)
- **FR8.3**: View environment variables (sanitized, no secrets)
- **FR8.4**: Download diagnostic bundle (config + logs + metrics)

### Non-Functional Requirements

#### NFR1: Performance
- **NFR1.1**: Admin UI must not impact gateway performance (< 1% CPU overhead)
- **NFR1.2**: Dashboard load time < 2 seconds on modern browsers
- **NFR1.3**: API endpoints respond within 500ms (p95)
- **NFR1.4**: Support concurrent admin users (up to 10)

#### NFR2: Security
- **NFR2.1**: All admin endpoints require authentication
- **NFR2.2**: Support OIDC/OAuth2 authentication (reuse existing auth)
- **NFR2.3**: Support role-based access control (admin vs viewer roles)
- **NFR2.4**: Sanitize secrets in all UI displays (mask API keys)
- **NFR2.5**: Audit log for all configuration changes
- **NFR2.6**: CSRF protection for state-changing operations
- **NFR2.7**: Content Security Policy (CSP) headers

#### NFR3: Usability
- **NFR3.1**: Responsive design (desktop, tablet, mobile)
- **NFR3.2**: Accessible (WCAG 2.1 Level AA)
- **NFR3.3**: Dark mode support
- **NFR3.4**: Keyboard navigation support
- **NFR3.5**: Inline help text and tooltips

#### NFR4: Reliability
- **NFR4.1**: Admin UI failures must not crash the gateway
- **NFR4.2**: Configuration validation prevents invalid states
- **NFR4.3**: Rollback capability for configuration changes
- **NFR4.4**: Graceful degradation if metrics unavailable

#### NFR5: Maintainability
- **NFR5.1**: Minimal external dependencies (prefer stdlib)
- **NFR5.2**: Embedded assets (single binary deployment)
- **NFR5.3**: API versioning for future compatibility
- **NFR5.4**: Comprehensive error messages

---

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Browser Client                        │
│  ┌────────────┐  ┌──────────────┐  ┌──────────────────┐    │
│  │ Dashboard  │  │  Providers   │  │  Configuration   │    │
│  └────────────┘  └──────────────┘  └──────────────────┘    │
│  ┌────────────┐  ┌──────────────┐  ┌──────────────────┐    │
│  │   Models   │  │Conversations │  │      Logs        │    │
│  └────────────┘  └──────────────┘  └──────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ HTTPS
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    go-llm-gateway Server                     │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │              Middleware Stack                       │    │
│  │  Auth → Rate Limit → Logging → CORS → Router      │    │
│  └────────────────────────────────────────────────────┘    │
│                                                              │
│  ┌──────────────────┐  ┌──────────────────────────────┐   │
│  │  Gateway API     │  │      Admin API               │   │
│  │  /v1/*           │  │      /admin/api/*            │   │
│  ├──────────────────┤  ├──────────────────────────────┤   │
│  │ • /responses     │  │ • /config                    │   │
│  │ • /models        │  │ • /providers                 │   │
│  │ • /health        │  │ • /models                    │   │
│  │ • /ready         │  │ • /conversations             │   │
│  │ • /metrics       │  │ • /metrics                   │   │
│  └──────────────────┘  │ • /logs                      │   │
│                        │ • /system                    │   │
│  ┌──────────────────┐  └──────────────────────────────┘   │
│  │  Static Assets   │                                      │
│  │  /admin/*        │                                      │
│  │  (embedded)      │                                      │
│  └──────────────────┘                                      │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │              Core Components                        │    │
│  │  • Provider Registry                               │    │
│  │  • Conversation Store                              │    │
│  │  • Config Manager (new)                            │    │
│  │  • Metrics Collector                               │    │
│  │  • Log Buffer (new)                                │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Component Breakdown

#### Frontend Components

**Technology Stack Options:**
1. **Vue 3 + Vite** (Recommended)
   - Lightweight (~50KB gzipped)
   - Reactive data binding
   - Component-based architecture
   - Excellent TypeScript support

2. **Svelte + Vite** (Alternative)
   - Even lighter (~20KB)
   - Compile-time optimization
   - Simpler learning curve

3. **htmx + Alpine.js** (Minimal)
   - No build step
   - Server-rendered hypermedia
   - ~40KB total

**Recommended Choice:** Vue 3 + Vite + TypeScript
- Balance of features and bundle size
- Strong ecosystem and tooling
- Familiar to most developers

**Frontend Structure:**
```
frontend/
├── src/
│   ├── main.ts                 # App entry point
│   ├── App.vue                 # Root component
│   ├── router.ts               # Vue Router config
│   ├── api/                    # API client
│   │   ├── client.ts           # Axios/fetch wrapper
│   │   ├── config.ts           # Config API
│   │   ├── providers.ts        # Provider API
│   │   ├── models.ts           # Model API
│   │   ├── conversations.ts    # Conversation API
│   │   ├── metrics.ts          # Metrics API
│   │   └── system.ts           # System API
│   ├── components/             # Reusable components
│   │   ├── Layout.vue          # App layout
│   │   ├── Sidebar.vue         # Navigation
│   │   ├── Header.vue          # Top bar
│   │   ├── StatusBadge.vue     # Provider status
│   │   ├── MetricCard.vue      # Metric display
│   │   ├── ProviderForm.vue    # Provider editor
│   │   ├── ModelForm.vue       # Model editor
│   │   └── ConfigEditor.vue    # YAML/JSON editor
│   ├── views/                  # Page components
│   │   ├── Dashboard.vue       # Overview dashboard
│   │   ├── Providers.vue       # Provider management
│   │   ├── ProviderDetail.vue  # Single provider view
│   │   ├── Models.vue          # Model management
│   │   ├── Configuration.vue   # Config editor
│   │   ├── Conversations.vue   # Conversation browser
│   │   ├── Metrics.vue         # Metrics dashboard
│   │   ├── Logs.vue            # Log viewer
│   │   └── System.vue          # System info
│   ├── stores/                 # Pinia state management
│   │   ├── auth.ts             # Auth state
│   │   ├── config.ts           # Config state
│   │   ├── providers.ts        # Provider state
│   │   └── metrics.ts          # Metrics state
│   ├── types/                  # TypeScript types
│   │   └── api.ts              # API response types
│   └── utils/                  # Utilities
│       ├── formatting.ts       # Format helpers
│       └── validation.ts       # Form validation
├── public/
│   └── favicon.ico
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
└── README.md
```

#### Backend Components

**New Go Packages:**

```
internal/
├── admin/                      # Admin API package (NEW)
│   ├── handler.go              # HTTP handlers
│   ├── config_handler.go       # Config management
│   ├── provider_handler.go     # Provider management
│   ├── model_handler.go        # Model management
│   ├── conversation_handler.go # Conversation management
│   ├── metrics_handler.go      # Metrics aggregation
│   ├── logs_handler.go         # Log streaming
│   ├── system_handler.go       # System operations
│   └── middleware.go           # Admin-specific middleware
├── configmanager/              # Config management (NEW)
│   ├── manager.go              # Config CRUD operations
│   ├── validator.go            # Config validation
│   ├── diff.go                 # Config diff generation
│   └── reload.go               # Hot-reload logic
├── logbuffer/                  # Log buffering (NEW)
│   ├── buffer.go               # Circular log buffer
│   └── writer.go               # slog.Handler wrapper
└── auditlog/                   # Audit logging (NEW)
    ├── logger.go               # Audit event logger
    └── types.go                # Audit event types
```

### Data Flow

#### Configuration Update Flow

```
User clicks "Save Config" in UI
    ↓
Frontend validates form input
    ↓
POST /admin/api/config with new config
    ↓
Backend validates config structure
    ↓
Generate diff (old vs new)
    ↓
Return diff to frontend for confirmation
    ↓
User confirms change
    ↓
POST /admin/api/config/apply
    ↓
Write to config file (or temp file)
    ↓
Reload config (hot-reload or restart)
    ↓
Update audit log
    ↓
Return success/failure
    ↓
Frontend refreshes dashboard
```

#### Metrics Data Flow

```
Prometheus metrics continuously collected
    ↓
GET /admin/api/metrics
    ↓
Backend queries Prometheus registry
    ↓
Aggregate by provider, model, status
    ↓
Calculate percentiles and rates
    ↓
Return JSON response
    ↓
Frontend updates charts (auto-refresh every 5s)
```

---

## API Specification

### Base Path
All admin API endpoints are under `/admin/api/v1`

### Authentication
All endpoints require authentication via OIDC JWT token in `Authorization: Bearer <token>` header.

### Common Response Format

**Success Response:**
```json
{
  "success": true,
  "data": { /* endpoint-specific data */ },
  "timestamp": "2026-03-05T10:30:00Z"
}
```

**Error Response:**
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid provider configuration",
    "details": {
      "field": "api_key",
      "reason": "API key is required"
    }
  },
  "timestamp": "2026-03-05T10:30:00Z"
}
```

### Endpoints

#### System Information

**GET /admin/api/v1/system/info**

Get system information and status.

Response:
```json
{
  "success": true,
  "data": {
    "version": "1.2.0",
    "build_time": "2026-03-01T08:00:00Z",
    "git_commit": "59ded10",
    "go_version": "1.25.7",
    "platform": "linux/amd64",
    "uptime_seconds": 86400,
    "config_file": "/app/config.yaml",
    "config_last_modified": "2026-03-05T09:00:00Z"
  }
}
```

**GET /admin/api/v1/system/health**

Get detailed health status.

Response:
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "checks": {
      "server": { "status": "pass", "message": "Server running" },
      "providers": { "status": "pass", "message": "3/3 providers healthy" },
      "conversation_store": { "status": "pass", "message": "Connected to Redis" },
      "metrics": { "status": "pass", "message": "Prometheus collecting" }
    }
  }
}
```

**POST /admin/api/v1/system/restart**

Trigger graceful restart.

Request:
```json
{
  "countdown_seconds": 5,
  "reason": "Configuration update"
}
```

Response:
```json
{
  "success": true,
  "data": {
    "message": "Restart scheduled in 5 seconds",
    "restart_at": "2026-03-05T10:30:05Z"
  }
}
```

#### Configuration Management

**GET /admin/api/v1/config**

Get current configuration.

Query Parameters:
- `sanitized` (boolean, default: true) - Mask sensitive values (API keys)

Response:
```json
{
  "success": true,
  "data": {
    "config": {
      "server": {
        "address": ":8080",
        "max_request_body_size": 10485760
      },
      "logging": {
        "format": "json",
        "level": "info"
      },
      "providers": {
        "openai": {
          "type": "openai",
          "api_key": "sk-*********************xyz",
          "endpoint": "https://api.openai.com/v1"
        }
      },
      "models": [
        {
          "name": "gpt-4",
          "provider": "openai"
        }
      ]
    },
    "source": "file",
    "last_modified": "2026-03-05T09:00:00Z"
  }
}
```

**POST /admin/api/v1/config/validate**

Validate configuration without applying.

Request:
```json
{
  "config": {
    "server": { "address": ":8081" }
  }
}
```

Response:
```json
{
  "success": true,
  "data": {
    "valid": true,
    "warnings": [
      "Changing server address requires restart"
    ],
    "errors": []
  }
}
```

**POST /admin/api/v1/config/diff**

Generate diff between current and proposed config.

Request:
```json
{
  "new_config": { /* full or partial config */ }
}
```

Response:
```json
{
  "success": true,
  "data": {
    "diff": [
      {
        "path": "server.address",
        "old_value": ":8080",
        "new_value": ":8081",
        "type": "modified"
      },
      {
        "path": "providers.anthropic",
        "old_value": null,
        "new_value": { "type": "anthropic", "api_key": "***" },
        "type": "added"
      }
    ],
    "requires_restart": true
  }
}
```

**PUT /admin/api/v1/config**

Update configuration.

Request:
```json
{
  "config": { /* new configuration */ },
  "apply_method": "hot_reload",  // or "restart"
  "backup": true
}
```

Response:
```json
{
  "success": true,
  "data": {
    "applied": true,
    "method": "hot_reload",
    "backup_file": "/app/backups/config.yaml.2026-03-05-103000.bak",
    "changes": [ /* diff */ ]
  }
}
```

**GET /admin/api/v1/config/export**

Export configuration as YAML.

Response: (Content-Type: application/x-yaml)
```yaml
server:
  address: ":8080"
# ... full config
```

#### Provider Management

**GET /admin/api/v1/providers**

List all providers.

Response:
```json
{
  "success": true,
  "data": {
    "providers": [
      {
        "name": "openai",
        "type": "openai",
        "status": "healthy",
        "circuit_breaker_state": "closed",
        "endpoint": "https://api.openai.com/v1",
        "metrics": {
          "total_requests": 1523,
          "error_count": 12,
          "error_rate": 0.0079,
          "avg_latency_ms": 342,
          "p95_latency_ms": 876
        },
        "last_request_at": "2026-03-05T10:29:45Z",
        "last_error_at": "2026-03-05T09:15:22Z"
      }
    ]
  }
}
```

**GET /admin/api/v1/providers/{name}**

Get provider details.

Response:
```json
{
  "success": true,
  "data": {
    "name": "openai",
    "type": "openai",
    "config": {
      "api_key": "sk-*********************xyz",
      "endpoint": "https://api.openai.com/v1"
    },
    "status": "healthy",
    "circuit_breaker": {
      "state": "closed",
      "consecutive_failures": 0,
      "last_state_change": "2026-03-05T08:00:00Z"
    },
    "metrics": { /* detailed metrics */ }
  }
}
```

**POST /admin/api/v1/providers**

Add new provider.

Request:
```json
{
  "name": "anthropic-prod",
  "type": "anthropic",
  "config": {
    "api_key": "sk-ant-...",
    "endpoint": "https://api.anthropic.com"
  }
}
```

Response:
```json
{
  "success": true,
  "data": {
    "name": "anthropic-prod",
    "created": true
  }
}
```

**PUT /admin/api/v1/providers/{name}**

Update provider configuration.

Request:
```json
{
  "config": {
    "api_key": "new-key",
    "endpoint": "https://api.anthropic.com"
  }
}
```

**DELETE /admin/api/v1/providers/{name}**

Delete provider.

Response:
```json
{
  "success": true,
  "data": {
    "deleted": true,
    "affected_models": ["claude-3-opus", "claude-3-sonnet"]
  }
}
```

**POST /admin/api/v1/providers/{name}/test**

Test provider connectivity.

Request:
```json
{
  "test_message": "Hello, test",
  "model": "gpt-4"  // optional, uses default
}
```

Response:
```json
{
  "success": true,
  "data": {
    "reachable": true,
    "latency_ms": 342,
    "response": "Test successful",
    "error": null
  }
}
```

**POST /admin/api/v1/providers/{name}/circuit-breaker/reset**

Reset circuit breaker state.

Response:
```json
{
  "success": true,
  "data": {
    "previous_state": "open",
    "new_state": "closed"
  }
}
```

#### Model Management

**GET /admin/api/v1/models**

List all model configurations.

Response:
```json
{
  "success": true,
  "data": {
    "models": [
      {
        "name": "gpt-4",
        "provider": "openai",
        "provider_model_id": null,
        "metrics": {
          "total_requests": 856,
          "avg_latency_ms": 1234
        }
      },
      {
        "name": "gpt-4-azure",
        "provider": "azure-openai",
        "provider_model_id": "gpt-4-deployment-001",
        "metrics": {
          "total_requests": 234,
          "avg_latency_ms": 987
        }
      }
    ]
  }
}
```

**POST /admin/api/v1/models**

Add new model mapping.

Request:
```json
{
  "name": "claude-opus",
  "provider": "anthropic-prod",
  "provider_model_id": "claude-3-opus-20240229"
}
```

**PUT /admin/api/v1/models/{name}**

Update model mapping.

**DELETE /admin/api/v1/models/{name}**

Delete model mapping.

#### Conversation Management

**GET /admin/api/v1/conversations**

List conversations with pagination.

Query Parameters:
- `page` (int, default: 1)
- `page_size` (int, default: 50, max: 200)
- `search` (string) - Search by conversation ID
- `sort` (string) - Sort field (created_at, updated_at)
- `order` (string) - asc or desc

Response:
```json
{
  "success": true,
  "data": {
    "conversations": [
      {
        "id": "conv_abc123",
        "created_at": "2026-03-05T10:00:00Z",
        "updated_at": "2026-03-05T10:15:00Z",
        "message_count": 6,
        "total_tokens": 2456,
        "model": "gpt-4",
        "metadata": {}
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 50,
      "total_count": 1234,
      "total_pages": 25
    }
  }
}
```

**GET /admin/api/v1/conversations/{id}**

Get conversation details.

Response:
```json
{
  "success": true,
  "data": {
    "id": "conv_abc123",
    "created_at": "2026-03-05T10:00:00Z",
    "updated_at": "2026-03-05T10:15:00Z",
    "messages": [
      {
        "role": "user",
        "content": "Hello",
        "timestamp": "2026-03-05T10:00:00Z"
      },
      {
        "role": "assistant",
        "content": "Hi there!",
        "timestamp": "2026-03-05T10:00:02Z"
      }
    ],
    "metadata": {},
    "total_tokens": 2456
  }
}
```

**DELETE /admin/api/v1/conversations/{id}**

Delete specific conversation.

**POST /admin/api/v1/conversations/bulk-delete**

Bulk delete conversations.

Request:
```json
{
  "criteria": {
    "older_than_days": 30,
    "model": "gpt-3.5-turbo"  // optional filter
  },
  "dry_run": true  // preview without deleting
}
```

Response:
```json
{
  "success": true,
  "data": {
    "matched_count": 456,
    "deleted_count": 0,  // 0 if dry_run
    "dry_run": true
  }
}
```

**GET /admin/api/v1/conversations/stats**

Get conversation statistics.

Response:
```json
{
  "success": true,
  "data": {
    "total_conversations": 1234,
    "total_messages": 7890,
    "total_tokens": 1234567,
    "by_model": {
      "gpt-4": 856,
      "claude-3-opus": 378
    },
    "by_date": [
      { "date": "2026-03-05", "count": 123 },
      { "date": "2026-03-04", "count": 98 }
    ],
    "storage_size_bytes": 52428800
  }
}
```

#### Metrics

**GET /admin/api/v1/metrics/summary**

Get aggregated metrics summary.

Query Parameters:
- `duration` (string, default: "1h") - Time window (1m, 5m, 1h, 24h)

Response:
```json
{
  "success": true,
  "data": {
    "time_window": "1h",
    "request_count": 1523,
    "error_count": 12,
    "error_rate": 0.0079,
    "requests_per_second": 0.42,
    "latency": {
      "p50": 234,
      "p90": 567,
      "p95": 876,
      "p99": 1234
    },
    "by_provider": {
      "openai": {
        "request_count": 1200,
        "error_count": 8,
        "avg_latency_ms": 342
      },
      "anthropic": {
        "request_count": 323,
        "error_count": 4,
        "avg_latency_ms": 567
      }
    },
    "by_model": {
      "gpt-4": { "request_count": 856, "error_count": 5 },
      "claude-3-opus": { "request_count": 323, "error_count": 4 }
    }
  }
}
```

**GET /admin/api/v1/metrics/timeseries**

Get time-series metrics for charting.

Query Parameters:
- `metric` (string) - request_count, error_rate, latency_p95
- `duration` (string) - 1h, 6h, 24h, 7d
- `interval` (string) - 1m, 5m, 1h
- `provider` (string, optional) - Filter by provider
- `model` (string, optional) - Filter by model

Response:
```json
{
  "success": true,
  "data": {
    "metric": "request_count",
    "interval": "5m",
    "data_points": [
      { "timestamp": "2026-03-05T10:00:00Z", "value": 42 },
      { "timestamp": "2026-03-05T10:05:00Z", "value": 38 },
      { "timestamp": "2026-03-05T10:10:00Z", "value": 51 }
    ]
  }
}
```

#### Logs

**GET /admin/api/v1/logs**

Get recent logs (last N entries).

Query Parameters:
- `limit` (int, default: 100, max: 1000)
- `level` (string) - Filter by level (debug, info, warn, error)
- `search` (string) - Search in message

Response:
```json
{
  "success": true,
  "data": {
    "logs": [
      {
        "timestamp": "2026-03-05T10:30:15Z",
        "level": "info",
        "message": "Request completed",
        "fields": {
          "method": "POST",
          "path": "/v1/responses",
          "status": 200,
          "duration_ms": 342
        }
      }
    ],
    "total_count": 100,
    "truncated": false
  }
}
```

**GET /admin/api/v1/logs/stream**

Stream logs via Server-Sent Events (SSE).

Response: (text/event-stream)
```
data: {"timestamp":"2026-03-05T10:30:15Z","level":"info","message":"..."}

data: {"timestamp":"2026-03-05T10:30:16Z","level":"error","message":"..."}
```

#### Audit Log

**GET /admin/api/v1/audit**

Get audit log of admin actions.

Query Parameters:
- `page` (int)
- `page_size` (int)
- `user` (string) - Filter by user
- `action` (string) - Filter by action type

Response:
```json
{
  "success": true,
  "data": {
    "events": [
      {
        "id": "audit_xyz789",
        "timestamp": "2026-03-05T10:25:00Z",
        "user": "admin@example.com",
        "action": "config.update",
        "resource": "server.address",
        "changes": {
          "old_value": ":8080",
          "new_value": ":8081"
        },
        "ip_address": "192.168.1.100",
        "user_agent": "Mozilla/5.0..."
      }
    ],
    "pagination": { /* ... */ }
  }
}
```

---

## UI Design

### Design Principles

1. **Clarity over Complexity** - Show what matters, hide what doesn't
2. **Progressive Disclosure** - Surface details on demand
3. **Immediate Feedback** - Loading states, success/error messages
4. **Consistency** - Reuse patterns across views
5. **Accessibility** - Keyboard navigation, screen reader support

### Layout Structure

```
┌────────────────────────────────────────────────────────────┐
│  Header: [Logo] go-llm-gateway Admin  [User] [Dark Mode]  │
├──────────┬─────────────────────────────────────────────────┤
│          │                                                  │
│ Sidebar  │              Main Content Area                  │
│          │                                                  │
│ ☰ Dash   │  ┌─────────────────────────────────────────┐   │
│ 📊 Prov  │  │                                          │   │
│ 🔧 Model │  │                                          │   │
│ ⚙️  Conf │  │                                          │   │
│ 💬 Conv  │  │         Page-Specific Content            │   │
│ 📈 Metr  │  │                                          │   │
│ 📝 Logs  │  │                                          │   │
│ 🖥️  Sys  │  │                                          │   │
│          │  └─────────────────────────────────────────┘   │
│          │                                                  │
└──────────┴─────────────────────────────────────────────────┘
```

### Page Wireframes

#### 1. Dashboard (Home)

```
┌─────────────────────────────────────────────────────────────┐
│  Dashboard                                                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐          │
│  │  Uptime     │ │  Requests   │ │  Error Rate │          │
│  │  2d 14h     │ │  1,523      │ │  0.79%      │          │
│  │  ✓ Healthy  │ │  ↑ 12% 1h   │ │  ↓ 0.3% 1h  │          │
│  └─────────────┘ └─────────────┘ └─────────────┘          │
│                                                              │
│  Provider Status                                            │
│  ┌───────────────────────────────────────────────────────┐ │
│  │ openai       ✓ Healthy     │ 1,200 req │  342ms      │ │
│  │ anthropic    ✓ Healthy     │   323 req │  567ms      │ │
│  │ google       ⚠ Degraded    │     0 req │    0ms      │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                              │
│  Request Rate (Last Hour)                                   │
│  ┌───────────────────────────────────────────────────────┐ │
│  │      📊 [Line Chart]                                   │ │
│  │      requests/sec over time                            │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                              │
│  Recent Activity                                            │
│  ┌───────────────────────────────────────────────────────┐ │
│  │ 10:30:15 INFO  Request completed (gpt-4, 342ms)       │ │
│  │ 10:30:10 INFO  Request completed (claude-3, 567ms)    │ │
│  │ 10:29:58 ERROR Provider timeout (google)              │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

#### 2. Providers

```
┌─────────────────────────────────────────────────────────────┐
│  Providers                            [+ Add Provider]      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ ┌─┐ openai                                ✓ Healthy    ││
│  │ │▼│ Type: OpenAI                          [Test] [Edit]││
│  │ └─┘ Endpoint: https://api.openai.com/v1  [Delete]      ││
│  │                                                          ││
│  │     Circuit Breaker: Closed (0 failures)                ││
│  │     Metrics: 1,200 requests, 0.67% errors, 342ms avg    ││
│  │     Last request: 2 seconds ago                         ││
│  │                                                          ││
│  │     ┌──────────────────────────────────────────────┐   ││
│  │     │ Request Count:  [Mini chart ↗]               │   ││
│  │     │ Latency P95:    [Mini chart →]               │   ││
│  │     └──────────────────────────────────────────────┘   ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ ┌─┐ anthropic-prod                       ✓ Healthy    ││
│  │ │▶│ Type: Anthropic                      [Test] [Edit]││
│  │ └─┘ Endpoint: https://api.anthropic.com  [Delete]      ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ ┌─┐ google                                ⚠ Degraded   ││
│  │ │▶│ Type: Google Generative AI           [Test] [Edit]││
│  │ └─┘ Circuit Breaker: OPEN (5 failures)   [Delete]      ││
│  │                                           [Reset CB]    ││
│  └────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

**Add/Edit Provider Modal:**
```
┌─────────────────────────────────────────────────────┐
│  Add Provider                              [X]      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  Provider Name *                                    │
│  [openai-prod              ]                        │
│                                                      │
│  Provider Type *                                    │
│  [OpenAI        ▼]                                  │
│                                                      │
│  API Key *                                          │
│  [sk-••••••••••••••••••••xyz]  [Show] [Test]       │
│                                                      │
│  Endpoint (optional)                                │
│  [https://api.openai.com/v1]                        │
│                                                      │
│  ⓘ Leave blank to use default endpoint              │
│                                                      │
│                          [Cancel]  [Save Provider]  │
└─────────────────────────────────────────────────────┘
```

#### 3. Models

```
┌─────────────────────────────────────────────────────────────┐
│  Models                                  [+ Add Model]      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Search: [          🔍]     Filter: [All Providers ▼]       │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ Name              Provider      Model ID      Requests ││
│  ├────────────────────────────────────────────────────────┤│
│  │ gpt-4             openai        (default)     856      ││
│  │ gpt-4-turbo       openai        (default)     432      ││
│  │ gpt-4-azure       azure-openai  gpt4-dep-001  234      ││
│  │ claude-3-opus     anthropic     claude-3-...  323      ││
│  │ claude-3-sonnet   anthropic     claude-3-...  189      ││
│  │ gemini-pro        google        (default)     56       ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  [← Prev]  Page 1 of 1  [Next →]                           │
└─────────────────────────────────────────────────────────────┘
```

#### 4. Configuration

```
┌─────────────────────────────────────────────────────────────┐
│  Configuration                                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  [Server] [Logging] [Rate Limit] [Auth] [Observability]    │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  Server Configuration                                       │
│  ┌────────────────────────────────────────────────────────┐│
│  │                                                          ││
│  │  Listen Address                                         ││
│  │  [:8080              ]                                  ││
│  │                                                          ││
│  │  Max Request Body Size (bytes)                          ││
│  │  [10485760           ]  (10 MB)                         ││
│  │                                                          ││
│  │  Read Timeout (seconds)                                 ││
│  │  [15                 ]                                  ││
│  │                                                          ││
│  │  Write Timeout (seconds)                                ││
│  │  [60                 ]                                  ││
│  │                                                          ││
│  │  Idle Timeout (seconds)                                 ││
│  │  [120                ]                                  ││
│  │                                                          ││
│  │  ⚠ Changing these settings requires a restart           ││
│  │                                                          ││
│  │                       [Reset]  [Save Configuration]     ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Advanced Options                                           │
│  [View as YAML]  [Export Config]  [Import Config]          │
└─────────────────────────────────────────────────────────────┘
```

**YAML Editor View:**
```
┌─────────────────────────────────────────────────────────────┐
│  Configuration (YAML)              [Switch to Form View]   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │  1  server:                                            ││
│  │  2    address: ":8080"                                 ││
│  │  3    max_request_body_size: 10485760                  ││
│  │  4                                                      ││
│  │  5  logging:                                           ││
│  │  6    format: "json"                                   ││
│  │  7    level: "info"                                    ││
│  │  8                                                      ││
│  │  9  providers:                                         ││
│  │ 10    openai:                                          ││
│  │ 11      type: "openai"                                 ││
│  │ 12      api_key: "${OPENAI_API_KEY}"                   ││
│  │                                                         ││
│  │ [Syntax highlighting and validation]                   ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  ✓ Configuration is valid                                  │
│                                                              │
│  [Show Diff]  [Validate]  [Save Configuration]             │
└─────────────────────────────────────────────────────────────┘
```

#### 5. Conversations

```
┌─────────────────────────────────────────────────────────────┐
│  Conversations                                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Search: [conv_abc123     🔍]   [Bulk Delete...]           │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ ID            Created    Messages  Model       Actions ││
│  ├────────────────────────────────────────────────────────┤│
│  │ conv_abc123   2h ago     6         gpt-4       [View]  ││
│  │ conv_def456   3h ago     12        claude-3    [View]  ││
│  │ conv_ghi789   5h ago     3         gpt-4       [View]  ││
│  │ conv_jkl012   1d ago     8         gemini-pro  [View]  ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  [← Prev]  Page 1 of 25 (1,234 total)  [Next →]           │
│                                                              │
│  Statistics                                                 │
│  Total: 1,234 conversations  |  7,890 messages  |  52 MB   │
└─────────────────────────────────────────────────────────────┘
```

**Conversation Detail Modal:**
```
┌─────────────────────────────────────────────────────────────┐
│  Conversation: conv_abc123                    [Delete] [X] │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Created: 2026-03-05 08:15:30  |  Model: gpt-4             │
│  Messages: 6  |  Tokens: 2,456  |  Updated: 08:30:15       │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 👤 User (08:15:30)                                     ││
│  │ Hello, can you help me with a coding question?         ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 🤖 Assistant (08:15:32)                                ││
│  │ Of course! I'd be happy to help. What's your question?││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 👤 User (08:16:10)                                     ││
│  │ How do I implement a binary search in Python?          ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  [... more messages ...]                                    │
│                                                              │
│                                              [Close]        │
└─────────────────────────────────────────────────────────────┘
```

#### 6. Metrics

```
┌─────────────────────────────────────────────────────────────┐
│  Metrics                    Time: [Last Hour ▼]  [Refresh] │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Overview                                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│  │ Total Req    │ │ Requests/sec │ │ Error Rate   │       │
│  │ 1,523        │ │ 0.42         │ │ 0.79%        │       │
│  └──────────────┘ └──────────────┘ └──────────────┘       │
│                                                              │
│  Request Rate                                               │
│  ┌────────────────────────────────────────────────────────┐│
│  │  50 ┤                                                   ││
│  │  40 ┤              ╭─╮                                  ││
│  │  30 ┤         ╭────╯ ╰─╮                               ││
│  │  20 ┤    ╭────╯        ╰──╮                            ││
│  │  10 ┤────╯                ╰────                         ││
│  │   0 ┼────────────────────────────────────              ││
│  │     9:30   10:00   10:30   11:00                       ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Latency (P95)                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 1200ms ┤                                                ││
│  │  900ms ┤         ╭─────╮                               ││
│  │  600ms ┤─────────╯     ╰─────────                      ││
│  │  300ms ┤                                                ││
│  │      0 ┼────────────────────────────────────            ││
│  │        9:30   10:00   10:30   11:00                    ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  By Provider                                                │
│  ┌────────────────────────────────────────────────────────┐│
│  │ Provider    Requests  Errors  Avg Latency  P95        ││
│  ├────────────────────────────────────────────────────────┤│
│  │ openai      1,200     8       342ms        876ms      ││
│  │ anthropic   323       4       567ms        1234ms     ││
│  │ google      0         0       -            -          ││
│  └────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

#### 7. Logs

```
┌─────────────────────────────────────────────────────────────┐
│  Logs                   [Auto-refresh: ON]  [Download]     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Level: [All ▼]  Search: [          🔍]                     │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 10:30:45 INFO  Request completed                       ││
│  │              method=POST path=/v1/responses status=200 ││
│  │              duration=342ms model=gpt-4                ││
│  │                                                         ││
│  │ 10:30:42 INFO  Provider request started                ││
│  │              provider=openai model=gpt-4               ││
│  │                                                         ││
│  │ 10:30:30 ERROR Provider request failed                 ││
│  │              provider=google error="connection timeout"││
│  │              circuit_breaker=open                      ││
│  │                                                         ││
│  │ 10:30:15 INFO  Request completed                       ││
│  │              method=POST path=/v1/responses status=200 ││
│  │                                                         ││
│  │ 10:29:58 WARN  Rate limit exceeded                     ││
│  │              ip=192.168.1.100 path=/v1/responses       ││
│  │                                                         ││
│  │ [... scrollable log entries ...]                       ││
│  │                                                         ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Showing last 100 entries  |  [Load More]                  │
└─────────────────────────────────────────────────────────────┘
```

#### 8. System

```
┌─────────────────────────────────────────────────────────────┐
│  System Information                                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Application                                                │
│  ┌────────────────────────────────────────────────────────┐│
│  │ Version:        1.2.0                                   ││
│  │ Build Time:     2026-03-01 08:00:00 UTC                ││
│  │ Git Commit:     59ded10                                ││
│  │ Go Version:     1.25.7                                 ││
│  │ Platform:       linux/amd64                            ││
│  │ Uptime:         2 days 14 hours 23 minutes             ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Configuration                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ Config File:    /app/config.yaml                       ││
│  │ Last Modified:  2026-03-05 09:00:00 UTC                ││
│  │ File Size:      4.2 KB                                 ││
│  │ Valid:          ✓ Yes                                  ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Health Checks                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │ ✓ Server               Healthy                         ││
│  │ ✓ Providers            3/3 healthy                     ││
│  │ ✓ Conversation Store   Connected (Redis)               ││
│  │ ✓ Metrics              Collecting                      ││
│  │ ✓ Tracing              Enabled (OTLP)                  ││
│  └────────────────────────────────────────────────────────┘│
│                                                              │
│  Operations                                                 │
│  [Download Diagnostic Bundle]  [Restart Service...]        │
│                                                              │
│  Environment (Sanitized)                                    │
│  [View Environment Variables]                              │
└─────────────────────────────────────────────────────────────┘
```

### UI Components Library

**Reusable Components:**

1. **StatusBadge** - Color-coded status indicators
   - Healthy (green), Degraded (yellow), Unhealthy (red), Unknown (gray)

2. **MetricCard** - Display single metric with trend
   - Large number, label, trend arrow, sparkline

3. **ProviderCard** - Provider summary with expand/collapse

4. **DataTable** - Sortable, filterable table with pagination

5. **Chart** - Line/bar charts for time-series data
   - Use lightweight charting library (Chart.js or Apache ECharts)

6. **CodeEditor** - Syntax-highlighted YAML/JSON editor
   - Monaco Editor (VS Code engine) or CodeMirror

7. **Modal** - Overlay dialogs for forms and details

8. **Toast** - Success/error notifications

9. **ConfirmDialog** - Confirmation for destructive actions

---

## Security

### Authentication & Authorization

**Authentication:**
- Reuse existing OIDC/OAuth2 middleware from `internal/auth/auth.go`
- All `/admin/*` routes require valid JWT token
- Support same identity providers as gateway API

**Authorization (RBAC):**

Introduce role-based access control with two roles:

1. **Admin Role** (`admin`)
   - Full read/write access
   - Can modify configuration
   - Can delete resources (conversations, providers)
   - Can restart service

2. **Viewer Role** (`viewer`)
   - Read-only access
   - Can view all pages
   - Cannot modify configuration
   - Cannot delete resources
   - Cannot restart service

**Role Assignment:**
- Roles extracted from JWT claims (e.g., `roles` or `groups` claim)
- Configurable claim name in config.yaml:
  ```yaml
  auth:
    enabled: true
    issuer: "https://auth.example.com"
    audience: "gateway-admin"
    roles_claim: "roles"  # JWT claim containing roles
    admin_roles:          # Values that grant admin access
      - "admin"
      - "gateway-admin"
  ```

**Implementation:**
```go
// internal/admin/middleware.go

func RequireRole(requiredRole string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := auth.ClaimsFromContext(r.Context())
            userRoles := claims["roles"].([]string)

            if !hasRole(userRoles, requiredRole) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

// Usage in routes
mux.Handle("/admin/api/v1/config", RequireRole("admin")(configHandler))
mux.Handle("/admin/api/v1/providers", RequireRole("viewer")(providersHandler))
```

### Input Validation & Sanitization

**Configuration Validation:**
- Validate all config changes before applying
- Use strong typing (Go structs) for validation
- Reject invalid YAML syntax
- Validate provider-specific fields (API key format, endpoint URLs)
- Prevent path traversal in file operations

**API Input Validation:**
- Validate all request bodies against expected schemas
- Sanitize user input (conversation search, log search)
- Limit input sizes (prevent DoS via large payloads)
- Validate pagination parameters (prevent negative pages)

### Secret Management

**Masking Secrets:**
- Always mask API keys and sensitive values in UI displays
- Show format: `sk-*********************xyz` (first 3 + last 3 chars)
- Never log full API keys in audit logs
- Sanitize secrets before returning in API responses

**Storage:**
- Secrets stored in config.yaml with environment variable references
- Never commit secrets to version control
- Support secret management systems (future: Vault, AWS Secrets Manager)

### CSRF Protection

**Protection Strategy:**
- Generate CSRF token on admin UI load
- Include token in all state-changing requests (POST, PUT, DELETE)
- Validate token on server before processing request
- Use SameSite cookies for additional protection

**Implementation:**
```go
// Double Submit Cookie pattern
func CSRFMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "GET" && r.Method != "HEAD" {
            tokenHeader := r.Header.Get("X-CSRF-Token")
            tokenCookie, _ := r.Cookie("csrf_token")

            if tokenHeader == "" || tokenCookie == nil || tokenHeader != tokenCookie.Value {
                http.Error(w, "CSRF token mismatch", http.StatusForbidden)
                return
            }
        }

        next.ServeHTTP(w, r)
    })
}
```

### Content Security Policy

**CSP Headers:**
```
Content-Security-Policy:
  default-src 'self';
  script-src 'self' 'unsafe-inline';  # Allow inline Vue scripts
  style-src 'self' 'unsafe-inline';   # Allow inline styles
  img-src 'self' data:;
  connect-src 'self';                 # API calls to same origin
  frame-ancestors 'none';             # Prevent clickjacking
  base-uri 'self';
  form-action 'self';
```

### Rate Limiting

**Admin API Rate Limiting:**
- Separate rate limits for admin API vs gateway API
- Higher limits for read operations, lower for writes
- Per-user rate limiting (based on JWT subject)
- Example: 100 req/min for reads, 20 req/min for writes

### Audit Logging

**Log All Admin Actions:**
- Configuration changes (before/after values)
- Provider additions/deletions
- Model changes
- Bulk deletions
- Service restarts
- Authentication failures

**Audit Log Format:**
```json
{
  "timestamp": "2026-03-05T10:25:00Z",
  "event_type": "config.update",
  "user": "admin@example.com",
  "user_ip": "192.168.1.100",
  "resource": "providers.openai.api_key",
  "action": "update",
  "old_value": "sk-***old***",
  "new_value": "sk-***new***",
  "success": true,
  "error": null
}
```

**Storage:**
- Write to separate audit log file (`/var/log/gateway-audit.log`)
- Structured JSON format for easy parsing
- Rotate logs daily, retain for 90 days
- Optional: Send to external SIEM system

### TLS/HTTPS

**Production Requirements:**
- Admin UI MUST be served over HTTPS in production
- Support TLS 1.2+ only
- Strong cipher suites only
- HSTS headers: `Strict-Transport-Security: max-age=31536000; includeSubDomains`

**Configuration:**
```yaml
server:
  address: ":8443"
  tls:
    enabled: true
    cert_file: "/etc/gateway/tls/cert.pem"
    key_file: "/etc/gateway/tls/key.pem"
```

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

**Goal:** Basic admin API and static UI serving

**Backend Tasks:**
1. Create `internal/admin/` package structure
2. Implement basic HTTP handlers for system info and health
3. Add static file serving for admin UI assets (using `embed.FS`)
4. Set up admin-specific middleware (auth, CORS, CSRF)
5. Implement audit logging infrastructure

**Frontend Tasks:**
1. Set up Vue 3 + Vite project in `frontend/admin/`
2. Create basic layout (header, sidebar, main content)
3. Implement routing (Vue Router)
4. Create API client wrapper (Axios)
5. Build Dashboard page (system info, health status)

**Deliverables:**
- Admin UI accessible at `/admin/`
- System info and health endpoints working
- Basic authentication enforced
- Static assets served from embedded FS

### Phase 2: Configuration Management (Week 2)

**Goal:** View and edit configuration

**Backend Tasks:**
1. Create `internal/configmanager/` package
2. Implement config CRUD operations
3. Add config validation logic
4. Implement diff generation
5. Add config export/import endpoints
6. Implement hot-reload for config changes (where possible)

**Frontend Tasks:**
1. Build Configuration page with tabbed interface
2. Implement form-based config editor
3. Build YAML editor with syntax highlighting (Monaco Editor)
4. Add config validation UI
5. Implement diff viewer before applying changes
6. Add export/import functionality

**Deliverables:**
- View current configuration (sanitized)
- Edit configuration via forms or YAML
- Validate configuration before saving
- Preview changes before applying
- Export configuration as YAML file

### Phase 3: Provider & Model Management (Week 3)

**Goal:** Manage providers and models

**Backend Tasks:**
1. Implement provider CRUD endpoints
2. Add provider test connectivity endpoint
3. Implement circuit breaker reset endpoint
4. Add model CRUD endpoints
5. Aggregate provider metrics from Prometheus

**Frontend Tasks:**
1. Build Providers page with expandable cards
2. Implement provider add/edit forms
3. Add provider connection testing
4. Display provider metrics and circuit breaker status
5. Build Models page with data table
6. Implement model add/edit functionality

**Deliverables:**
- List all providers with status
- Add/edit/delete providers
- Test provider connectivity
- Reset circuit breakers
- Manage model mappings

### Phase 4: Metrics & Monitoring (Week 4)

**Goal:** Real-time metrics visualization

**Backend Tasks:**
1. Implement metrics aggregation endpoints
2. Add time-series data endpoints
3. Implement metrics filtering (by provider, model)
4. Add circuit breaker state change history

**Frontend Tasks:**
1. Build Metrics page with charts (Chart.js)
2. Implement real-time metrics (auto-refresh)
3. Add interactive time range selection
4. Build provider-specific metric views
5. Add latency percentile charts

**Deliverables:**
- Real-time request rate charts
- Error rate visualization
- Latency percentile charts
- Provider-specific metrics
- Auto-refreshing dashboard

### Phase 5: Conversations & Logs (Week 5)

**Goal:** Conversation management and log viewing

**Backend Tasks:**
1. Implement `internal/logbuffer/` for log buffering
2. Add conversation list/search endpoints
3. Implement conversation detail endpoint
4. Add bulk delete functionality
5. Implement log streaming (SSE)

**Frontend Tasks:**
1. Build Conversations page with pagination
2. Implement conversation search
3. Add conversation detail modal
4. Build bulk delete interface
5. Build Logs page with filtering
6. Implement real-time log streaming

**Deliverables:**
- Browse and search conversations
- View conversation details
- Delete conversations (single and bulk)
- View application logs with filtering
- Real-time log streaming

### Phase 6: Polish & Production Readiness (Week 6)

**Goal:** Security hardening, testing, documentation

**Tasks:**
1. Implement RBAC (admin vs viewer roles)
2. Add comprehensive input validation
3. Implement CSRF protection
4. Add CSP headers
5. Write unit tests (backend handlers)
6. Write integration tests (API endpoints)
7. Add E2E tests (Playwright)
8. Performance optimization (bundle size, lazy loading)
9. Accessibility audit and fixes
10. Documentation (user guide, API docs)
11. Docker image updates (include frontend build)

**Deliverables:**
- Production-ready security hardening
- Comprehensive test coverage
- Performance optimized
- Fully documented
- Docker deployment ready

---

## Testing Strategy

### Backend Testing

**Unit Tests:**
- Test all handler functions with mock dependencies
- Test config validation logic
- Test audit logging
- Target: 80%+ code coverage

**Integration Tests:**
- Test API endpoints with real HTTP requests
- Test authentication/authorization flows
- Test RBAC enforcement
- Test configuration hot-reload

**Example:**
```go
func TestProviderHandler(t *testing.T) {
    tests := []struct {
        name           string
        method         string
        path           string
        body           string
        expectedStatus int
    }{
        {
            name:           "List providers",
            method:         "GET",
            path:           "/admin/api/v1/providers",
            expectedStatus: http.StatusOK,
        },
        {
            name:           "Add provider",
            method:         "POST",
            path:           "/admin/api/v1/providers",
            body:           `{"name":"test","type":"openai","config":{"api_key":"sk-test"}}`,
            expectedStatus: http.StatusCreated,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Frontend Testing

**Unit Tests (Vitest):**
- Test Vue components in isolation
- Test API client functions
- Test utility functions
- Target: 70%+ component coverage

**Component Tests:**
- Test user interactions
- Test form validation
- Test state management (Pinia stores)

**E2E Tests (Playwright):**
- Test complete user workflows
- Test authentication flow
- Test config editing flow
- Test provider management

**Example:**
```typescript
// tests/e2e/providers.spec.ts
test('should add new provider', async ({ page }) => {
  await page.goto('/admin/providers');
  await page.click('text=Add Provider');
  await page.fill('input[name="name"]', 'test-provider');
  await page.selectOption('select[name="type"]', 'openai');
  await page.fill('input[name="api_key"]', 'sk-test-key');
  await page.click('button:has-text("Save Provider")');

  await expect(page.locator('.toast-success')).toBeVisible();
  await expect(page.locator('text=test-provider')).toBeVisible();
});
```

### Performance Testing

**Load Testing:**
- Test admin API under load (Apache Bench, k6)
- Ensure < 1% CPU overhead when admin UI active
- Test with 10 concurrent admin users
- Verify no impact on gateway API performance

**Frontend Performance:**
- Lighthouse audit (target: 90+ performance score)
- Bundle size analysis (target: < 500KB gzipped)
- Time to Interactive (target: < 2s)

### Security Testing

**Automated Scans:**
- OWASP ZAP scan for common vulnerabilities
- npm audit / go mod audit for dependency vulnerabilities
- CodeQL static analysis

**Manual Testing:**
- Test RBAC enforcement
- Test CSRF protection
- Test secret masking
- Test input validation
- Test audit logging

---

## Deployment

### Build Process

**Frontend Build:**
```bash
cd frontend/admin
npm install
npm run build  # Outputs to frontend/admin/dist/
```

**Embed Frontend in Go Binary:**
```go
// internal/admin/assets.go
package admin

import "embed"

//go:embed frontend/dist/*
var frontendAssets embed.FS
```

**Full Build:**
```bash
# Build frontend
cd frontend/admin && npm run build && cd ../..

# Build Go binary (includes embedded frontend)
go build -o gateway ./cmd/gateway

# Result: Single binary with admin UI embedded
```

### Docker Image

**Updated Dockerfile:**
```dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend/admin
COPY frontend/admin/package*.json ./
RUN npm ci
COPY frontend/admin/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25.7-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/admin/dist ./internal/admin/frontend/dist
RUN CGO_ENABLED=1 go build -o gateway ./cmd/gateway

# Stage 3: Runtime
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=go-builder /app/gateway /app/gateway
COPY config.example.yaml /app/config.yaml
EXPOSE 8080
USER 1000:1000
ENTRYPOINT ["/app/gateway"]
```

**Build Command:**
```bash
docker build -t go-llm-gateway:latest .
```

### Kubernetes Deployment

**Updated Deployment:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: gateway
        image: go-llm-gateway:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: gateway-secrets
              key: openai-api-key
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
      volumes:
      - name: config
        configMap:
          name: gateway-config
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8080
    name: http
  selector:
    app: gateway
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gateway-admin
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - admin.gateway.example.com
    secretName: gateway-admin-tls
  rules:
  - host: admin.gateway.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: gateway
            port:
              number: 80
```

### Configuration Management

**Production Config:**
```yaml
# config.yaml
server:
  address: ":8080"
  tls:
    enabled: false  # Terminated at ingress

auth:
  enabled: true
  issuer: "https://auth.example.com"
  audience: "gateway-admin"
  roles_claim: "roles"
  admin_roles: ["admin", "gateway-admin"]

admin:
  enabled: true
  base_path: "/admin"
  cors:
    allowed_origins:
      - "https://admin.gateway.example.com"
    allowed_methods: ["GET", "POST", "PUT", "DELETE"]
    allowed_headers: ["Authorization", "Content-Type", "X-CSRF-Token"]
```

### Monitoring

**Prometheus Metrics:**

New metrics for admin UI:
```
# Admin API request count
gateway_admin_requests_total{endpoint, method, status}

# Admin API request duration
gateway_admin_request_duration_seconds{endpoint, method}

# Configuration changes
gateway_admin_config_changes_total{user, resource}

# Authentication failures
gateway_admin_auth_failures_total{reason}
```

**Grafana Dashboard:**

Create dedicated admin UI dashboard with panels for:
- Admin API request rate
- Admin API error rate
- Configuration change timeline
- Active admin sessions
- Authentication failures

### Backup & Recovery

**Configuration Backup:**
- Automatic backup before applying config changes
- Stored in `/app/backups/config.yaml.TIMESTAMP.bak`
- Retain last 10 backups
- Restore via UI or CLI

**Audit Log Backup:**
- Rotate audit logs daily
- Compress and archive old logs
- Retain for 90 days (configurable)
- Optional: Ship to external storage (S3, GCS)

---

## Future Enhancements

### Phase 2 Features (Post-MVP)

1. **Multi-Instance Management**
   - Manage multiple gateway instances from single UI
   - Fleet view with aggregate metrics
   - Centralized configuration management

2. **Advanced Monitoring**
   - Custom alerting rules
   - Anomaly detection (ML-based)
   - Cost tracking per provider/model
   - Token usage forecasting

3. **Enhanced Security**
   - SSO integration (SAML, LDAP)
   - Fine-grained permissions (resource-level RBAC)
   - API key rotation automation
   - Secret management integration (HashiCorp Vault)

4. **Configuration Templates**
   - Pre-built provider templates
   - Environment-specific configs (dev, staging, prod)
   - Config versioning and rollback
   - Git integration for config-as-code

5. **Testing & Debugging**
   - Interactive API playground (Swagger UI style)
   - Request/response inspector
   - Provider response comparison
   - Load testing tools

6. **Conversation Analytics**
   - Conversation analytics dashboard
   - Topic clustering
   - Sentiment analysis
   - Export conversations to CSV/JSON

7. **User Management**
   - Multi-user support (not just admins)
   - Team workspaces
   - Usage quotas per user/team
   - Billing integration

8. **Notifications**
   - Email/Slack alerts for errors
   - Webhook support for events
   - Scheduled reports (daily/weekly summaries)

9. **Mobile Support**
   - Progressive Web App (PWA)
   - Native mobile app (React Native)
   - Push notifications

10. **AI-Powered Features**
    - Automatic provider selection based on query type
    - Cost optimization suggestions
    - Performance recommendations
    - Anomaly detection in logs

### Technical Debt & Improvements

1. **Performance Optimizations**
   - Server-side pagination for large datasets
   - Caching layer (Redis) for metrics
   - WebSocket for real-time updates (replace polling)
   - GraphQL API (alternative to REST)

2. **Developer Experience**
   - Admin API SDK (TypeScript, Python)
   - Terraform provider for config management
   - CLI tool for admin operations
   - OpenAPI/Swagger spec for API

3. **Observability**
   - Distributed tracing for admin operations
   - Request correlation IDs
   - Detailed error tracking (Sentry integration)
   - User session replay (LogRocket style)

4. **Internationalization**
   - Multi-language UI support
   - Localized date/time formats
   - Currency formatting for costs

---

## Appendix

### Technology Choices Rationale

**Why Vue 3?**
- Lightweight (50KB gzipped vs React's 130KB)
- Progressive framework (can start simple, add complexity as needed)
- Excellent TypeScript support
- Single-file components (easy to understand)
- Strong ecosystem (Vue Router, Pinia)

**Why embed.FS?**
- Single binary deployment (no separate asset hosting)
- Simplifies Docker images
- No CDN dependencies
- Faster initial load (no external requests)

**Why Monaco Editor?**
- Full VS Code editing experience
- Excellent YAML/JSON support
- Syntax validation built-in
- Auto-completion

**Why Chart.js?**
- Simple API
- Good performance for real-time updates
- Small bundle size (~40KB)
- Responsive by default

### Alternative Architectures Considered

1. **Server-Side Rendering (SSR)**
   - Pros: Better SEO, faster initial load
   - Cons: More complex deployment, slower interactions
   - Decision: Not needed for admin UI (auth-required, no SEO needs)

2. **Separate Admin Service**
   - Pros: True separation of concerns, independent scaling
   - Cons: More infrastructure, harder deployment, network latency
   - Decision: Embedded admin (simpler, one binary)

3. **GraphQL API**
   - Pros: Flexible queries, reduced over-fetching
   - Cons: Added complexity, overkill for admin use case
   - Decision: REST API (simpler, adequate)

4. **WebSockets for Real-Time**
   - Pros: True bi-directional real-time
   - Cons: Connection management complexity, harder to scale
   - Decision: SSE + polling (simpler, sufficient)

### Security Considerations Summary

| Threat                    | Mitigation                                   |
|---------------------------|----------------------------------------------|
| Unauthorized access       | OIDC authentication required                 |
| Privilege escalation      | RBAC with admin/viewer roles                 |
| CSRF attacks              | Double-submit cookie pattern                 |
| XSS attacks               | CSP headers, Vue auto-escaping               |
| Secret exposure           | Mask secrets in UI, audit logs               |
| Injection attacks         | Input validation, parameterized queries      |
| DoS attacks               | Rate limiting, request size limits           |
| Man-in-the-middle         | HTTPS/TLS required in production             |
| Session hijacking         | Secure cookies, short JWT expiry             |
| Brute force auth          | Rate limiting on auth endpoints              |

### Performance Benchmarks (Targets)

| Metric                    | Target         | Notes                          |
|---------------------------|----------------|--------------------------------|
| Dashboard load time       | < 2s           | On modern browsers, 4G network |
| API response time (p95)   | < 500ms        | For most endpoints             |
| Concurrent admin users    | 10+            | Without degradation            |
| CPU overhead              | < 1%           | When admin UI active           |
| Memory overhead           | < 50MB         | For admin UI components        |
| Frontend bundle size      | < 500KB        | Gzipped, with code splitting   |
| Time to Interactive (TTI) | < 3s           | Lighthouse metric              |

---

## Success Metrics

### Adoption Metrics
- Number of active admin users per week
- Frequency of configuration changes
- Time spent in admin UI per session

### Efficiency Metrics
- Reduction in configuration errors (target: 50%)
- Time to configure new provider (target: < 2 minutes)
- Time to diagnose issues (target: < 5 minutes)

### Reliability Metrics
- Admin UI uptime (target: 99.9%)
- Zero impact on gateway API performance
- Admin API error rate (target: < 0.1%)

### User Satisfaction
- User feedback score (target: 4.5/5)
- Feature adoption rate (target: 80% use within 1 month)
- Support ticket reduction (target: 30% reduction)

---

## References

- [Go embed package](https://pkg.go.dev/embed)
- [Vue 3 Documentation](https://vuejs.org/)
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)

---

**Document Version:** 1.0
**Last Updated:** 2026-03-05
**Authors:** Development Team
**Status:** Draft - Pending Review
