# SID64 Quest - Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
TELNET_GATEWAY_BINARY=telnet-gateway
AUTH_SERVICE_BINARY=auth-service
GAME_ENGINE_BINARY=game-engine
WORLD_BUILDER_BINARY=world-builder
ADMIN_CONSOLE_BINARY=admin-console

# Build directory
BUILD_DIR=build

# Docker parameters
DOCKER_REGISTRY=localhost:5000
DOCKER_TAG=latest
COMPOSE ?= docker compose

.PHONY: all build clean test deps docker-build docker-push help

# Default target
all: clean deps test build

# Build all services
build: build-telnet-gateway build-auth-service build-core build-edge build-core-switch

build-core:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/game-core ./cmd/game-core

build-edge:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/terminal-edge ./cmd/terminal-edge

build-core-switch:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/core-switch ./cmd/core-switch

# Local live-service tests create and drop their own disposable databases.
test-core-restart: build-core build-edge build-core-switch
	@set -a; . ./.env; set +a; RCK_CORE_INTEGRATION=1 RCK_INTEGRATION_BIN_DIR="$(CURDIR)/$(BUILD_DIR)" $(GOTEST) -race -count=1 -v ./internal/telnet -run TestCoreIntegration

# Build individual services
build-telnet-gateway:
	@echo "Building telnet gateway..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(TELNET_GATEWAY_BINARY) ./cmd/telnet-gateway

build-auth-service:
	@echo "Building auth service..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(AUTH_SERVICE_BINARY) ./cmd/auth-service

build-game-engine:
	@echo "Building game engine..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(GAME_ENGINE_BINARY) ./cmd/game-engine

build-world-builder:
	@echo "Building world builder..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(WORLD_BUILDER_BINARY) ./cmd/world-builder

build-admin-console:
	@echo "Building admin console..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(ADMIN_CONSOLE_BINARY) ./cmd/admin-console

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@./scripts/dev-setup.sh

# Database setup
db-setup:
	@echo "Setting up database..."
	@./scripts/db-setup.sh

# Database migration
db-migrate:
	@echo "Running database migrations..."
	@./scripts/db-migrate.sh

# Start development services
dev-start:
	@echo "Starting development services..."
	@$(COMPOSE) -f docker-compose.simple.yml up -d

# Start full development services (with monitoring)
dev-start-full:
	@echo "Starting full development services..."
	@$(COMPOSE) -f docker-compose.dev.yml up -d

# Stop development services
dev-stop:
	@echo "Stopping development services..."
	@$(COMPOSE) -f docker-compose.simple.yml down

# Stop full development services
dev-stop-full:
	@echo "Stopping full development services..."
	@$(COMPOSE) -f docker-compose.dev.yml down

# View development logs
dev-logs:
	@$(COMPOSE) -f docker-compose.simple.yml logs -f

# View full development logs
dev-logs-full:
	@$(COMPOSE) -f docker-compose.dev.yml logs -f

# Run telnet gateway locally
run-telnet-gateway: build-telnet-gateway
	@echo "Starting telnet gateway..."
	@set -a; . ./.env; set +a; exec ./$(BUILD_DIR)/$(TELNET_GATEWAY_BINARY)

# Run auth service locally
run-auth-service: build-auth-service
	@echo "Starting auth service..."
	@set -a; . ./.env; set +a; exec ./$(BUILD_DIR)/$(AUTH_SERVICE_BINARY)

# Docker builds
docker-build: docker-build-telnet-gateway docker-build-auth-service

docker-build-telnet-gateway:
	@echo "Building telnet gateway Docker image..."
	docker build -f docker/Dockerfile.telnet-gateway -t $(DOCKER_REGISTRY)/race-condition-kingdom/telnet-gateway:$(DOCKER_TAG) .

docker-build-auth-service:
	@echo "Building auth service Docker image..."
	docker build -f docker/Dockerfile.auth-service -t $(DOCKER_REGISTRY)/race-condition-kingdom/auth-service:$(DOCKER_TAG) .

docker-build-game-engine:
	@echo "Building game engine Docker image..."
	docker build -f docker/Dockerfile.game-engine -t $(DOCKER_REGISTRY)/race-condition-kingdom/game-engine:$(DOCKER_TAG) .

# Docker push
docker-push:
	@echo "Pushing Docker images..."
	docker push $(DOCKER_REGISTRY)/race-condition-kingdom/telnet-gateway:$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/race-condition-kingdom/auth-service:$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/race-condition-kingdom/game-engine:$(DOCKER_TAG)

# Kubernetes deployment
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f k8s/

# Kubernetes cleanup
k8s-clean:
	@echo "Cleaning up Kubernetes resources..."
	kubectl delete -f k8s/

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Security scan
security:
	@echo "Running security scan..."
	gosec ./...

# Generate documentation
docs:
	@echo "Generating documentation..."
	godoc -http=:6060

# Benchmark tests
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Profile application
profile-cpu:
	@echo "CPU profiling..."
	$(GOTEST) -cpuprofile=cpu.prof -bench=. ./...

profile-mem:
	@echo "Memory profiling..."
	$(GOTEST) -memprofile=mem.prof -bench=. ./...

# Load testing
load-test:
	@echo "Running load tests..."
	@./scripts/load-test.sh

# Integration tests
integration-test:
	@echo "Running integration tests..."
	@./scripts/integration-test.sh

# Help
help:
	@echo "Available targets:"
	@echo "  all              - Clean, download deps, test, and build"
	@echo "  build            - Build all services"
	@echo "  build-*          - Build specific service"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage"
	@echo "  deps             - Download dependencies"
	@echo "  deps-update      - Update dependencies"
	@echo "  dev-setup        - Setup development environment"
	@echo "  dev-start        - Start development services"
	@echo "  dev-stop         - Stop development services"
	@echo "  dev-logs         - View development logs"
	@echo "  db-setup         - Setup database"
	@echo "  db-migrate       - Run database migrations"
	@echo "  run-*            - Run specific service locally"
	@echo "  docker-build     - Build Docker images"
	@echo "  docker-push      - Push Docker images"
	@echo "  k8s-deploy       - Deploy to Kubernetes"
	@echo "  k8s-clean        - Clean Kubernetes resources"
	@echo "  fmt              - Format code"
	@echo "  lint             - Lint code"
	@echo "  security         - Run security scan"
	@echo "  docs             - Generate documentation"
	@echo "  benchmark        - Run benchmarks"
	@echo "  profile-*        - Profile application"
	@echo "  load-test        - Run load tests"
	@echo "  integration-test - Run integration tests"
	@echo "  help             - Show this help"
