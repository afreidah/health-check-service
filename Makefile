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
        test fmt lint lint-fix lint-verbose install-golangci-lint install-gotestsum help

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
	@echo "  $(COLOR_BLUE)init$(COLOR_RESET)                  - Initialize project (install tools, fetch deps)"
	@echo "  $(COLOR_BLUE)deps$(COLOR_RESET)                  - Download and tidy Go dependencies"
	@echo "  $(COLOR_BLUE)install-golangci-lint$(COLOR_RESET) - Install golangci-lint"
	@echo "  $(COLOR_BLUE)install-gotestsum$(COLOR_RESET)     - Install gotestsum"
	@echo ""
	@echo "$(COLOR_YELLOW)Build & Run:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)all$(COLOR_RESET)                   - Fetch dependencies and build (default)"
	@echo "  $(COLOR_BLUE)build$(COLOR_RESET)                 - Build the binary"
	@echo "  $(COLOR_BLUE)run$(COLOR_RESET)                   - Build and run (use SERVICE=name PORT=8080 to customize)"
	@echo "  $(COLOR_BLUE)run-env$(COLOR_RESET)               - Run with environment variables"
	@echo "  $(COLOR_BLUE)run-config$(COLOR_RESET)            - Run with config file"
	@echo ""
	@echo "$(COLOR_YELLOW)Development:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)test$(COLOR_RESET)                  - Run Go tests"
	@echo "  $(COLOR_BLUE)fmt$(COLOR_RESET)                   - Format Go code"
	@echo "  $(COLOR_BLUE)lint$(COLOR_RESET)                  - Run golangci-lint"
	@echo "  $(COLOR_BLUE)lint-fix$(COLOR_RESET)              - Run golangci-lint with auto-fix"
	@echo "  $(COLOR_BLUE)lint-verbose$(COLOR_RESET)          - Run golangci-lint (verbose output)"
	@echo ""
	@echo "$(COLOR_YELLOW)Cleanup:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)clean$(COLOR_RESET)                 - Remove build artifacts"
	@echo "  $(COLOR_BLUE)clean-all$(COLOR_RESET)             - Remove build artifacts and Go cache"
	@echo ""
	@echo "$(COLOR_YELLOW)Examples:$(COLOR_RESET)"
	@echo "  make run SERVICE=postgresql PORT=9090"
	@echo "  make run-env SERVICE=redis"
	@echo "  make test"
	@echo "  make lint-fix"
