# ------------------------------------------------------------------------------
# Makefile - Health Check Service
#
# Systemd service health checker with Prometheus metrics and graceful shutdown.
# Single Makefile for building, testing, and running the service.
# ------------------------------------------------------------------------------

# --- Variables ---
BINARY_NAME := health-checker
BUILD_DIR := bin
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

.PHONY: all build run clean deps test fmt lint help

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
init: deps
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Project initialized and ready to build"

# ------------------------------------------------------------------------------
# Build Targets
# ------------------------------------------------------------------------------

# Build the binary
build: deps
	@echo "$(COLOR_CYAN)==> Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build and run with default flags
run: build
	@echo "$(COLOR_CYAN)==> Running $(BINARY_NAME)...$(COLOR_RESET)"
	./$(BUILD_DIR)/$(BINARY_NAME) -service nginx -port 8080 -interval 10

# ------------------------------------------------------------------------------
# Development Targets
# ------------------------------------------------------------------------------

# Run Go tests
test:
	@echo "$(COLOR_CYAN)==> Running Go tests...$(COLOR_RESET)"
	@$(GOTEST) -v ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Tests passed"

# Format Go code
fmt:
	@echo "$(COLOR_CYAN)==> Formatting Go code...$(COLOR_RESET)"
	@$(GOFMT) ./...
	@echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Code formatted"

# Run Go linter (requires golangci-lint)
lint:
	@echo "$(COLOR_CYAN)==> Running Go linter...$(COLOR_RESET)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
		echo "$(COLOR_GREEN)[OK]$(COLOR_RESET) Linting complete"; \
	else \
		echo "$(COLOR_YELLOW)[WARN]$(COLOR_RESET) golangci-lint not installed, skipping"; \
	fi

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
	@echo "$(COLOR_CYAN)Available targets:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)all$(COLOR_RESET)        - Fetch dependencies and build (default)"
	@echo "  $(COLOR_BLUE)build$(COLOR_RESET)      - Build the binary"
	@echo "  $(COLOR_BLUE)run$(COLOR_RESET)        - Build and run with default flags"
	@echo "  $(COLOR_BLUE)deps$(COLOR_RESET)       - Download and tidy Go dependencies"
	@echo "  $(COLOR_BLUE)init$(COLOR_RESET)       - Initialize project (first-time setup)"
	@echo "  $(COLOR_BLUE)test$(COLOR_RESET)       - Run Go tests"
	@echo "  $(COLOR_BLUE)fmt$(COLOR_RESET)        - Format Go code"
	@echo "  $(COLOR_BLUE)lint$(COLOR_RESET)       - Run Go linter"
	@echo "  $(COLOR_BLUE)clean$(COLOR_RESET)      - Remove build artifacts"
	@echo "  $(COLOR_BLUE)clean-all$(COLOR_RESET)  - Remove build artifacts and Go cache"
	@echo "  $(COLOR_BLUE)help$(COLOR_RESET)       - Show this help message"
