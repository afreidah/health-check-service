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

# Main package
MAIN_PATH := ./cmd/health-checker/main.go

# Docker image name and tag
DOCKER_IMAGE := health-checker
DOCKER_TAG ?= latest

# Multi-arch platforms for Buildx
PLATFORMS ?= linux/amd64,linux/arm64
ARCH_AMD  := linux/amd64
ARCH_ARM  := linux/arm64

# Container registry (internal, insecure is already configured on hosts)
REGISTRY_HOST ?= docker-mirror.service.consul
REGISTRY_PORT ?= 5000
REGISTRY ?= $(REGISTRY_HOST):$(REGISTRY_PORT)

# Full image name with registry
FULL_IMAGE := $(REGISTRY)/$(DOCKER_IMAGE)

# Arch-tagged names for daemon push + manifest assembly
IMG_AMD := $(FULL_IMAGE):$(DOCKER_TAG)-amd64
IMG_ARM := $(FULL_IMAGE):$(DOCKER_TAG)-arm64
IMG_LATEST_AMD := $(FULL_IMAGE):latest-amd64
IMG_LATEST_ARM := $(FULL_IMAGE):latest-arm64

# Go commands
GOCMD   := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST  := $(GOCMD) test
GOMOD   := $(GOCMD) mod
GOFMT   := $(GOCMD) fmt
GOVET   := $(GOCMD) vet

# Build reproducibility / metadata
GOFLAGS ?= -mod=readonly
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# ANSI color codes
COLOR_RESET := \033[0m
COLOR_INFO  := \033[0;36m
COLOR_OK    := \033[0;32m
COLOR_WARN  := \033[0;33m

.PHONY: all build run run-env run-config clean clean-all deps init \
        test fmt lint lint-fix lint-verbose vet vuln \
        docker-build buildx-ensure docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image \
        docker-scan docker-tag docker-push docker-push-latest docker-run \
        docker-clean docker-release \
        generate-cert run-tls docker-run-tls clean-certs run-autocert docker-run-autocert \
        pull_request merge help

# ------------------------------------------------------------------------------
# Default Target
# ------------------------------------------------------------------------------

all: deps build

# ------------------------------------------------------------------------------
# Setup & Dependency Targets
# ------------------------------------------------------------------------------

deps:
	@echo "$(COLOR_INFO)==> Fetching Go dependencies...$(COLOR_RESET)"
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Dependencies updated"

init: deps
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Project initialized and ready to build"

# ------------------------------------------------------------------------------
# Build Targets
# ------------------------------------------------------------------------------

build: deps
	@echo "$(COLOR_INFO)==> Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# ------------------------------------------------------------------------------
# Run Targets
# ------------------------------------------------------------------------------

run: build
	@echo "$(COLOR_INFO)==> Running $(BINARY_NAME)...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Set SERVICE=<name> to monitor a different service$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) \
		--service $${SERVICE:-cron} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

run-env: build
	@echo "$(COLOR_INFO)==> Running $(BINARY_NAME) with environment variables...$(COLOR_RESET)"
	HEALTH_SERVICE=$${SERVICE:-cron} \
	HEALTH_PORT=$${PORT:-8181} \
	HEALTH_INTERVAL=$${INTERVAL:-7} \
	./$(BUILD_DIR)/$(BINARY_NAME)

run-config: build
	@echo "$(COLOR_INFO)==> Running $(BINARY_NAME) with config file...$(COLOR_RESET)"
	@if [ ! -f "config.yaml" ]; then \
		echo "$(COLOR_WARN)[WARN]$(COLOR_RESET) config.yaml not found, creating example..."; \
		echo "port: 8080" > config.yaml; \
		echo "service: cron" >> config.yaml; \
		echo "interval: 10" >> config.yaml; \
	fi
	./$(BUILD_DIR)/$(BINARY_NAME) --config config.yaml

# ------------------------------------------------------------------------------
# Development Targets
# ------------------------------------------------------------------------------

test:
	@echo "$(COLOR_INFO)==> Running Go tests...$(COLOR_RESET)"
	@$(GOTEST) ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Tests passed"

fmt:
	@echo "$(COLOR_INFO)==> Formatting Go code...$(COLOR_RESET)"
	@$(GOFMT) ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Code formatted"

vet:
	@echo "$(COLOR_INFO)==> go vet...$(COLOR_RESET)"
	@$(GOVET) ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Vet complete"

lint:
	@echo "$(COLOR_INFO)==> Running golangci-lint...$(COLOR_RESET)"
	@golangci-lint run ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Linting complete"

lint-fix:
	@echo "$(COLOR_INFO)==> Running golangci-lint with auto-fix...$(COLOR_RESET)"
	@golangci-lint run --fix ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Linting with fixes complete"

lint-verbose:
	@echo "$(COLOR_INFO)==> Running golangci-lint (verbose)...$(COLOR_RESET)"
	@golangci-lint run -v ./...

vuln:
	@echo "$(COLOR_INFO)==> govulncheck...$(COLOR_RESET)"
	@govulncheck ./...
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Vulnerability check complete"

# ------------------------------------------------------------------------------
# TLS/HTTPS Targets
# ------------------------------------------------------------------------------

generate-cert:
	@echo "$(COLOR_INFO)==> Generating self-signed certificate...$(COLOR_RESET)"
	@mkdir -p certs
	@openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt \
		-days 365 -nodes -subj "/CN=localhost" 2>/dev/null
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Certificate generated: certs/server.crt"
	@echo "$(COLOR_WARN)Note: This is a self-signed certificate for testing only$(COLOR_RESET)"

run-tls: build generate-cert
	@echo "$(COLOR_INFO)==> Running $(BINARY_NAME) with TLS enabled...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Using self-signed certificate$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) \
		--service $${SERVICE:-cron} \
		--port $${PORT:-8443} \
		--interval $${INTERVAL:-10} \
		--tls_enabled \
		--tls_cert certs/server.crt \
		--tls_key certs/server.key

run-autocert: build
	@echo "$(COLOR_INFO)==> Running $(BINARY_NAME) with Let's Encrypt autocert...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Requires ports 80 and 443 on this host and public DNS$(COLOR_RESET)"
	@if [ -z "$${HEALTH_TLS_AUTOCERT_DOMAIN}" ]; then \
		echo "$(COLOR_WARN)[ERR]$(COLOR_RESET) Set HEALTH_TLS_AUTOCERT_DOMAIN=alexfreidah.com"; exit 1; \
	fi
	HEALTH_SERVICE=$${HEALTH_SERVICE:-cron} \
	HEALTH_PORT=443 \
	HEALTH_INTERVAL=$${HEALTH_INTERVAL:-10} \
	HEALTH_TLS_AUTOCERT=true \
	HEALTH_TLS_AUTOCERT_DOMAIN=$${HEALTH_TLS_AUTOCERT_DOMAIN} \
	HEALTH_TLS_AUTOCERT_CACHE=$${HEALTH_TLS_AUTOCERT_CACHE:-./acme-cache} \
	HEALTH_TLS_AUTOCERT_EMAIL=$${HEALTH_TLS_AUTOCERT_EMAIL:-} \
	./$(BUILD_DIR)/$(BINARY_NAME)

docker-run-tls: docker-build generate-cert
	@echo "$(COLOR_INFO)==> Running Docker container with TLS...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		-v $(PWD)/certs:/app/certs:ro \
		--network host \
		$(FULL_IMAGE):$(DOCKER_TAG) \
		--service $${SERVICE:-cron} \
		--port $${PORT:-8443} \
		--interval $${INTERVAL:-10} \
		--tls_enabled \
		--tls_cert /app/certs/server.crt \
		--tls_key /app/certs/server.key

docker-run-autocert: docker-build
	@echo "$(COLOR_INFO)==> Running Docker container with Let's Encrypt autocert...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Requires public DNS -> host IP, and ports 80/443 free$(COLOR_RESET)"
	@if [ -z "$${HEALTH_TLS_AUTOCERT_DOMAIN}" ]; then \
		echo "$(COLOR_WARN)[ERR]$(COLOR_RESET) Set HEALTH_TLS_AUTOCERT_DOMAIN=your.domain.tld"; exit 1; \
	fi
	mkdir -p $${HEALTH_TLS_AUTOCERT_CACHE:-$(PWD)/acme-cache}
	docker run --rm \
		--network host \
		--cap-add=NET_BIND_SERVICE \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		-v $${HEALTH_TLS_AUTOCERT_CACHE:-$(PWD)/acme-cache}:/var/cache/health-checker \
		-e HEALTH_SERVICE=$${HEALTH_SERVICE:-cron} \
		-e HEALTH_PORT=443 \
		-e HEALTH_INTERVAL=$${HEALTH_INTERVAL:-10} \
		-e HEALTH_TLS_AUTOCERT=true \
		-e HEALTH_TLS_AUTOCERT_DOMAIN=$${HEALTH_TLS_AUTOCERT_DOMAIN} \
		-e HEALTH_TLS_AUTOCERT_CACHE=/var/cache/health-checker \
		-e HEALTH_TLS_AUTOCERT_EMAIL=$${HEALTH_TLS_AUTOCERT_EMAIL:-} \
		$(FULL_IMAGE):$(DOCKER_TAG)

clean-certs:
	@echo "$(COLOR_INFO)==> Removing certificates...$(COLOR_RESET)"
	@rm -rf certs
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Certificates removed"

# ------------------------------------------------------------------------------
# Docker Targets
# ------------------------------------------------------------------------------

# Build single-arch image for the current host arch (tags with $(DOCKER_TAG))
docker-build:
	@echo "$(COLOR_INFO)==> Building Docker image (single-arch)...$(COLOR_RESET)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t $(FULL_IMAGE):$(DOCKER_TAG) .
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Docker image built: $(FULL_IMAGE):$(DOCKER_TAG)"

# Ensure buildx builder is ready for multi-platform builds with insecure registry config
buildx-ensure:
	@docker buildx version >/dev/null 2>&1 || { echo "$(COLOR_WARN)[ERR]$(COLOR_RESET) Docker Buildx is not available. Please upgrade Docker."; exit 1; }
	@docker buildx inspect multiarch-builder >/dev/null 2>&1 || { \
		echo "$(COLOR_INFO)==> Creating multiarch-builder for multi-platform builds...$(COLOR_RESET)"; \
		mkdir -p .buildkit; \
		echo "[registry.\"$(REGISTRY)\"]" > .buildkit/buildkitd.toml; \
		echo "  http = true" >> .buildkit/buildkitd.toml; \
		echo "  insecure = true" >> .buildkit/buildkitd.toml; \
		docker buildx create \
			--name multiarch-builder \
			--driver docker-container \
			--driver-opt network=host \
			--config .buildkit/buildkitd.toml \
			--use; \
		docker buildx inspect --bootstrap multiarch-builder >/dev/null; \
		echo "$(COLOR_OK)[OK]$(COLOR_RESET) multiarch-builder created (network=host, HTTP insecure for $(REGISTRY))"; \
	}
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Using builder: multiarch-builder"

# Build and push multi-arch image with versioned tag and :latest
docker-release: buildx-ensure
	@echo "$(COLOR_INFO)==> Building and pushing multi-arch image...$(COLOR_RESET)"
	@echo "$(COLOR_INFO)     Platforms: $(PLATFORMS)$(COLOR_RESET)"
	@echo "$(COLOR_INFO)     Image: $(FULL_IMAGE)$(COLOR_RESET)"
	@echo "$(COLOR_INFO)     Tags: $(DOCKER_TAG), latest$(COLOR_RESET)"
	docker buildx build \
		--builder multiarch-builder \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		--tag $(FULL_IMAGE):$(DOCKER_TAG) \
		--tag $(FULL_IMAGE):latest \
		--push \
		.
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Multi-arch image pushed: $(FULL_IMAGE):$(DOCKER_TAG) and $(FULL_IMAGE):latest"

docker-scan-checkov:
	@echo "$(COLOR_INFO)==> Scanning Dockerfile with Checkov...$(COLOR_RESET)"
	@checkov -d . -o cli || true
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Checkov scan complete"

docker-scan-trivy-config:
	@echo "$(COLOR_INFO)==> Scanning Dockerfile with Trivy (config)...$(COLOR_RESET)"
	trivy config --quiet --file-patterns "dockerfile:Dockerfile" .
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Trivy config scan complete"

# NOTE: This scans the single-arch image tagged $(DOCKER_TAG).
docker-scan-trivy-image: docker-build
	@echo "$(COLOR_INFO)==> Scanning Docker image with Trivy (CRITICAL)...$(COLOR_RESET)"
	trivy image --quiet --severity CRITICAL $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Trivy image scan complete"

docker-scan: docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) All security scans complete"

docker-tag:
	@echo "$(COLOR_INFO)==> Tagging image :$(DOCKER_TAG) -> :latest...$(COLOR_RESET)"
	docker tag $(FULL_IMAGE):$(DOCKER_TAG) $(FULL_IMAGE):latest
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Tagged: $(FULL_IMAGE):latest"

docker-push:
	@echo "$(COLOR_INFO)==> Pushing $(FULL_IMAGE):$(DOCKER_TAG)...$(COLOR_RESET)"
	docker push $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Pushed: $(FULL_IMAGE):$(DOCKER_TAG)"

docker-push-latest:
	@echo "$(COLOR_INFO)==> Pushing $(FULL_IMAGE):latest...$(COLOR_RESET)"
	docker push $(FULL_IMAGE):latest
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Pushed: $(FULL_IMAGE):latest"

docker-run: docker-build
	@echo "$(COLOR_INFO)==> Running Docker container...$(COLOR_RESET)"
	@echo "$(COLOR_WARN)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		--network host \
		$(FULL_IMAGE):$(DOCKER_TAG) \
		--service $${SERVICE:-cron} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

docker-clean:
	@echo "$(COLOR_INFO)==> Cleaning Docker images...$(COLOR_RESET)"
	-@docker rmi $(FULL_IMAGE):latest 2>/dev/null || true
	-@docker rmi $(FULL_IMAGE):$(DOCKER_TAG) 2>/dev/null || true
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Docker cleanup complete"

# ------------------------------------------------------------------------------
# CI/CD Pipeline Targets
# ------------------------------------------------------------------------------

pull_request: fmt vet lint test build docker-scan
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) PR pipeline complete"

# For multi-arch publishing in CI:
#   make merge DOCKER_TAG=v1.2.3
merge: pull_request docker-release
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Merge pipeline complete - multi-arch image pushed to registry"

# ------------------------------------------------------------------------------
# Cleanup Targets
# ------------------------------------------------------------------------------

clean:
	@echo "$(COLOR_INFO)==> Cleaning build artifacts...$(COLOR_RESET)"
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Clean complete"

clean-all: clean
	@echo "$(COLOR_INFO)==> Cleaning Go cache, buildkit, and builder...$(COLOR_RESET)"
	@$(GOCMD) clean -cache -modcache
	@rm -rf .buildkit
	@docker buildx rm -f multiarch-builder 2>/dev/null || true
	@echo "$(COLOR_OK)[OK]$(COLOR_RESET) Full clean complete"

# ------------------------------------------------------------------------------
# Help Target
# ------------------------------------------------------------------------------

help:
	@echo "$(COLOR_INFO)Health Check Service - Available Targets:$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_WARN)Setup:$(COLOR_RESET)"
	@echo "  init                         - Initialize project"
	@echo "  deps                         - Download and tidy Go dependencies"
	@echo ""
	@echo "$(COLOR_WARN)Build & Run:$(COLOR_RESET)"
	@echo "  all                          - Fetch dependencies and build (default)"
	@echo "  build                        - Build the binary (version metadata embedded)"
	@echo "  run                          - Build and run (SERVICE / PORT / INTERVAL overridable)"
	@echo "  run-env                      - Run with environment variables"
	@echo "  run-config                   - Run with config file"
	@echo ""
	@echo "$(COLOR_WARN)Development:$(COLOR_RESET)"
	@echo "  test                         - Run Go tests"
	@echo "  fmt                          - Format Go code"
	@echo "  vet                          - Run go vet"
	@echo "  lint                         - Run golangci-lint"
	@echo "  lint-fix                     - Run golangci-lint with auto-fix"
	@echo "  lint-verbose                 - Run golangci-lint (verbose output)"
	@echo "  vuln                         - Run govulncheck"
	@echo ""
	@echo "$(COLOR_WARN)TLS/HTTPS:$(COLOR_RESET)"
	@echo "  generate-cert                - Generate self-signed certificate for testing"
	@echo "  run-tls                      - Build and run with TLS enabled"
	@echo "  docker-run-tls               - Run Docker container with TLS"
	@echo "  run-autocert                 - Run with Let's Encrypt autocert"
	@echo "  docker-run-autocert          - Run Docker container with Let's Encrypt"
	@echo ""
	@echo "$(COLOR_WARN)Docker:$(COLOR_RESET)"
	@echo "  docker-build                 - Build single-arch image (tag=$(DOCKER_TAG))"
	@echo "  buildx-ensure                - Ensure buildx builder exists (run once per host)"
	@echo "  docker-release               - Build & push multi-arch (versioned tag + :latest)"
	@echo "  docker-scan-checkov          - Scan Dockerfile with Checkov"
	@echo "  docker-scan-trivy-config     - Scan Docker config with Trivy"
	@echo "  docker-scan-trivy-image      - Scan built image with Trivy"
	@echo "  docker-scan                  - Run all security scans"
	@echo "  docker-tag                   - Tag :$(DOCKER_TAG) -> :latest"
	@echo "  docker-push                  - Push :$(DOCKER_TAG)"
	@echo "  docker-push-latest           - Push :latest"
	@echo "  docker-run                   - Run container (uses tag=$(DOCKER_TAG))"
	@echo ""
	@echo "$(COLOR_WARN)CI/CD:$(COLOR_RESET)"
	@echo "  pull_request                 - PR pipeline (fmt, vet, lint, test, build, scans)"
	@echo "  merge                        - Multi-arch release"
	@echo ""
	@echo "$(COLOR_WARN)Cleanup:$(COLOR_RESET)"
	@echo "  clean                        - Remove build artifacts"
	@echo "  clean-all                    - Remove build artifacts, Go cache, and buildkit"
	@echo "  clean-certs                  - Remove generated certificates"
	@echo ""
	@echo "$(COLOR_WARN)Examples:$(COLOR_RESET)"
	@echo "  make docker-release-daemon DOCKER_TAG=v1.2.3"
	@echo "  make docker-buildx PLATFORMS=linux/amd64,linux/arm64"
	@echo "  make docker-run SERVICE=redis PORT=6379"
	@echo "  make merge DOCKER_TAG=v$$(date +%Y.%m.%d)-$$(git rev-parse --short HEAD)"
