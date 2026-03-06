# Docker Deployment Guide

> Deploy the LLM Gateway using pre-built Docker images or build your own.

## Table of Contents

- [Quick Start](#quick-start)
- [Using Pre-Built Images](#using-pre-built-images)
- [Configuration](#configuration)
- [Docker Compose](#docker-compose)
- [Building from Source](#building-from-source)
- [Production Considerations](#production-considerations)
- [Troubleshooting](#troubleshooting)

## Quick Start

Pull and run the latest image:

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -e OPENAI_API_KEY="sk-your-key" \
  -e ANTHROPIC_API_KEY="sk-ant-your-key" \
  -e GOOGLE_API_KEY="your-key" \
  ghcr.io/yourusername/llm-gateway:latest

# Verify it's running
curl http://localhost:8080/health
```

## Using Pre-Built Images

Images are automatically built and published via GitHub Actions on every release.

### Available Tags

- `latest` - Latest stable release
- `v1.2.3` - Specific version tags
- `main` - Latest commit on main branch (unstable)
- `sha-abc1234` - Specific commit SHA

### Pull from Registry

```bash
# Pull latest stable
docker pull ghcr.io/yourusername/llm-gateway:latest

# Pull specific version
docker pull ghcr.io/yourusername/llm-gateway:v1.2.3

# List local images
docker images | grep llm-gateway
```

### Basic Usage

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  --env-file .env \
  ghcr.io/yourusername/llm-gateway:latest
```

## Configuration

### Environment Variables

Create a `.env` file with your API keys:

```bash
# Required: At least one provider
OPENAI_API_KEY=sk-your-openai-key
ANTHROPIC_API_KEY=sk-ant-your-anthropic-key
GOOGLE_API_KEY=your-google-key

# Optional: Server settings
SERVER_ADDRESS=:8080
LOGGING_LEVEL=info
LOGGING_FORMAT=json

# Optional: Features
ADMIN_ENABLED=true
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_SECOND=10
RATE_LIMIT_BURST=20

# Optional: Auth
AUTH_ENABLED=false
AUTH_ISSUER=https://accounts.google.com
AUTH_AUDIENCE=your-client-id.apps.googleusercontent.com

# Optional: Observability
OBSERVABILITY_ENABLED=false
OBSERVABILITY_METRICS_ENABLED=false
OBSERVABILITY_TRACING_ENABLED=false
```

Run with environment file:

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  --env-file .env \
  ghcr.io/yourusername/llm-gateway:latest
```

### Using Config File

For more complex configurations, use a YAML config file:

```bash
# Create config from example
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# Mount config file into container
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/yourusername/llm-gateway:latest \
  --config /app/config.yaml
```

### Persistent Storage

For persistent conversation storage with SQLite:

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -v llm-gateway-data:/app/data \
  -e OPENAI_API_KEY="your-key" \
  -e CONVERSATIONS_STORE=sql \
  -e CONVERSATIONS_DRIVER=sqlite3 \
  -e CONVERSATIONS_DSN=/app/data/conversations.db \
  ghcr.io/yourusername/llm-gateway:latest
```

## Docker Compose

The project includes a production-ready `docker-compose.yaml` file.

### Basic Setup

```bash
# Create .env file with API keys
cat > .env <<EOF
GOOGLE_API_KEY=your-google-key
ANTHROPIC_API_KEY=sk-ant-your-key
OPENAI_API_KEY=sk-your-key
EOF

# Start gateway + Redis
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f gateway
```

### With Monitoring

Enable Prometheus and Grafana:

```bash
docker-compose --profile monitoring up -d
```

Access services:
- Gateway: http://localhost:8080
- Admin UI: http://localhost:8080/admin/
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

### Managing Services

```bash
# Stop all services
docker-compose down

# Stop and remove volumes (deletes data!)
docker-compose down -v

# Restart specific service
docker-compose restart gateway

# View logs
docker-compose logs -f gateway

# Update to latest image
docker-compose pull
docker-compose up -d
```

## Building from Source

If you need to build your own image:

```bash
# Clone repository
git clone https://github.com/yourusername/latticelm.git
cd latticelm

# Build image (includes frontend automatically)
docker build -t llm-gateway:local .

# Run your build
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  --env-file .env \
  llm-gateway:local
```

### Multi-Platform Builds

Build for multiple architectures:

```bash
# Setup buildx
docker buildx create --use

# Build and push multi-platform
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/yourusername/llm-gateway:latest \
  --push .
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

**Read-only filesystem:**
```bash
docker run -d \
  --name llm-gateway \
  --read-only \
  --tmpfs /tmp \
  -v llm-gateway-data:/app/data \
  -p 8080:8080 \
  --env-file .env \
  ghcr.io/yourusername/llm-gateway:latest
```

### Resource Limits

Set memory and CPU limits:

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  --memory="512m" \
  --cpus="1.0" \
  --env-file .env \
  ghcr.io/yourusername/llm-gateway:latest
```

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

Configure structured JSON logging:

```bash
docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -e LOGGING_FORMAT=json \
  -e LOGGING_LEVEL=info \
  --log-driver=json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  ghcr.io/yourusername/llm-gateway:latest
```

### Networking

**Custom network:**
```bash
# Create network
docker network create llm-network

# Run gateway on network
docker run -d \
  --name llm-gateway \
  --network llm-network \
  -p 8080:8080 \
  --env-file .env \
  ghcr.io/yourusername/llm-gateway:latest

# Run Redis on same network
docker run -d \
  --name redis \
  --network llm-network \
  redis:7-alpine
```

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
docker exec -it llm-gateway wget -O- http://localhost:8080/health
```

**Check port bindings:**
```bash
docker port llm-gateway
```

**Test provider connectivity:**
```bash
docker exec llm-gateway wget -O- https://api.openai.com
```

### Database Locked (SQLite)

If using SQLite with multiple containers:
```bash
# SQLite doesn't support concurrent writes
# Use Redis or PostgreSQL instead:

docker run -d \
  --name redis \
  redis:7-alpine

docker run -d \
  --name llm-gateway \
  -p 8080:8080 \
  -e CONVERSATIONS_STORE=redis \
  -e CONVERSATIONS_DSN=redis://redis:6379/0 \
  --link redis \
  ghcr.io/yourusername/llm-gateway:latest
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
docker exec -it llm-gateway sh
```

**Inspect configuration:**
```bash
# Check environment variables
docker exec llm-gateway env

# Check config file
docker exec llm-gateway cat /app/config.yaml
```

**Network debugging:**
```bash
docker exec llm-gateway wget --spider http://localhost:8080/health
docker exec llm-gateway ping google.com
```

## Useful Commands

```bash
# Container lifecycle
docker stop llm-gateway
docker start llm-gateway
docker restart llm-gateway
docker rm -f llm-gateway

# Logs
docker logs -f llm-gateway
docker logs --tail 100 llm-gateway
docker logs --since 30m llm-gateway

# Cleanup
docker system prune -a
docker volume prune
docker image prune -a

# Updates
docker pull ghcr.io/yourusername/llm-gateway:latest
docker stop llm-gateway
docker rm llm-gateway
docker run -d --name llm-gateway ... ghcr.io/yourusername/llm-gateway:latest
```

## Next Steps

- **Production deployment**: See [Kubernetes guide](../k8s/README.md) for orchestration
- **Monitoring**: Enable Prometheus metrics and set up Grafana dashboards
- **Security**: Configure OAuth2/OIDC authentication
- **Scaling**: Use Kubernetes HPA or Docker Swarm for auto-scaling

## Additional Resources

- [Main README](../README.md) - Full documentation
- [Kubernetes Deployment](../k8s/README.md) - Production orchestration
- [Configuration Reference](../config.example.yaml) - All config options
- [GitHub Container Registry](https://github.com/yourusername/latticelm/pkgs/container/llm-gateway) - Published images
