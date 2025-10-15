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
        install-golangci-lint install-gotestsum install-checkov install-trivy \
        test fmt lint lint-fix lint-verbose \
        docker-build docker-buildx buildx-setup docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image \
        docker-scan docker-tag docker-push docker-push-latest docker-run docker-compose-up docker-compose-down docker-clean \
        docker-release docker-release-daemon \
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
	@echo "$(COLOR_CYAN)==> Fetching Go dependencies...$(COLOR_RESET)"
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Dependencies updated"

init: deps install-golangci-lint install-gotestsum
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Project initialized and ready to build"

install-golangci-lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing golangci-lint...$(COLOR_RESET)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) golangci-lint installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) golangci-lint already installed"; \
	fi

install-gotestsum:
	@if ! command -v gotestsum >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing gotestsum...$(COLOR_RESET)"; \
		go install gotest.tools/gotestsum@latest; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) gotestsum installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) gotestsum already installed"; \
	fi

install-checkov:
	@if ! command -v checkov >/dev/null 2>&1; then \
		echo "$(COLOR_CYAN)==> Installing Checkov...$(COLOR_RESET)"; \
		pip3 install checkov || pip install checkov; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov installed"; \
	else \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov already installed"; \
	fi

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

build: deps
	@echo "$(COLOR_CYAN)==> Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# ------------------------------------------------------------------------------
# Run Targets
# ------------------------------------------------------------------------------

run: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME)...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Set SERVICE=<name> to monitor a different service$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

run-env: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with environment variables...$(COLOR_RESET)"
	HEALTH_SERVICE=$${SERVICE:-nginx} \
	HEALTH_PORT=$${PORT:-8181} \
	HEALTH_INTERVAL=$${INTERVAL:-7} \
	./$(BUILD_DIR)/$(BINARY_NAME)

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

test: install-gotestsum
	@echo "$(COLOR_CYAN)==> Running Go tests...$(COLOR_RESET)"
	@gotestsum --format testname ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Tests passed"

fmt:
	@echo "$(COLOR_CYAN)==> Formatting Go code...$(COLOR_RESET)"
	@$(GOFMT) ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Code formatted"

lint: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint...$(COLOR_RESET)"
	@golangci-lint run ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Linting complete"

lint-fix: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint with auto-fix...$(COLOR_RESET)"
	@golangci-lint run --fix ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Linting with fixes complete"

lint-verbose: install-golangci-lint
	@echo "$(COLOR_CYAN)==> Running golangci-lint (verbose)...$(COLOR_RESET)"
	@golangci-lint run -v ./...

# ------------------------------------------------------------------------------
# TLS/HTTPS Targets
# ------------------------------------------------------------------------------

generate-cert:
	@echo "$(COLOR_CYAN)==> Generating self-signed certificate...$(COLOR_RESET)"
	@mkdir -p certs
	@openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt \
		-days 365 -nodes -subj "/CN=localhost" 2>/dev/null
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Certificate generated: certs/server.crt"
	@echo "$(COLOR_YELLOW)Note: This is a self-signed certificate for testing only$(COLOR_RESET)"

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

run-autocert: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME) with Let's Encrypt autocert...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires ports 80 and 443 on this host and public DNS$(COLOR_RESET)"
	@if [ -z "$${HEALTH_TLS_AUTOCERT_DOMAIN}" ]; then \
		echo "$(COLOR_RED)[ERR]$(COLOR_RESET) Set HEALTH_TLS_AUTOCERT_DOMAIN=alexfreidah.com"; exit 1; \
	fi
	HEALTH_SERVICE=$${HEALTH_SERVICE:-nginx} \
	HEALTH_PORT=443 \
	HEALTH_INTERVAL=$${HEALTH_INTERVAL:-10} \
	HEALTH_TLS_AUTOCERT=true \
	HEALTH_TLS_AUTOCERT_DOMAIN=$${HEALTH_TLS_AUTOCERT_DOMAIN} \
	HEALTH_TLS_AUTOCERT_CACHE=$${HEALTH_TLS_AUTOCERT_CACHE:-./acme-cache} \
	HEALTH_TLS_AUTOCERT_EMAIL=$${HEALTH_TLS_AUTOCERT_EMAIL:-} \
	./$(BUILD_DIR)/$(BINARY_NAME)

docker-run-tls: docker-build generate-cert
	@echo "$(COLOR_CYAN)==> Running Docker container with TLS...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		-v $(PWD)/certs:/app/certs:ro \
		--network host \
		$(FULL_IMAGE):$(DOCKER_TAG) \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8443} \
		--interval $${INTERVAL:-10} \
		--tls_enabled \
		--tls_cert /app/certs/server.crt \
		--tls_key /app/certs/server.key

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
		$(FULL_IMAGE):$(DOCKER_TAG)

clean-certs:
	@echo "$(COLOR_CYAN)==> Removing certificates...$(COLOR_RESET)"
	@rm -rf certs
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Certificates removed"

# ------------------------------------------------------------------------------
# Docker Targets
# ------------------------------------------------------------------------------

# Build single-arch image for the current host arch (tags with $(DOCKER_TAG))
docker-build:
	@echo "$(COLOR_CYAN)==> Building Docker image (single-arch)...$(COLOR_RESET)"
	docker build -t $(FULL_IMAGE):$(DOCKER_TAG) .
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Docker image built: $(FULL_IMAGE):$(DOCKER_TAG)"

# Setup Buildx builder with HOST networking and a BuildKit config that forces HTTP/insecure
buildx-setup:
	@echo "$(COLOR_CYAN)==> Ensuring Buildx builder is ready (host DNS + HTTP registry)...$(COLOR_RESET)"
	@docker buildx version >/dev/null 2>&1 || { echo "$(COLOR_RED)[ERR]$(COLOR_RESET) Docker Buildx is not available. Please upgrade Docker."; exit 1; }
	@docker run --privileged --rm tonistiigi/binfmt --install all >/dev/null 2>&1 || true
	@mkdir -p .buildkit
	@echo "[registry.\"$(REGISTRY)\"]"                > .buildkit/buildkitd.toml
	@echo "  http = true"                            >> .buildkit/buildkitd.toml
	@echo "  insecure = true"                        >> .buildkit/buildkitd.toml
	@docker buildx rm -f multiarch-builder >/dev/null 2>&1 || true
	@docker buildx create \
		--name multiarch-builder \
		--driver docker-container \
		--driver-opt network=host \
		--config .buildkit/buildkitd.toml \
		--use
	@docker buildx inspect --bootstrap >/dev/null
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Buildx builder: multiarch-builder (network=host, http/insecure for $(REGISTRY))"

# Build multi-arch image locally (no push)
docker-buildx: buildx-setup
	@echo "$(COLOR_CYAN)==> Building multi-arch image (no push): $(PLATFORMS)$(COLOR_RESET)"
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(FULL_IMAGE):$(DOCKER_TAG) \
		--load \
		.
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Multi-arch build complete (loaded into local docker as host arch only)"

# Build and PUSH multi-arch image (tags: $(DOCKER_TAG), latest) - may fail on private HTTP/DNS
docker-release: buildx-setup
	@echo "$(COLOR_CYAN)==> Building & pushing MULTI-ARCH image (HTTP/insecure registry)...$(COLOR_RESET)"
	@echo "$(COLOR_CYAN)     Image: $(FULL_IMAGE)$(COLOR_RESET)"
	@echo "$(COLOR_CYAN)     Tags : $(DOCKER_TAG), latest$(COLOR_RESET)"
	docker buildx build \
		--platform $(PLATFORMS) \
		--tag $(FULL_IMAGE):$(DOCKER_TAG) \
		--tag $(FULL_IMAGE):latest \
		--push \
		.
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Multi-arch image pushed: $(FULL_IMAGE):$(DOCKER_TAG) and :latest"

# Reliable path: build each arch, push via daemon, then compose + push manifest via Buildx imagetools
docker-release-daemon: buildx-setup
	@set -e; \
	echo "$(COLOR_CYAN)==> Building $(ARCH_AMD) (--load) → $(IMG_AMD)$(COLOR_RESET)"; \
	docker buildx build --platform $(ARCH_AMD) -t $(IMG_AMD) --load .; \
	echo "$(COLOR_CYAN)==> Building $(ARCH_ARM) (--load) → $(IMG_ARM)$(COLOR_RESET)"; \
	docker buildx build --platform $(ARCH_ARM) -t $(IMG_ARM) --load .; \
	echo "$(COLOR_CYAN)==> Pushing arch images via Docker daemon...$(COLOR_RESET)"; \
	docker push $(IMG_AMD); \
	docker push $(IMG_ARM); \
	echo "$(COLOR_CYAN)==> Creating multi-arch manifest (Buildx imagetools) for :$(DOCKER_TAG)$(COLOR_RESET)"; \
	BUILDX_REGISTRY_PLAINHTTP=1 docker buildx imagetools create \
		--builder multiarch-builder \
		--tag $(FULL_IMAGE):$(DOCKER_TAG) \
		$(IMG_AMD) $(IMG_ARM); \
	echo "$(COLOR_CYAN)==> Creating multi-arch manifest (Buildx imagetools) for :latest$(COLOR_RESET)"; \
	BUILDX_REGISTRY_PLAINHTTP=1 docker buildx imagetools create \
		--builder multiarch-builder \
		--tag $(FULL_IMAGE):latest \
		$(IMG_LATEST_AMD) $(IMG_LATEST_ARM); \
	echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Multi-arch manifests pushed: $(FULL_IMAGE):$(DOCKER_TAG), latest"

docker-scan-checkov: install-checkov
	@echo "$(COLOR_CYAN)==> Scanning Dockerfile with Checkov...$(COLOR_RESET)"
	checkov -f Dockerfile
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Checkov scan complete"

docker-scan-trivy-config: install-trivy
	@echo "$(COLOR_CYAN)==> Scanning Dockerfile with Trivy (config)...$(COLOR_RESET)"
	trivy config --quiet --file-patterns "dockerfile:Dockerfile" .
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy config scan complete"

# NOTE: This scans the single-arch image tagged $(DOCKER_TAG).
docker-scan-trivy-image: docker-build install-trivy
	@echo "$(COLOR_CYAN)==> Scanning Docker image with Trivy (CRITICAL)...$(COLOR_RESET)"
	trivy image --quiet --severity CRITICAL $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Trivy image scan complete"

docker-scan: docker-scan-checkov docker-scan-trivy-config docker-scan-trivy-image
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) All security scans complete"

docker-tag:
	@echo "$(COLOR_CYAN)==> Tagging image :$(DOCKER_TAG) -> :latest...$(COLOR_RESET)"
	docker tag $(FULL_IMAGE):$(DOCKER_TAG) $(FULL_IMAGE):latest
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Tagged: $(FULL_IMAGE):latest"

docker-push:
	@echo "$(COLOR_CYAN)==> Pushing $(FULL_IMAGE):$(DOCKER_TAG)...$(COLOR_RESET)"
	docker push $(FULL_IMAGE):$(DOCKER_TAG)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Pushed: $(FULL_IMAGE):$(DOCKER_TAG)"

docker-push-latest:
	@echo "$(COLOR_CYAN)==> Pushing $(FULL_IMAGE):latest...$(COLOR_RESET)"
	docker push $(FULL_IMAGE):latest
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Pushed: $(FULL_IMAGE):latest"

docker-run: docker-build
	@echo "$(COLOR_CYAN)==> Running Docker container...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)Note: Requires access to host D-Bus socket$(COLOR_RESET)"
	docker run --rm \
		-v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
		--network host \
		$(FULL_IMAGE):$(DOCKER_TAG) \
		--service $${SERVICE:-nginx} \
		--port $${PORT:-8080} \
		--interval $${INTERVAL:-10}

docker-compose-up:
	@echo "$(COLOR_CYAN)==> Starting with docker compose...$(COLOR_RESET)"
	docker compose up --build

docker-compose-down:
	@echo "$(COLOR_CYAN)==> Stopping docker compose...$(COLOR_RESET)"
	docker compose down

docker-clean:
	@echo "$(COLOR_CYAN)==> Cleaning Docker images...$(COLOR_RESET)"
	-@docker rmi $(FULL_IMAGE):latest 2>/dev/null || true
	-@docker rmi $(FULL_IMAGE):$(DOCKER_TAG) 2>/dev/null || true
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Docker cleanup complete"

# ------------------------------------------------------------------------------
# CI/CD Pipeline Targets
# ------------------------------------------------------------------------------

pull_request: fmt lint test build docker-scan
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) PR pipeline complete"

# For multi-arch publishing in CI:
#   make merge DOCKER_TAG=v1.2.3
# Use the reliable daemon-push path by default
merge: pull_request docker-release-daemon
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Merge pipeline complete - multi-arch image pushed to registry"

# ------------------------------------------------------------------------------
# Cleanup Targets
# ------------------------------------------------------------------------------

clean:
	@echo "$(COLOR_CYAN)==> Cleaning build artifacts...$(COLOR_RESET)"
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Clean complete"

clean-all: clean
	@echo "$(COLOR_CYAN)==> Cleaning Go cache...$(COLOR_RESET)"
	@$(GOCMD) clean -cache -modcache
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Full clean complete"

# ------------------------------------------------------------------------------
# Help Target
# ------------------------------------------------------------------------------

help:
	@echo "$(COLOR_CYAN)Health Check Service - Available Targets:$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_YELLOW)Setup & Dependencies:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)init$(COLOR_RESET)                         - Initialize project (install tools, fetch deps)"
	@echo "  $(COLOR_BLUE)deps$(COLOR_RESET)                         - Download and tidy Go dependencies"
	@echo "  $(COLOR_BLUE)install-golangci-lint$(COLOR_RESET)        - Install golangci-lint"
	@echo "  $(COLOR_BLUE)install-gotestsum$(COLOR_RESET)            - Install gotestsum"
	@echo "  $(COLOR_BLUE)install-checkov$(COLOR_RESET)              - Install Checkov"
	@echo "  $(COLOR_BLUE)install-trivy$(COLOR_RESET)                - Install Trivy"
	@echo ""
	@echo "$(COLOR_YELLOW)Build & Run:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)all$(COLOR_RESET)                          - Fetch dependencies and build (default)"
	@echo "  $(COLOR_BLUE)build$(COLOR_RESET)                        - Build the binary"
	@echo "  $(COLOR_BLUE)run$(COLOR_RESET)                          - Build and run (SERVICE / PORT / INTERVAL overridable)"
	@echo "  $(COLOR_BLUE)run-env$(COLOR_RESET)                      - Run with environment variables"
	@echo "  $(COLOR_BLUE)run-config$(COLOR_RESET)                   - Run with config file"
	@echo ""
	@echo "$(COLOR_YELLOW)Development:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)test$(COLOR_RESET)                         - Run Go tests"
	@echo "  $(COLOR_BLUE)fmt$(COLOR_RESET)                          - Format Go code"
	@echo "  $(COLOR_BLUE)lint$(COLOR_RESET)                         - Run golangci-lint"
	@echo "  $(COLOR_BLUE)lint-fix$(COLOR_RESET)                     - Run golangci-lint with auto-fix"
	@echo "  $(COLOR_BLUE)lint-verbose$(COLOR_RESET)                 - Run golangci-lint (verbose output)"
	@echo ""
	@echo "$(COLOR_YELLOW)TLS/HTTPS:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)generate-cert$(COLOR_RESET)                - Generate self-signed certificate for testing"
	@echo "  $(COLOR_BLUE)run-tls$(COLOR_RESET)                      - Build and run with TLS enabled"
	@echo "  $(COLOR_BLUE)docker-run-tls$(COLOR_RESET)               - Run Docker container with TLS"
	@echo ""
	@echo "$(COLOR_YELLOW)Docker:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)docker-build$(COLOR_RESET)                 - Build single-arch image (tag=$(DOCKER_TAG))"
	@echo "  $(COLOR_BLUE)buildx-setup$(COLOR_RESET)                 - Prepare Buildx (host networking + insecure HTTP registry)"
	@echo "  $(COLOR_BLUE)docker-buildx$(COLOR_RESET)                - Build multi-arch (no push)"
	@echo "  $(COLOR_BLUE)docker-release$(COLOR_RESET)               - Build & PUSH multi-arch via Buildx (may fail on private registries)"
	@echo "  $(COLOR_BLUE)docker-release-daemon$(COLOR_RESET)        - Build & PUSH multi-arch via daemon + imagetools (reliable)"
	@echo "  $(COLOR_BLUE)docker-scan-checkov$(COLOR_RESET)          - Scan Dockerfile with Checkov"
	@echo "  $(COLOR_BLUE)docker-scan-trivy-config$(COLOR_RESET)     - Scan Docker config with Trivy"
	@echo "  $(COLOR_BLUE)docker-scan-trivy-image$(COLOR_RESET)      - Scan built image with Trivy"
	@echo "  $(COLOR_BLUE)docker-tag$(COLOR_RESET)                   - Tag :$(DOCKER_TAG) -> :latest"
	@echo "  $(COLOR_BLUE)docker-push$(COLOR_RESET)                  - Push :$(DOCKER_TAG)"
	@echo "  $(COLOR_BLUE)docker-push-latest$(COLOR_RESET)           - Push :latest"
	@echo "  $(COLOR_BLUE)docker-run$(COLOR_RESET)                   - Run container (uses tag=$(DOCKER_TAG))"
	@echo ""
	@echo "$(COLOR_YELLOW)CI/CD:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)pull_request$(COLOR_RESET)                 - PR pipeline (fmt, lint, test, build, scans)"
	@echo "  $(COLOR_BLUE)merge$(COLOR_RESET)                        - Multi-arch release (daemon push + imagetools manifest)"
	@echo ""
	@echo "$(COLOR_YELLOW)Cleanup:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)clean$(COLOR_RESET)                        - Remove build artifacts"
	@echo "  $(COLOR_BLUE)clean-all$(COLOR_RESET)                    - Remove build artifacts and Go cache"
	@echo "  $(COLOR_BLUE)clean-certs$(COLOR_RESET)                  - Remove generated certificates"
	@echo ""
	@echo "$(COLOR_YELLOW)Examples:$(COLOR_RESET)"
	@echo "  make docker-release-daemon DOCKER_TAG=v1.2.3"
	@echo "  make docker-buildx PLATFORMS=linux/amd64,linux/arm64"
	@echo "  make docker-run SERVICE=redis PORT=6379"
	@echo "  make merge DOCKER_TAG=v$$(date +%Y.%m.%d)-$$(git rev-parse --short HEAD)"

