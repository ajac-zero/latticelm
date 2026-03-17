# Multi-stage build for Go LLM Gateway

# Stage 1: Build the frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /frontend

# Copy package files for better caching
COPY ui/package*.json ./
RUN npm ci

# Copy frontend source and build
COPY ui/ ./
RUN npm run build

# Stage 2: Build the Go binary
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy pre-built frontend assets from stage 1
COPY --from=frontend-builder /internal/ui/dist ./internal/ui/dist

# Build the binary with optimizations
# CGO is required for SQLite support
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o gateway \
    ./cmd/gateway

# Stage 3: Create minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -D -u 1000 -G gateway gateway

# Create necessary directories
RUN mkdir -p /app /app/data && \
    chown -R gateway:gateway /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gateway /app/gateway

# Switch to non-root user
USER gateway

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/gateway"]
