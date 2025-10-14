# ------------------------------------------------------------------------------
# Makefile - Health Check Service
#
# Systemd service health checker with Prometheus metrics and graceful shutdown.
# Single Makefile for building, testing, and running the service.
# ------------------------------------------------------------------------------

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

# Binary name
BINARY_NAME := health-checker

# Build directory
BUILD_DIR := bin

# Main package location
MAIN_PATH := ./cmd/health-checker/main.go

# Docker image name and tag
DOCKER_IMAGE := health-checker
DOCKER_TAG ?= latest

# Container registry
REGISTRY_HOST ?= docker-mirror.service.consul
REGISTRY_PORT ?= 5000
REGISTRY ?= $(REGISTRY_HOST):$(REGISTRY_PORT)
# Full image name with registry
FULL_IMAGE := $(REGISTRY)/$(DOCKER_IMAGE)

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt

# ANSI color codes
COLOR_RESET   := \033[0m
COLOR_RED     := \033[0;31m
COLOR_GREEN   := \033[0;32m
COLOR_YELLOW  := \033[0;33m
COLOR_BLUE    := \033[0;34m
COLOR_CYAN    := \033[0;36m

.PHONY: all build run run-env run-config clean clean-all deps init \
        test fmt lint lint-fix lint-verbose install-golangci-lint install-gotestsum \
        install-checkov install-trivy \
        docker-build docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image \
        docker-scan docker-tag docker-push docker-run docker-compose-up docker-compose-down docker-clean \
        generate-cert run-tls docker-run-tls clean-certs \
        pull_request merge help

# ------------------------------------------------------------------------------
# Default Target
# ------------------------------------------------------------------------------

# Default target
all: deps build

# ------------------------------------------------------------------------------
# Setup & Dependency Targets
# ------------------------------------------------------------------------------

# Fetch and tidy dependencies
deps:
	@echo "$(COLOR_CYAN)==> Fetching Go dependencies...$(COLOR_RESET)"
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Dependencies updated"

# Initialize project (first-time setup)
init: deps install-golangci-lint install-gotestsum
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Project initialized and ready to build"

# Install golangci-lint if not present
install-golangci-lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing golangci-lint...$(COLOR_RESET)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) golangci-lint installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) golangci-lint already installed"; \
	fi

# Install gotestsum for better test output
install-gotestsum:
	@if ! command -v gotestsum >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing gotestsum...$(COLOR_RESET)"; \
		go install gotest.tools/gotestsum@latest; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) gotestsum installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) gotestsum already installed"; \
	fi

# Install checkov if not present
install-checkov:
	@if ! command -v checkov >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing Checkov...$(COLOR_RESET)"; \
		pip3 install checkov || pip install checkov; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov already installed"; \
	fi

# Install trivy if not present
install-trivy:
	@if ! command -v trivy >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing Trivy...$(COLOR_RESET)"; \
		curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy already installed"; \
	fi

# ------------------------------------------------------------------------------
# Build Targets
# ------------------------------------------------------------------------------

# Build the binary
build: deps
	@echo "$(COLOR_CYAN)==> Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# ------------------------------------------------------------------------------
# Run Targets
# ------------------------------------------------------------------------------

# Build and run with default flags (customize SERVICE, PORT, INTERVAL as needed)
run: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME)...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Set SERVICE=<name> to monitor a different service$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

# Run with environment variables
run-env: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with environment variables...$(COLOR_RESET)"
	HEALTH_SERVICE=$${SERVICE:-nginx} \
	HEALTH_PORT=$${PORT:-8181} \
	HEALTH_INTERVAL=$${INTERVAL:-7} \
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run with config file
run-config: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with config file...$(COLOR_RESET)"
	@if [ ! -f "config.yaml" ]; then \
		echo "$(COLOR_YELLOW)[WARN]$(COLOR_RESET) config.yaml not found, creating example..."; \
		echo "port: 8080" > config.yaml; \
		echo "service: nginx" >> config.yaml; \
		echo "interval: 10" >> config.yaml; \
	fi
	./$(BUILD_DIR)/$(BINARY_NAME) --config config.yaml

# ------------------------------------------------------------------------------
# Development Targets
# ------------------------------------------------------------------------------

# Run Go tests with better output
test: install-gotestsum
	@echo "$(COLOR_CYAN)==> Running Go tests...$(COLOR_RESET)"
	@gotestsum --format testname ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Tests passed"

# Format Go code
fmt:
	@echo "$(COLOR_CYAN)==> Formatting Go code...$(COLOR_RESET)"
	@$(GOFMT) ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Code formatted"

# Run golangci-lint
lint: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint...$(COLOR_RESET)"
	@golangci-lint run ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Linting complete"

# Run golangci-lint with auto-fix
lint-fix: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint with auto-fix...$(COLOR_RESET)"
	@golangci-lint run --fix ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Linting with fixes complete"

# Show linting issues in verbose mode
lint-verbose: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint (verbose)...$(COLOR_RESET)"
	@golangci-lint run -v ./...

# ------------------------------------------------------------------------------
# TLS/HTTPS Targets
# ------------------------------------------------------------------------------

# Generate self-signed certificate for testing
generate-cert:
	@echo "$(COLOR_CYAN)==> Generating self-signed certificate...$(COLOR_RESET)"
	@mkdir -p certs
	@openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt \
		-days 365 -nodes -subj "/CN=localhost" 2>/dev/null
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Certificate generated: certs/server.crt"
	@echo "$(COLOR_YELLOW)Note: This is a self-signed certificate for testing only$(COLOR_RESET)"

# Run with TLS enabled
run-tls: build generate-cert
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with TLS enabled...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Using self-signed certificate$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8443} \
		--interval $${INTERVAL:-10} \
		--tls_enabled \
		--tls_cert certs/server.crt \
		--tls_key certs/server.key

# Run with Let's Encrypt autocert (host)
run-autocert: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with Let's Encrypt autocert...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires ports 80 and 443 on this host and public DNS$(COLOR_RESET)"
	@if [ -z "$${HEALTH_TLS_AUTOCERT_DOMAIN}" ]; then \
		echo "$(COLOR_RED)[ERR]$(COLOR_RESET) Set HEALTH_TLS_AUTOCERT_DOMAIN=alexfreidah.com"; exit 1; \
	fi
	# Use 443 per your validation; 80 is used by the ACME HTTP challenge handler
	HEALTH_SERVICE=$${HEALTH_SERVICE:-nginx} \
	HEALTH_PORT=443 \
	HEALTH_INTERVAL=$${HEALTH_INTERVAL:-10} \
	HEALTH_TLS_AUTOCERT=true \
	HEALTH_TLS_AUTOCERT_DOMAIN=$${HEALTH_TLS_AUTOCERT_DOMAIN} \
	HEALTH_TLS_AUTOCERT_CACHE=$${HEALTH_TLS_AUTOCERT_CACHE:-./acme-cache} \
	HEALTH_TLS_AUTOCERT_EMAIL=$${HEALTH_TLS_AUTOCERT_EMAIL:-} \
	./$(BUILD_DIR)/$(BINARY_NAME)

# Docker run with TLS
docker-run-tls: docker-build generate-cert
	@echo "$(COLOR_CYAN)==> Running Docker container with TLS...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		-v $(PWD)/certs:/app/certs:ro \
		--network host \
		$(FULL_IMAGE):latest \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8443} \
		--interval $${INTERVAL:-10} \
		--tls_enabled \
		--tls_cert /app/certs/server.crt \
		--tls_key /app/certs/server.key

# Docker run with Let's Encrypt autocert
docker-run-autocert: docker-build
	@echo "$(COLOR_CYAN)==> Running Docker container with Let's Encrypt autocert...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires public DNS -> host IP, and ports 80/443 free$(COLOR_RESET)"
	@if [ -z "$${HEALTH_TLS_AUTOCERT_DOMAIN}" ]; then \
		echo "$(COLOR_RED)[ERR]$(COLOR_RESET) Set HEALTH_TLS_AUTOCERT_DOMAIN=your.domain.tld"; exit 1; \
	fi
	mkdir -p $${HEALTH_TLS_AUTOCERT_CACHE:-$(PWD)/acme-cache}
	docker run --rm \
		--network host \
		--cap-add=NET_BIND_SERVICE \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		-v $${HEALTH_TLS_AUTOCERT_CACHE:-$(PWD)/acme-cache}:/var/cache/health-checker \
		-e HEALTH_SERVICE=$${HEALTH_SERVICE:-nginx} \
		-e HEALTH_PORT=443 \
		-e HEALTH_INTERVAL=$${HEALTH_INTERVAL:-10} \
		-e HEALTH_TLS_AUTOCERT=true \
		-e HEALTH_TLS_AUTOCERT_DOMAIN=$${HEALTH_TLS_AUTOCERT_DOMAIN} \
		-e HEALTH_TLS_AUTOCERT_CACHE=/var/cache/health-checker \
		-e HEALTH_TLS_AUTOCERT_EMAIL=$${HEALTH_TLS_AUTOCERT_EMAIL:-} \
		$(FULL_IMAGE):latest

# Clean certificates
clean-certs:
	@echo "$(COLOR_CYAN)==> Removing certificates...$(COLOR_RESET)"
	@rm -rf certs
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Certificates removed"

# ------------------------------------------------------------------------------
# Docker Targets
# ------------------------------------------------------------------------------

# Build Docker image
docker-build:
	@echo "$(COLOR_CYAN)==> Building Docker image...$(COLOR_RESET)"
	docker build -t $(FULL_IMAGE):latest .
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Docker image built: $(FULL_IMAGE):latest"

# Scan Dockerfile with Checkov
docker-scan-checkov: install-checkov
	@echo "$(COLOR_CYAN)==> Scanning Dockerfile with Checkov...$(COLOR_RESET)"
	checkov -f Dockerfile
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov scan complete"

# Scan Dockerfile with Trivy (config scan)
docker-scan-trivy-config: install-trivy
	@echo "$(COLOR_CYAN)==> Scanning Dockerfile with Trivy (config)...$(COLOR_RESET)"
	trivy config --quiet --file-patterns "dockerfile:Dockerfile" .
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy config scan complete"

# Scan built image with Trivy (fail on CRITICAL)
docker-scan-trivy-image: docker-build install-trivy
	@echo "$(COLOR_CYAN)==> Scanning Docker image with Trivy (CRITICAL)...$(COLOR_RESET)"
	trivy image --quiet --severity CRITICAL $(FULL_IMAGE):latest
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy image scan complete"

# Run all Docker security scans (PR pipeline)
docker-scan: docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) All security scans complete"

# Tag latest image with release tag (for merge)
docker-tag:
	@echo "$(COLOR_CYAN)==> Tagging image :latest -> :$(DOCKER_TAG)...$(COLOR_RESET)"
	docker tag $(FULL_IMAGE):latest $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Tagged: $(FULL_IMAGE):$(DOCKER_TAG)"

# Push tagged image to registry
docker-push:
	@echo "$(COLOR_CYAN)==> Pushing $(FULL_IMAGE):$(DOCKER_TAG)...$(COLOR_RESET)"
	docker push $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Pushed: $(FULL_IMAGE):$(DOCKER_TAG)"

# Run Docker container (requires D-Bus access)
docker-run: docker-build
	@echo "$(COLOR_CYAN)==> Running Docker container...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		--network host \
		$(FULL_IMAGE):latest \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

# Run with docker compose
docker-compose-up:
	@echo "$(COLOR_CYAN)==> Starting with docker compose...$(COLOR_RESET)"
	docker compose up --build

# Stop docker compose
docker-compose-down:
	@echo "$(COLOR_CYAN)==> Stopping docker compose...$(COLOR_RESET)"
	docker compose down

# Clean Docker artifacts
docker-clean:
	@echo "$(COLOR_CYAN)==> Cleaning Docker images...$(COLOR_RESET)"
	docker rmi $(FULL_IMAGE):latest 2>/dev/null || true
	docker rmi $(FULL_IMAGE):$(DOCKER_TAG) 2>/dev/null || true
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Docker cleanup complete"

# ------------------------------------------------------------------------------
# CI/CD Pipeline Targets
# ------------------------------------------------------------------------------

# PR pipeline: format -> lint -> test -> build -> docker scans
pull_request: fmt lint test build docker-scan
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) PR pipeline complete"

# Merge pipeline: re-run PR checks, tag, and push
merge: pull_request docker-tag docker-push
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Merge pipeline complete - image pushed to registry"

# ------------------------------------------------------------------------------
# Cleanup Targets
# ------------------------------------------------------------------------------

# Remove build artifacts
clean:
	@echo "$(COLOR_CYAN)==> Cleaning build artifacts...$(COLOR_RESET)"
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Clean complete"

# Full clean including Go cache
clean-all: clean
	@echo "$(COLOR_CYAN)==> Cleaning Go cache...$(COLOR_RESET)"
	@$(GOCMD) clean -cache -modcache
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Full clean complete"

# ------------------------------------------------------------------------------
# Help Target
# ------------------------------------------------------------------------------

# Show available targets
help:
	@echo "$(COLOR_CYAN)Health Check Service - Available Targets:$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_YELLOW)Setup & Dependencies:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)init$(COLOR_RESET)                       - Initialize project (install tools, fetch deps)"
	@echo "  $(COLOR_BLUE)deps$(COLOR_RESET)                       - Download and tidy Go dependencies"
	@echo "  $(COLOR_BLUE)install-golangci-lint$(COLOR_RESET)      - Install golangci-lint"
	@echo "  $(COLOR_BLUE)install-gotestsum$(COLOR_RESET)          - Install gotestsum"
	@echo "  $(COLOR_BLUE)install-checkov$(COLOR_RESET)            - Install Checkov"
	@echo "  $(COLOR_BLUE)install-trivy$(COLOR_RESET)              - Install Trivy"
	@echo ""
	@echo "$(COLOR_YELLOW)Build & Run:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)all$(COLOR_RESET)                        - Fetch dependencies and build (default)"
	@echo "  $(COLOR_BLUE)build$(COLOR_RESET)                      - Build the binary"
	@echo "  $(COLOR_BLUE)run$(COLOR_RESET)                        - Build and run (use SERVICE=name PORT=8080 to customize)"
	@echo "  $(COLOR_BLUE)run-env$(COLOR_RESET)                    - Run with environment variables"
	@echo "  $(COLOR_BLUE)run-config$(COLOR_RESET)                 - Run with config file"
	@echo ""
	@echo "$(COLOR_YELLOW)Development:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)test$(COLOR_RESET)                       - Run Go tests"
	@echo "  $(COLOR_BLUE)fmt$(COLOR_RESET)                        - Format Go code"
	@echo "  $(COLOR_BLUE)lint$(COLOR_RESET)                       - Run golangci-lint"
	@echo "  $(COLOR_BLUE)lint-fix$(COLOR_RESET)                   - Run golangci-lint with auto-fix"
	@echo "  $(COLOR_BLUE)lint-verbose$(COLOR_RESET)               - Run golangci-lint (verbose output)"
	@echo ""
	@echo "$(COLOR_YELLOW)TLS/HTTPS:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)generate-cert$(COLOR_RESET)              - Generate self-signed certificate for testing"
	@echo "  $(COLOR_BLUE)run-tls$(COLOR_RESET)                    - Build and run with TLS enabled (use SERVICE=name PORT=8443)"
	@echo "  $(COLOR_BLUE)docker-run-tls$(COLOR_RESET)             - Run Docker container with TLS enabled"
	@echo "  $(COLOR_BLUE)clean-certs$(COLOR_RESET)                - Remove generated certificates"
	@echo ""
	@echo "$(COLOR_YELLOW)Docker:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)docker-build$(COLOR_RESET)               - Build Docker image"
	@echo "  $(COLOR_BLUE)docker-scan-checkov$(COLOR_RESET)        - Scan Dockerfile with Checkov"
	@echo "  $(COLOR_BLUE)docker-scan-trivy-config$(COLOR_RESET)   - Scan Dockerfile with Trivy (config)"
	@echo "  $(COLOR_BLUE)docker-scan-trivy-image$(COLOR_RESET)    - Scan image with Trivy (CRITICAL)"
	@echo "  $(COLOR_BLUE)docker-scan$(COLOR_RESET)                - Run all security scans"
	@echo "  $(COLOR_BLUE)docker-tag$(COLOR_RESET)                 - Tag image (use DOCKER_TAG=version)"
	@echo "  $(COLOR_BLUE)docker-push$(COLOR_RESET)                - Push tagged image to registry"
	@echo "  $(COLOR_BLUE)docker-run$(COLOR_RESET)                 - Run Docker container (use SERVICE=name to customize)"
	@echo "  $(COLOR_BLUE)docker-run-tls$(COLOR_RESET)             - Run Docker container with TLS"
	@echo "  $(COLOR_BLUE)docker-compose-up$(COLOR_RESET)          - Start with docker compose"
	@echo "  $(COLOR_BLUE)docker-compose-down$(COLOR_RESET)        - Stop docker compose"
	@echo "  $(COLOR_BLUE)docker-clean$(COLOR_RESET)               - Remove Docker images"
	@echo ""
	@echo "$(COLOR_YELLOW)CI/CD:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)pull_request$(COLOR_RESET)               - Run full PR pipeline (fmt, lint, test, build, scans)"
	@echo "  $(COLOR_BLUE)merge$(COLOR_RESET)                      - Run merge pipeline (PR checks + tag + push)"
	@echo ""
	@echo "$(COLOR_YELLOW)Cleanup:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)clean$(COLOR_RESET)                      - Remove build artifacts"
	@echo "  $(COLOR_BLUE)clean-all$(COLOR_RESET)                  - Remove build artifacts and Go cache"
	@echo "  $(COLOR_BLUE)clean-certs$(COLOR_RESET)                - Remove generated certificates"
	@echo ""
	@echo "$(COLOR_YELLOW)Examples:$(COLOR_RESET)"
	@echo "  make run SERVICE=postgresql PORT=9090"
	@echo "  make run-tls SERVICE=nginx PORT=8443"
	@echo "  make pull_request"
	@echo "  make merge DOCKER_TAG=v1.2.3"
	@echo "  make docker-run SERVICE=redis"
	@echo "  make docker-run-tls SERVICE=nginx"
	@echo "  make docker-compose-up"
	@echo "  make test"
	@echo "  make lint-fix"
