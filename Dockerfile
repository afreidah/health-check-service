# ------------------------------------------------------------------------------
# Update the Build Stage section (around line 15)
# Add ARG declarations at the top of build stage
# ------------------------------------------------------------------------------

# ------------------------------------------------------------------------------
# Build Stage
# ------------------------------------------------------------------------------
FROM golang:1.25.1-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Install build dependencies
RUN apk add --no-cache git make curl

# Set working directory
WORKDIR /build

# Copy entire project
COPY . .

# Build using Makefile with build metadata
# The Makefile will use VERSION, COMMIT, BUILD_TIME from environment
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    VERSION=$(VERSION) \
    COMMIT=$(COMMIT) \
    BUILD_TIME=$(BUILD_TIME) \
    make build

# Verify binary was created
RUN test -f bin/health-checker || (echo "Binary not found!" && exit 1)

# ------------------------------------------------------------------------------
# Runtime Stage (no changes needed below)
# ------------------------------------------------------------------------------
FROM alpine:3.21

# Add metadata labels
LABEL org.opencontainers.image.title="Health Check Service"
LABEL org.opencontainers.image.description="Systemd service health checker with Prometheus metrics"
LABEL org.opencontainers.image.authors="alex.freidah@gmail.com"
LABEL org.opencontainers.image.source="https://github.com/afreidah/health-check-service"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.version="$(VERSION)"
LABEL org.opencontainers.image.revision="$(COMMIT)"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates dbus

# Create non-root user
RUN addgroup -g 1000 healthcheck && \
    adduser -D -u 1000 -G healthcheck healthcheck

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/health-checker /app/health-checker

# Change ownership
RUN chown -R healthcheck:healthcheck /app

# Switch to non-root user
USER healthcheck

# Expose HTTP port
EXPOSE 8080

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command
ENTRYPOINT ["/app/health-checker"]
CMD ["--service", "nginx", "--port", "8080", "--interval", "10"]
