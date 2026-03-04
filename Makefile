.PHONY: help build build-web build-all run run-insecure docker-build docker-up docker-down docker-logs stop-all docker-rebuild clean clean-web fmt lint mod-tidy dev dev-web frontend web demo demo-down demo-logs reset test test-short

# Include local overrides if present (git-ignored)
-include Makefile.local

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'

# ========================
# Build
# ========================

build: ## Build the Go binary
	@# Copy web/dist to cmd/nekzus/webdist for embedding
	@if [ -d web/dist ]; then \
		rm -rf cmd/nekzus/webdist; \
		cp -r web/dist cmd/nekzus/webdist; \
		echo "Copied web UI for embedding"; \
	fi
	@# Get version from git or use 'dev'
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building version: $$VERSION"; \
	go build -ldflags="-X main.version=$$VERSION" -o bin/nekzus ./cmd/nekzus

build-web: ## Build web UI
	@echo "Building web"
	cd web && npm install && npm run build
	@echo "Web built successfully"

build-all: build-web build ## Build web UI and Go binary together
	@echo "Complete build finished"

# ========================
# Development
# ========================

run: ## Run locally with TLS
	go run ./cmd/nekzus --config configs/config.yaml

run-insecure: ## Run locally without TLS (HTTP only)
	go run ./cmd/nekzus --config configs/config.yaml --insecure-http

dev: docker-rebuild docker-logs ## Quick dev: rebuild and show logs

dev-web: ## Run web UI in development mode
	cd web && npm run dev

frontend: dev-web ## Alias for dev-web (start frontend dev server)

web: dev-web ## Alias for dev-web (start frontend dev server)

# ========================
# Demo Environment
# ========================

demo: ## Start clean demo with Nekzus + example services (webapp, api)
	@echo "Starting Nekzus Demo Environment..."
	@echo ""
	@# Build web UI if not already built
	@if [ ! -d web/dist ]; then \
		echo "Building web UI..."; \
		$(MAKE) build-web; \
		echo ""; \
	fi
	@# Clean up any existing containers/networks to avoid conflicts
	@echo "Cleaning up any existing demo containers..."
	@docker compose -f demo/docker-compose.yaml down -v 2>/dev/null || true
	@echo ""
	@# Auto-detect host IP for mobile device connectivity
	$(eval HOST_IP := $(shell ifconfig | grep 'inet ' | grep -v 127.0.0.1 | grep -E '192\.168\.|10\.' | head -1 | awk '{print $$2}'))
	@if [ -n "$(HOST_IP)" ]; then \
		echo "Detected host IP: $(HOST_IP)"; \
	else \
		echo "Could not detect LAN IP, mobile pairing may not work"; \
	fi
	@echo ""
	@# Build and start containers
	@echo "Starting Nekzus and example services..."
	docker compose -f demo/docker-compose.yaml build --quiet
	NEKZUS_BASE_URL="http://$(HOST_IP):8080" docker compose -f demo/docker-compose.yaml up -d
	@sleep 15
	@echo ""
	@echo "Web UI:        http://localhost:8080"
	@echo "API:           http://localhost:8080/api/v1"
	@echo "Metrics:       http://localhost:8080/metrics"
	@echo "Health:        http://localhost:8080/healthz"
	@echo ""

demo-down: ## Stop demo environment
	@echo "Stopping..."
	docker compose -f demo/docker-compose.yaml down -v
	@echo "Demo stopped"

reset: ## Full reset: stop demo, rebuild everything, start fresh
	@echo "Full reset"
	@echo ""
	$(MAKE) demo-down
	$(MAKE) build-all
	$(MAKE) demo

demo-logs: ## View demo environment logs
	docker compose -f demo/docker-compose.yaml logs -f
# ========================
# Docker Compose
# ========================

docker-build: ## Build Docker image
	docker compose build

docker-up: ## Start containers with docker-compose
	docker compose up -d

docker-down: ## Stop and remove containers
	docker compose down

docker-logs: ## Follow docker container logs
	docker compose logs -f

docker-rebuild: ## Rebuild and restart containers
	docker compose down && docker compose up --build -d

# ========================
# Testing
# ========================

test: ## Run all tests (unit + integration) with race detector
	go test -race -v ./...

test-short: ## Run only unit tests (skip E2E) with race detector
	go test -race -v -short ./...

# ========================
# Code Quality
# ========================

fmt: ## Format Go code
	go fmt ./...

lint: ## Run golangci-lint
	golangci-lint run
	
# ========================
# Utilities
# ========================

clean: ## Clean build artifacts and stop containers
	rm -rf bin/
	docker compose down -v

clean-web: ## Clean web build artifacts
	rm -rf web/dist
	rm -rf web/node_modules
	rm -rf cmd/nekzus/webdist

stop-all: ## Stop and remove all Docker containers
	@echo "Stopping all Docker containers..."
	@docker ps -q | xargs -r docker stop || true
	@echo "Removing all Docker containers..."
	@docker ps -a -q | xargs -r docker rm || true
	@echo "All containers stopped and removed"
