# Data Retention & Storage Policy

## Overview

LatticeLM defaults to **no-store** for all conversation data. Persistence must
be explicitly enabled in the server configuration and/or requested per-call by
the client.

## Configuration

```yaml
conversations:
  enabled: true            # master switch (default: false)
  store_by_default: false  # persist when client omits "store" (default: false)
  store: "memory"          # backend: "memory", "sql", or "redis"
  ttl: "1h"                # expiration duration
  max_ttl: "24h"           # ceiling that overrides any higher ttl value
```

| Field              | Default | Description |
|--------------------|---------|-------------|
| `enabled`          | `false` | Master switch. When `false`, a no-op store is used and nothing is persisted. |
| `store_by_default` | `false` | Controls behavior when the client request does not include `"store"`. |
| `store`            | `memory`| Backend type. |
| `ttl`              | none    | Conversation expiration. Zero means no expiry (subject to `max_ttl`). |
| `max_ttl`          | none    | Hard ceiling. If `ttl` is unset or exceeds `max_ttl`, it is clamped. |

## Per-Request Control

Clients can set `"store": true` or `"store": false` in the request body:

```json
{
  "model": "gpt-4",
  "input": "Hello",
  "store": false
}
```

- `"store": false` — **strict no-persist**: no conversation record is created
  regardless of server defaults.
- `"store": true` — conversation is persisted (requires `conversations.enabled`
  on the server side).
- Field omitted — falls back to `store_by_default`.

## Deletion

Stored conversations can be deleted via the API:

```
DELETE /v1/responses/{response_id}
```

Returns `204 No Content` on success, `404` if the conversation does not exist.

## TTL & Automatic Expiry

All backends support automatic cleanup of expired conversations:

- **Memory**: background goroutine sweeps every minute.
- **SQL**: background goroutine runs at `min(ttl/10, 1 minute)` intervals.
- **Redis**: native key TTL; no background sweep needed.

## Operational Guidance

1. **Production default**: keep `enabled: false` unless conversation history is
   a product requirement.
2. **Minimise retention**: set `max_ttl` to the shortest acceptable window
   (e.g. `"1h"` or `"24h"`).
3. **Redaction**: if you need to redact specific messages, use the DELETE
   endpoint to remove the full conversation record. Individual message-level
   redaction is not currently supported.
4. **Audit**: enable structured logging (`logging.format: "json"`) to capture
   `stored` flags on every request for compliance auditing.
5. **Encryption at rest**: for SQL/Redis backends, configure TLS and
   disk-level encryption on the backing data store.
