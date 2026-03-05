# Makefile for LLM Gateway

.PHONY: help build test docker-build docker-push k8s-deploy k8s-delete clean

# Variables
APP_NAME := llm-gateway
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY ?= your-registry
IMAGE := $(REGISTRY)/$(APP_NAME)
DOCKER_TAG := $(IMAGE):$(VERSION)
LATEST_TAG := $(IMAGE):latest

# Go variables
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt

# Build directory
BUILD_DIR := bin

# Help target
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Frontend targets
frontend-install: ## Install frontend dependencies
	@echo "Installing frontend dependencies..."
	cd frontend/admin && npm install

frontend-build: ## Build frontend
	@echo "Building frontend..."
	cd frontend/admin && npm run build
	rm -rf internal/admin/dist
	cp -r frontend/admin/dist internal/admin/

frontend-dev: ## Run frontend dev server
	cd frontend/admin && npm run dev

# Development targets
build: ## Build the binary
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=1 $(GOBUILD) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/gateway

build-all: frontend-build build ## Build frontend and backend

build-static: ## Build static binary
	@echo "Building static binary..."
	CGO_ENABLED=1 $(GOBUILD) -ldflags='-w -s -extldflags "-static"' -a -installsuffix cgo -o $(BUILD_DIR)/$(APP_NAME) ./cmd/gateway

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report saved to coverage.html"

fmt: ## Format Go code
	@echo "Formatting code..."
	$(GOFMT) ./...

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	$(GOMOD) tidy

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -rf internal/admin/dist
	rm -rf frontend/admin/dist
	rm -f coverage.out coverage.html

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_TAG)..."
	docker build -t $(DOCKER_TAG) -t $(LATEST_TAG) .

docker-push: docker-build ## Push Docker image to registry
	@echo "Pushing Docker image..."
	docker push $(DOCKER_TAG)
	docker push $(LATEST_TAG)

docker-run: ## Run Docker container locally
	@echo "Running Docker container..."
	docker run --rm -p 8080:8080 \
		-e GOOGLE_API_KEY="$(GOOGLE_API_KEY)" \
		-e ANTHROPIC_API_KEY="$(ANTHROPIC_API_KEY)" \
		-e OPENAI_API_KEY="$(OPENAI_API_KEY)" \
		-v $(PWD)/config.yaml:/app/config/config.yaml:ro \
		$(DOCKER_TAG)

docker-compose-up: ## Start services with docker-compose
	@echo "Starting services with docker-compose..."
	docker-compose up -d

docker-compose-down: ## Stop services with docker-compose
	@echo "Stopping services with docker-compose..."
	docker-compose down

docker-compose-logs: ## View docker-compose logs
	docker-compose logs -f

# Kubernetes targets
k8s-namespace: ## Create Kubernetes namespace
	kubectl create namespace llm-gateway --dry-run=client -o yaml | kubectl apply -f -

k8s-secrets: ## Create Kubernetes secrets (requires env vars)
	@echo "Creating secrets..."
	@if [ -z "$(GOOGLE_API_KEY)" ] || [ -z "$(ANTHROPIC_API_KEY)" ] || [ -z "$(OPENAI_API_KEY)" ]; then \
		echo "Error: Please set GOOGLE_API_KEY, ANTHROPIC_API_KEY, and OPENAI_API_KEY environment variables"; \
		exit 1; \
	fi
	kubectl create secret generic llm-gateway-secrets \
		--from-literal=GOOGLE_API_KEY="$(GOOGLE_API_KEY)" \
		--from-literal=ANTHROPIC_API_KEY="$(ANTHROPIC_API_KEY)" \
		--from-literal=OPENAI_API_KEY="$(OPENAI_API_KEY)" \
		--from-literal=OIDC_AUDIENCE="$(OIDC_AUDIENCE)" \
		-n llm-gateway \
		--dry-run=client -o yaml | kubectl apply -f -

k8s-deploy: k8s-namespace k8s-secrets ## Deploy to Kubernetes
	@echo "Deploying to Kubernetes..."
	kubectl apply -k k8s/

k8s-delete: ## Delete from Kubernetes
	@echo "Deleting from Kubernetes..."
	kubectl delete -k k8s/

k8s-status: ## Check Kubernetes deployment status
	@echo "Checking deployment status..."
	kubectl get all -n llm-gateway

k8s-logs: ## View Kubernetes logs
	kubectl logs -n llm-gateway -l app=llm-gateway --tail=100 -f

k8s-describe: ## Describe Kubernetes deployment
	kubectl describe deployment llm-gateway -n llm-gateway

k8s-port-forward: ## Port forward to local machine
	kubectl port-forward -n llm-gateway svc/llm-gateway 8080:80

# CI/CD targets
ci: lint test ## Run CI checks

security-scan: ## Run security scan
	@echo "Running security scan..."
	@which gosec > /dev/null || (echo "gosec not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest" && exit 1)
	gosec ./...

# Run target
run: ## Run locally
	@echo "Running $(APP_NAME) locally..."
	$(GOCMD) run ./cmd/gateway --config config.yaml

# Version info
version: ## Show version
	@echo "Version: $(VERSION)"
	@echo "Image: $(DOCKER_TAG)"
