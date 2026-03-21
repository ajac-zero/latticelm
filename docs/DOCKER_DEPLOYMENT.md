# Docker Deployment Guide

> Canonical deployment path for latticelm: Docker Compose with gateway + PostgreSQL + Redis.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Docker Compose Operations](#docker-compose-operations)
- [Production Considerations](#production-considerations)
- [Troubleshooting](#troubleshooting)

## Quick Start

Start the supported local stack:

```bash
# 1) Create environment file
cp .env.example .env

# 2) Set required values in .env
# - ENCRYPTION_KEY=... (generate with: openssl rand -base64 32)
# - DATABASE_URL=postgres://latticelm:latticelm@postgres:5432/latticelm?sslmode=disable
# - RATE_LIMIT_REDIS_URL=redis://redis:6379/0

# 3) Start gateway + postgres + redis
docker compose up -d --build

# 4) Verify gateway health
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Configuration

### Environment Variables

Create a `.env` file from the project template:

```bash
# Required
DATABASE_URL=postgres://latticelm:latticelm@postgres:5432/latticelm?sslmode=disable
ENCRYPTION_KEY=  # generate with: openssl rand -base64 32
RATE_LIMIT_REDIS_URL=redis://redis:6379/0

# Provider API keys (add via Admin UI after first start)
# Or set environment variables for auto-detection

# Optional
SERVER_ADDRESS=:8080
LOG_FORMAT=json
LOG_LEVEL=info
UI_ENABLED=true
```

The `docker-compose.yaml` file wires `gateway` to the built-in `postgres` and `redis` services.

### Persistent Storage

All persisted state is stored in PostgreSQL via `DATABASE_URL`. Redis is required for distributed rate limiting.

## Docker Compose Operations

The project includes a production-ready `docker-compose.yaml` file.

### Basic Setup

```bash
# Create .env file from template
cp .env.example .env

# Start gateway + postgres + redis
docker compose up -d --build

# Check status
docker compose ps

# View logs
docker compose logs -f gateway
```

### With Monitoring

Enable Prometheus and Grafana:

```bash
docker compose --profile monitoring up -d
```

Access services:
- Gateway: http://localhost:8080
- Admin UI: http://localhost:8080/admin/
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

### Managing Services

```bash
# Stop all services
docker compose down

# Stop and remove volumes (deletes data!)
docker compose down -v

# Restart specific service
docker compose restart gateway

# View logs
docker compose logs -f gateway

# Update to latest image
docker compose pull
docker compose up -d
```

## Production Considerations

### Security

**Use secrets management:**
```bash
# Docker secrets (Swarm)
echo "sk-your-key" | docker secret create openai_key -

docker service create \
  --name llm-gateway \
  --secret openai_key \
  -e OPENAI_API_KEY_FILE=/run/secrets/openai_key \
  ghcr.io/yourusername/llm-gateway:latest
```

**Run as non-root:**
The image already runs as UID 1000 (non-root) by default.

**Read-only filesystem:** configure read-only root filesystems and tmpfs in your Compose or orchestrator runtime policy.

### Resource Limits

Set memory and CPU limits in your Compose deployment or runtime environment.

### Health Checks

The image includes built-in health checks:

```bash
# Check health status
docker inspect --format='{{.State.Health.Status}}' llm-gateway

# Manual health check
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Logging

Configure structured JSON logging via `.env` (`LOG_FORMAT=json`, `LOG_LEVEL=info`) and your container runtime log driver.

### Networking

Use the default `llm-network` bridge created by `docker compose` unless your environment requires custom networking.

## Troubleshooting

### Container Won't Start

Check logs:
```bash
docker logs llm-gateway
docker logs --tail 50 llm-gateway
```

Common issues:
- Missing required API keys
- Port 8080 already in use (use `-p 9000:8080`)
- Invalid configuration file syntax

### High Memory Usage

Monitor resources:
```bash
docker stats llm-gateway
```

Set limits:
```bash
docker update --memory="512m" llm-gateway
```

### Connection Issues

**Test from inside container:**
```bash
docker compose exec gateway wget -O- http://localhost:8080/health
```

**Check port bindings:**
```bash
docker compose port gateway 8080
```

**Test provider connectivity:**
```bash
docker compose exec gateway wget -O- https://api.openai.com
```

### Image Pull Failures

**Authentication:**
```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Pull image
docker pull ghcr.io/yourusername/llm-gateway:latest
```

**Rate limiting:**
Images are public but may be rate-limited. Use Docker Hub mirror or cache.

### Debugging

**Interactive shell:**
```bash
docker compose exec gateway sh
```

**Inspect configuration:**
```bash
# Check environment variables
docker compose exec gateway env

# Check config file
docker compose exec gateway env | grep -E 'DATABASE_URL|RATE_LIMIT_REDIS_URL|ENCRYPTION_KEY'
```

**Network debugging:**
```bash
docker compose exec gateway wget --spider http://localhost:8080/health
docker compose exec gateway ping google.com
```

## Useful Commands

```bash
# Container lifecycle
docker compose stop
docker compose start
docker compose restart gateway
docker compose down -v

# Logs
docker compose logs -f gateway
docker compose logs --tail=100 gateway

# Cleanup
docker compose down -v
docker system prune -a

# Updates
docker compose pull
docker compose up -d --build
```

## Next Steps

- **Production deployment**: See [Kubernetes guide](../k8s/README.md) for orchestration
- **Monitoring**: Enable Prometheus metrics and set up Grafana dashboards
- **Security**: Configure OAuth2/OIDC authentication
- **Scaling**: Use Kubernetes HPA for auto-scaling in production

## Additional Resources

- [Main README](../README.md) - Full documentation
- [Kubernetes Deployment](../k8s/README.md) - Production orchestration
- [Configuration Reference](../config.example.yaml) - All config options
- [GitHub Container Registry](https://github.com/yourusername/latticelm/pkgs/container/llm-gateway) - Published images
