# Health Check Service

A lightweight, production-ready Go service that monitors systemd services via D-Bus and exposes their health status through HTTP endpoints with Prometheus metrics and a real-time React dashboard.

## Features

- **Systemd Integration**: Monitors any systemd service via D-Bus
- **Real-time Dashboard**: Interactive React-based web UI with live status updates
- **Health Endpoint**: HTTP endpoint returns service status with appropriate status codes
- **JSON API**: RESTful API for programmatic access to service status
- **Prometheus Metrics**: Built-in metrics for monitoring and alerting
- **Stale Data Detection**: Automatic detection and warning for outdated health checks
- **Graceful Shutdown**: Proper cleanup on SIGTERM/SIGINT
- **Flexible Configuration**: Supports command-line flags, environment variables, and config files
- **Thread-Safe**: Concurrent-safe status caching
- **Self-Healing**: Automatic D-Bus reconnection with exponential backoff
- **TLS/HTTPS Support**: Manual certificates or Let's Encrypt autocert
- **Containerized**: Multi-stage Docker build for minimal image size (~20MB)
- **Multi-Architecture**: Native AMD64 and ARM64 support
- **CI/CD Ready**: GitHub Actions workflow for self-hosted runners
- **Security Scanned**: Automated Checkov and Trivy security scanning
- **Deployment Ready**: Kubernetes, Nomad, and Terraform configurations included

## Architecture

```
health-check-service/
├── cmd/health-checker/        # Application entry point
│   ├── main.go               # Main application
│   └── static/
│       └── dashboard.html    # Embedded React dashboard
├── internal/
│   ├── app/                  # Application setup and lifecycle
│   ├── cache/                # Thread-safe status cache
│   ├── checker/              # Systemd service checker with auto-reconnect
│   ├── config/               # Configuration management
│   ├── handlers/             # HTTP handlers
│   └── metrics/              # Prometheus metrics
├── integrations/
│   ├── kubernetes/           # K8s manifests and Terraform
│   │   ├── health-checker.k8s.yaml
│   │   └── tf/               # Terraform module
│   └── nomad/                # Nomad job specification
│       └── health-checker.nomad.hcl
├── .github/workflows/        # CI/CD pipeline
├── Dockerfile                # Multi-stage Docker build
├── docker-compose.yml        # Local development setup
└── Makefile                  # Comprehensive build system
```

## Prerequisites

### Local Development
- **Go**: 1.25.1 or later
- **Make**: For using the Makefile
- **Docker**: For containerized builds (optional)
- **systemd**: Required on host for service monitoring
- **D-Bus**: Required for systemd communication

### Tools (Auto-installed via Makefile)
- **golangci-lint**: For linting (`make install-golangci-lint`)
- **gotestsum**: For test output (`make install-gotestsum`)
- **Checkov**: For Dockerfile scanning (`make install-checkov`)
- **Trivy**: For image vulnerability scanning (`make install-trivy`)

### Runtime Requirements
- **Linux**: Service requires D-Bus system socket access
- **systemd**: Must be available and running
- **Permissions**: Access to `/var/run/dbus/system_bus_socket`

## Quick Start

### 1. Clone and Initialize
```bash
git clone https://github.com/afreidah/health-check-service.git
cd health-check-service

# Install dependencies and tools
make init
```

### 2. Build
```bash
make build
```

### 3. Run
```bash
# Monitor nginx service on port 8080
make run SERVICE=nginx PORT=8080

# Or run directly
./bin/health-checker --service nginx --port 8080 --interval 10
```

### 4. Access the Dashboard
Open your browser to:
- **Dashboard**: http://localhost:8080/
- **Health Check**: http://localhost:8080/health
- **Status API**: http://localhost:8080/api/status
- **Metrics**: http://localhost:8080/metrics

## Configuration

The service supports three configuration methods with the following precedence (highest to lowest):
1. Command-line flags (highest priority)
2. Environment variables
3. Configuration file
4. Defaults (lowest priority)

### Command-Line Flags
```bash
./bin/health-checker \
  --service nginx \           # Systemd service to monitor (required)
  --port 8080 \               # HTTP port to listen on (default: 8080)
  --interval 10 \             # Check interval in seconds (default: 10)
  --config config.yaml        # Optional config file path
```

### Environment Variables
```bash
export HEALTH_SERVICE=nginx
export HEALTH_PORT=8080
export HEALTH_INTERVAL=10

./bin/health-checker
```

### Configuration File (config.yaml)
```yaml
port: 8080
service: nginx
interval: 10
```

```bash
./bin/health-checker --config config.yaml
```

### TLS/HTTPS Configuration

The service supports three modes:

#### 1. HTTP Only (Default)
```bash
make run SERVICE=nginx PORT=8080
```

#### 2. HTTPS with Manual Certificates
```bash
# Generate self-signed cert for testing
make generate-cert

# Run with TLS
make run-tls SERVICE=nginx PORT=8443

# Or with flags
./bin/health-checker \
  --service nginx \
  --port 8443 \
  --tls-enabled \
  --tls-cert certs/server.crt \
  --tls-key certs/server.key
```

#### 3. HTTPS with Let's Encrypt (Autocert)
```bash
# Requires public DNS pointing to your server and ports 80/443 open
export HEALTH_TLS_AUTOCERT_DOMAIN=health.example.com
export HEALTH_TLS_AUTOCERT_EMAIL=admin@example.com

make run-autocert SERVICE=nginx

# Or with flags
./bin/health-checker \
  --service nginx \
  --port 443 \
  --tls-autocert \
  --tls-autocert-domain health.example.com \
  --tls-autocert-cache /var/cache/health-checker \
  --tls-autocert-email admin@example.com
```

## Dashboard

The service includes a real-time React dashboard that provides visual monitoring of your service health.

### Features
- **Live Status Updates**: Polls `/api/status` every 2 seconds
- **Visual Indicators**: Color-coded status (green=healthy, red=unhealthy)
- **Status History**: Bar chart showing last 20 health checks
- **Metrics Display**: Service state, status code, last checked time
- **Endpoint Inspector**: View raw responses from `/health` and `/metrics`

### Dashboard Screenshot
```
┌─────────────────────────────────────────────┐
│  Health Check Dashboard                     │
│  Real-time service monitoring               │
├─────────────────────────────────────────────┤
│  Service: nginx                             │
│  ✓ HEALTHY                                  │
│  State: active                              │
│  Status Code: 200                           │
├─────────────────────────────────────────────┤
│  Last Checked: 12:34:56                     │
│  Uptime: 99.9%                              │
│  Recent Checks: 20                          │
├─────────────────────────────────────────────┤
│  [Status History Chart]                     │
│  ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬                     │
└─────────────────────────────────────────────┘
```

### Access
- Open browser to `http://localhost:8080/` (or your configured port)
- Dashboard auto-refreshes every 2 seconds
- No external dependencies - served from embedded HTML

## API Endpoints

### Dashboard
```
GET /
```
Serves the embedded React dashboard.

### Health Check Endpoint
```
GET /health
```

**Response Codes:**
- `200 OK` - Service is active and healthy
- `503 Service Unavailable` - Service is inactive, failed, or in transition
- `500 Internal Server Error` - Error checking service status

**Headers:**
- `Warning: 199 - Stale health check data` - Appears when cached data is >30s old

**Example:**
```bash
curl -i http://localhost:8080/health

HTTP/1.1 200 OK
Date: Tue, 15 Oct 2025 12:00:00 GMT
Content-Length: 0
```

### Status API Endpoint
```
GET /api/status
```

Returns detailed JSON status for the dashboard.

**Response Format:**
```json
{
  "service": "nginx",
  "status": "healthy",
  "state": "active",
  "status_code": 200,
  "last_checked": "2025-10-15T12:34:56Z",
  "uptime": 99.9,
  "healthy": true
}
```

**Status Values:**
- `healthy` - Service is active (200)
- `unhealthy` - Service is down (503)
- `error` - Error checking service (500)

**Example:**
```bash
curl http://localhost:8080/api/status | jq
```

### Metrics Endpoint
```
GET /metrics
```

Returns Prometheus metrics in text format.

**Example:**
```bash
curl http://localhost:8080/metrics
```

## D-Bus Reconnection & Self-Healing

The service automatically recovers from D-Bus connection failures without manual intervention. This ensures continuous monitoring even during system bus restarts or transient connection issues.

### How It Works

When a D-Bus connection failure is detected:

1. **Detection**: Connection error triggers reconnection logic
2. **Cleanup**: Old connection is closed
3. **Reconnection**: Attempts to establish new connection
4. **Exponential Backoff**: Wait time increases between attempts
5. **Recovery**: Once reconnected, monitoring resumes normally

### Backoff Strategy

The reconnection logic uses exponential backoff to avoid overwhelming the system bus:

```
Attempt 1: Wait 1 second
Attempt 2: Wait 2 seconds
Attempt 3: Wait 4 seconds
Attempt 4: Wait 8 seconds
Attempt 5: Wait 16 seconds
Attempt 6+: Wait 30 seconds (max)
```

### Benefits

- ✅ **Zero Downtime**: HTTP endpoints remain responsive during reconnection
- ✅ **No Manual Intervention**: Service heals itself automatically
- ✅ **Observable**: All reconnection attempts are logged
- ✅ **Respectful**: Exponential backoff prevents system bus overload
- ✅ **Graceful**: Honors shutdown signals even during reconnection

### Monitoring Reconnections

Watch for reconnection events in logs:
```bash
# Connection failure
D-Bus connection error, attempting reconnection: <error details>

# Reconnection attempts
[Attempt 1] D-Bus reconnection failed, retrying in 2s: <error>
[Attempt 2] D-Bus reconnection failed, retrying in 4s: <error>

# Successful recovery
Successfully reconnected to D-Bus
```

## Stale Data Detection

The service includes automatic detection of stale health check data to ensure reliability.

### How It Works

- Health checks update the cache with a timestamp
- When serving requests, checks if data is older than 30 seconds
- Adds a `Warning` header to HTTP responses if stale
- Continues serving stale data rather than failing requests

### Warning Header

```bash
curl -i http://localhost:8080/health

HTTP/1.1 200 OK
Warning: 199 - Stale health check data
```

This indicates the cached status is >30s old, which may happen if:
- The background checker is stuck
- D-Bus is unresponsive
- System is under extreme load

### Use Cases

- **Monitoring Systems**: Can detect health checker problems
- **Load Balancers**: Can make informed decisions about stale data
- **Debugging**: Helps identify when the checker itself has issues

## Makefile Targets

The Makefile provides a comprehensive build system with color-coded output. Run `make help` to see all available targets.

### Setup & Dependencies
| Target | Description |
|--------|-------------|
| `make init` | First-time setup: install tools and fetch dependencies |
| `make deps` | Download and tidy Go dependencies |
| `make install-golangci-lint` | Install golangci-lint if not present |
| `make install-gotestsum` | Install gotestsum for better test output |
| `make install-checkov` | Install Checkov for Dockerfile scanning |
| `make install-trivy` | Install Trivy for container scanning |

### Build & Run
| Target | Description |
|--------|-------------|
| `make all` | Default target: fetch deps and build |
| `make build` | Build the binary to `bin/health-checker` |
| `make run` | Build and run with flags (use `SERVICE=name PORT=port` to customize) |
| `make run-env` | Run with environment variables |
| `make run-config` | Run with config file (creates example if missing) |

**Examples:**
```bash
make run SERVICE=postgresql PORT=9090
make run SERVICE=redis INTERVAL=5
make run-env SERVICE=nginx PORT=8181 INTERVAL=7
```

### Development
| Target | Description |
|--------|-------------|
| `make test` | Run all tests with gotestsum |
| `make fmt` | Format Go code |
| `make lint` | Run golangci-lint |
| `make lint-fix` | Run golangci-lint with auto-fix |
| `make lint-verbose` | Run linting with verbose output |

### TLS/HTTPS
| Target | Description |
|--------|-------------|
| `make generate-cert` | Generate self-signed certificate for testing |
| `make run-tls` | Build and run with TLS enabled |
| `make docker-run-tls` | Run Docker container with TLS |
| `make run-autocert` | Run with Let's Encrypt autocert |
| `make docker-run-autocert` | Run Docker with autocert |
| `make clean-certs` | Remove generated certificates |

### Docker
| Target | Description |
|--------|-------------|
| `make docker-build` | Build single-arch image (tag=$(DOCKER_TAG)) |
| `make buildx-setup` | Prepare Buildx (host networking + insecure HTTP registry) |
| `make docker-buildx` | Build multi-arch (no push) |
| `make docker-release` | Build & PUSH multi-arch via Buildx |
| `make docker-release-daemon` | Build & PUSH multi-arch via daemon + imagetools (reliable) |
| `make docker-scan-checkov` | Scan Dockerfile with Checkov |
| `make docker-scan-trivy-config` | Scan Docker config with Trivy |
| `make docker-scan-trivy-image` | Scan built image with Trivy (CRITICAL vulns) |
| `make docker-scan` | Run all security scans |
| `make docker-tag` | Tag image (use `DOCKER_TAG=version`) |
| `make docker-push` | Push tagged image to registry |
| `make docker-run` | Run container with D-Bus access |
| `make docker-compose-up` | Start with docker compose |
| `make docker-compose-down` | Stop docker compose |
| `make docker-clean` | Remove Docker images |

**Examples:**
```bash
# Build and scan
make docker-build
make docker-scan

# Tag and push
make docker-tag DOCKER_TAG=v1.2.3
make docker-push DOCKER_TAG=v1.2.3

# Multi-arch build (reliable method)
make docker-release-daemon DOCKER_TAG=v1.2.3

# Run with custom service
make docker-run SERVICE=postgresql
```

### CI/CD Pipelines
| Target | Description |
|--------|-------------|
| `make pull_request` | Full PR pipeline: fmt → lint → test → build → security scans |
| `make merge` | Merge pipeline: PR checks → multi-arch build → tag → push to registry |

**CI/CD Flow:**
```bash
# PR Pipeline (runs on pull requests)
make pull_request
# → Format code
# → Run linter
# → Run tests
# → Build binary
# → Build Docker image
# → Run Checkov scan
# → Run Trivy config scan
# → Run Trivy image scan

# Merge Pipeline (runs on push to main)
make merge DOCKER_TAG=abc123
# → Runs full PR pipeline
# → Builds multi-arch images (AMD64 + ARM64)
# → Tags images with commit SHA
# → Pushes to registry
```

### Cleanup
| Target | Description |
|--------|-------------|
| `make clean` | Remove build artifacts |
| `make clean-all` | Remove build artifacts and Go cache |
| `make clean-certs` | Remove generated certificates |

## Docker Usage

### Build
```bash
# Build image
make docker-build

# Or with docker directly
docker build -t health-checker:latest .
```

### Run Container
```bash
# Using Makefile (recommended)
make docker-run SERVICE=nginx

# Or with docker directly
docker run --rm \
  -v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
  --network host \
  health-checker:latest \
  --service nginx --port 8080 --interval 10
```

**Important**: The container requires access to the host's D-Bus socket to communicate with systemd:
- Mount: `/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro`
- Network mode: `host` (for accessing systemd on the host)

### Multi-Architecture Builds

The project supports building native images for AMD64 and ARM64:

```bash
# Build multi-arch images locally (no push)
make docker-buildx PLATFORMS=linux/amd64,linux/arm64

# Build and push multi-arch images to registry (reliable method)
make docker-release-daemon DOCKER_TAG=v1.2.3

# This creates:
# - your-registry/health-checker:v1.2.3-amd64
# - your-registry/health-checker:v1.2.3-arm64
# - your-registry/health-checker:v1.2.3 (manifest)
# - your-registry/health-checker:latest (manifest)
```

### Docker Compose
```bash
# Start service
make docker-compose-up

# Stop service
make docker-compose-down
```

Edit `docker-compose.yml` to customize the service being monitored.

## GitHub Actions CI/CD

The service includes a complete CI/CD pipeline that runs on **self-hosted runners in a Nomad cluster**.

### Workflow Overview

**File**: `.github/workflows/main.yml`

#### PR Pipeline (on pull_request to main)
1. **Checkout code**
2. **Setup Go 1.25.1**
3. **Install tooling** (golangci-lint, gotestsum, checkov via pipx)
4. **Run PR pipeline**: `make pull_request`
   - Format check
   - Linting
   - Tests
   - Build
   - Docker build
   - Security scans (Checkov + Trivy)

#### Merge Pipeline (on push to main)
1. **Checkout code**
2. **Setup Go 1.25.1**
3. **Install tooling**
4. **Run merge pipeline**: `make merge DOCKER_TAG=${{ github.sha }}`
   - All PR checks
   - Multi-arch build (AMD64 + ARM64)
   - Tag image with commit SHA
   - Push to configured registry
   - Push `:latest` tag

### Self-Hosted Runner Configuration

The workflow runs on your self-hosted Linux runners with the labels:
```yaml
runs-on: [self-hosted, linux]
```

**Runner Requirements:**
- Docker installed and configured
- Go tooling support
- Python 3 with pipx (for Checkov)
- Network connectivity for package downloads

### Registry

Images are pushed to your configured private Docker registry. The registry is configured in the Makefile:
```makefile
REGISTRY_HOST ?= docker-mirror.service.consul
REGISTRY_PORT ?= 5000
```

## Deployment Examples

The project includes example deployment configurations for Kubernetes and Nomad. These demonstrate the key requirements for running the service in container orchestration platforms - adapt them to your specific environment and requirements.

### Kubernetes

Example Kubernetes manifests show how to deploy the service in a cluster. The specifics (ports, namespaces, domains) are examples - what matters is the pattern of D-Bus access and security context.

#### Key Requirements for K8s Deployment

**D-Bus Socket Access:**
```yaml
volumes:
  - name: dbus-socket
    hostPath:
      path: /var/run/dbus/system_bus_socket
      type: Socket

volumeMounts:
  - name: dbus-socket
    mountPath: /var/run/dbus/system_bus_socket
    readOnly: true
```

**Security Context:**
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  # Add messagebus GID if needed for D-Bus access
  supplementalGroups: [106]
```

**Example Deployment Pattern:**
```bash
# See example manifests in integrations/kubernetes/
kubectl apply -f integrations/kubernetes/health-checker.k8s.yaml

# Or use the Terraform module
cd integrations/kubernetes/tf
terraform init
terraform apply
```

The example deploys to the `infra` namespace, monitors the `k3s` service, and exposes on port 18081. Includes readiness/liveness probes and optional Traefik ingress configuration. **These are reference values - use what makes sense for your environment.**

**Key Patterns to Follow:**
- Mount D-Bus socket from host
- Run as non-root with appropriate group membership
- Use hostPort or NodePort for direct node access
- Configure resource limits appropriately

### Nomad

Example Nomad job specification shows deployment patterns for HashiCorp Nomad. The specifics (datacenters, ports, domains) are examples - what matters is the pattern of D-Bus volume mounting and network configuration.

#### Key Requirements for Nomad Deployment

**D-Bus Volume Mount:**
```hcl
config {
  volumes = [
    "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro"
  ]
}
```

**Static Port Binding:**
```hcl
network {
  port "http" {
    static = 18080  # Host port
  }
}

config {
  ports = ["http"]  # Container binds to this
}
```

**Service Registration with Traefik:**
```hcl
service {
  name = "health-checker"
  port = "http"
  
  tags = [
    "traefik.enable=true",
    "traefik.http.routers.health.rule=Host(`health.example.com`)",
    "traefik.http.routers.health.entrypoints=websecure",
    "traefik.http.services.health.loadbalancer.server.port=18080",
  ]
}
```

**Example Deployment:**
```bash
# See example job specification
nomad job run integrations/nomad/health-checker.nomad.hcl

# Check status
nomad status health-checker
```

The example uses journald logging, static port binding, and Traefik integration with HTTPS. **These are reference values - adapt ports, datacenters, node pools, and service tags to your environment.**

**Key Patterns to Follow:**
- Mount D-Bus socket as read-only volume
- Use static ports for predictable service discovery
- Configure Traefik tags for ingress routing
- Enable health checks at both Nomad and load balancer level

### Docker Compose (Local Development)

```bash
# Start with docker compose
make docker-compose-up

# Or directly
docker compose up

# Access dashboard
open http://localhost:8080

# Stop
make docker-compose-down
```

## Prometheus Metrics

The service exposes the following Prometheus metrics at `/metrics`:

### `health_check_requests_total`
**Type**: Counter  
**Description**: Total number of health check requests by HTTP status code  
**Labels**: `status_code` (200, 503, 500)

**Example:**
```prometheus
health_check_requests_total{status_code="200"} 1523
health_check_requests_total{status_code="503"} 42
```

**Queries:**
```promql
# Request rate
rate(health_check_requests_total[5m])

# Error rate
rate(health_check_requests_total{status_code="500"}[5m])

# Success rate percentage
100 * rate(health_check_requests_total{status_code="200"}[5m]) 
  / rate(health_check_requests_total[5m])
```

### `monitored_service_status`
**Type**: Gauge  
**Description**: Status of the monitored systemd service (1=active, 0=not active)  
**Labels**: `service` (service name), `state` (systemd state)

**Example:**
```prometheus
monitored_service_status{service="nginx",state="active"} 1
monitored_service_status{service="postgresql",state="inactive"} 0
```

**Queries:**
```promql
# Service is down
monitored_service_status{service="nginx",state="active"} == 0

# Service uptime percentage (5m window)
100 * avg_over_time(monitored_service_status{service="nginx"}[5m])
```

### `health_check_request_duration_seconds`
**Type**: Histogram  
**Description**: Duration of health check requests in seconds

**Example:**
```prometheus
health_check_request_duration_seconds_bucket{le="0.005"} 1234
health_check_request_duration_seconds_bucket{le="0.01"} 1456
health_check_request_duration_seconds_sum 123.45
health_check_request_duration_seconds_count 10000
```

**Queries:**
```promql
# P95 latency
histogram_quantile(0.95, 
  rate(health_check_request_duration_seconds_bucket[5m]))

# P99 latency
histogram_quantile(0.99, 
  rate(health_check_request_duration_seconds_bucket[5m]))

# Average latency
rate(health_check_request_duration_seconds_sum[5m]) 
  / rate(health_check_request_duration_seconds_count[5m])
```

### `health_check_failures_total`
**Type**: Counter  
**Description**: Total number of failed health checks by error type  
**Labels**: `service` (service name), `error_type` (dbus_error, type_error)

**Example:**
```prometheus
health_check_failures_total{service="nginx",error_type="dbus_error"} 5
health_check_failures_total{service="nginx",error_type="type_error"} 1
```

**Queries:**
```promql
# Failure rate
rate(health_check_failures_total[5m])

# D-Bus connection failures
rate(health_check_failures_total{error_type="dbus_error"}[5m])
```

### Example Alerts

```yaml
groups:
  - name: health-checker
    interval: 30s
    rules:
      # Service is down
      - alert: ServiceDown
        expr: monitored_service_status{state="active"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Service {{ $labels.service }} is down"
          description: "Service has been down for more than 2 minutes"

      # Service is flapping
      - alert: ServiceFlapping
        expr: changes(monitored_service_status[5m]) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Service {{ $labels.service }} is flapping"
          description: "Service state changed {{ $value }} times in 5 minutes"

      # Health checker failures
      - alert: HealthCheckerFailures
        expr: rate(health_check_failures_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Health checker experiencing failures"
          description: "Health check failure rate: {{ $value | humanize }}/sec"

      # Slow health checks
      - alert: SlowHealthChecks
        expr: |
          histogram_quantile(0.95, 
            rate(health_check_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Health checks are slow"
          description: "P95 latency: {{ $value | humanizeDuration }}"

      # High error rate
      - alert: HighHealthCheckErrorRate
        expr: |
          rate(health_check_requests_total{status_code="500"}[5m]) 
            / rate(health_check_requests_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate on health checks"
          description: "Error rate: {{ $value | humanizePercentage }}"
```

## Testing

### Run All Tests
```bash
make test
```

### Run Tests with Coverage
```bash
go test -cover ./...

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Tests with Race Detector
```bash
go test -race ./...
```

### Run Specific Package Tests
```bash
go test ./internal/cache
go test ./internal/handlers
go test ./internal/checker
```

### Test Files
The project includes comprehensive unit tests:
- `internal/cache/cache_test.go` - Cache concurrency and thread safety
- `internal/checker/checker_test.go` - State mapping validation
- `internal/config/config_test.go` - Configuration validation
- `internal/handlers/handlers_test.go` - HTTP handler tests
- `internal/metrics/metrics_test.go` - Prometheus metrics tests

## Development Workflow

### 1. Create Feature Branch
```bash
git checkout -b feature/my-feature
```

### 2. Make Changes
```bash
# Format code
make fmt

# Run linter with auto-fix
make lint-fix

# Run tests
make test
```

### 3. Build and Test Locally
```bash
# Build binary
make build

# Run locally
make run SERVICE=nginx

# Or test with Docker
make docker-build
make docker-run SERVICE=nginx
```

### 4. Run Full PR Pipeline
```bash
# This runs all checks that GitHub Actions will run
make pull_request
```

### 5. Commit and Push
```bash
git add .
git commit -m "Add feature: description"
git push origin feature/my-feature
```

### 6. Create Pull Request
- GitHub Actions will automatically run the PR pipeline
- Checks: formatting, linting, tests, build, security scans
- Review required checks in the PR

### 7. Merge to Main
- After PR approval, merge to main
- GitHub Actions will run the merge pipeline
- Multi-arch image will be built, tagged with commit SHA, and pushed to registry

## Systemd Service States

The service maps systemd ActiveState values to HTTP status codes:

| Systemd State | HTTP Status Code | Description |
|--------------|------------------|-------------|
| `active` | 200 OK | Service is running normally |
| `inactive` | 503 Service Unavailable | Service is stopped |
| `failed` | 503 Service Unavailable | Service has failed |
| `activating` | 503 Service Unavailable | Service is starting up |
| `deactivating` | 503 Service Unavailable | Service is shutting down |
| `reloading` | 503 Service Unavailable | Service is reloading configuration |

**Design Decision:** Only `active` returns 200 OK. All other states indicate the service is not fully operational and should be taken out of rotation.

## Troubleshooting

### Service Not Found
```
Service 'myservice' not found in systemd
```
**Solution**: Verify the service exists:
```bash
systemctl status myservice

# List all services
systemctl list-units --type=service
```

### D-Bus Connection Failed
```
Failed to connect to D-Bus
```
**Solution**: Ensure D-Bus is running and you have permission to access the system bus:
```bash
systemctl status dbus

# Check D-Bus socket
ls -la /var/run/dbus/system_bus_socket

# For Docker, ensure socket is mounted correctly
docker run -v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro ...
```

**Note**: If D-Bus restarts while the service is running, the service will automatically reconnect - no manual intervention required. Watch the logs for reconnection messages.

### Permission Denied
```
Error checking service: permission denied
```
**Solution**: The service may require elevated permissions to access certain systemd units.

**For systemd units:**
```bash
# Run as root
sudo ./bin/health-checker --service myservice

# Or add user to systemd-journal group
sudo usermod -a -G systemd-journal $USER
```

**For Docker:**
```bash
# Add container user to messagebus group (GID 106)
# See Kubernetes manifest for example:
securityContext:
  supplementalGroups: [106]
```

### Port Already in Use
```
listen tcp :8080: bind: address already in use
```
**Solution**: Use a different port:
```bash
make run SERVICE=nginx PORT=8081

# Or find and kill the process using the port
lsof -i :8080
kill <PID>
```

### Reconnection Loops
If you see repeated reconnection attempts in logs:
```
[Attempt 10] D-Bus reconnection failed, retrying in 30s
```

**Possible Causes:**
1. D-Bus service is stopped
2. Permissions issue preventing connection
3. D-Bus socket not accessible (check mount in Docker)
4. SELinux/AppArmor blocking access

**Solution:**
```bash
# Check D-Bus status
sudo systemctl status dbus

# Restart D-Bus if needed
sudo systemctl restart dbus

# Check permissions
ls -la /var/run/dbus/system_bus_socket

# For Docker, verify mount
docker inspect <container> | grep -A 10 Mounts

# Check SELinux (if applicable)
sudo getenforce
sudo ausearch -m avc -ts recent
```

### Dashboard Not Loading
```
Failed to fetch /api/status
```

**Possible Causes:**
1. Service is not running
2. Wrong port
3. Firewall blocking access
4. CORS issues (if accessing from different origin)

**Solution:**
```bash
# Check if service is running
curl http://localhost:8080/health

# Check logs
docker logs health-checker
# or
kubectl logs -n infra deployment/health-checker

# Check firewall
sudo ufw status
sudo firewall-cmd --list-all

# Test from different origin (CORS)
curl -H "Origin: http://example.com" -v http://localhost:8080/api/status
```

### Stale Data Warnings
```
Warning: 199 - Stale health check data
```

**Meaning**: The cached health check data is older than 30 seconds.

**Possible Causes:**
1. Background checker is stuck
2. D-Bus is unresponsive
3. High CPU load delaying checks
4. Reconnection in progress

**Solution:**
```bash
# Check logs for D-Bus errors or reconnection attempts
docker logs health-checker

# Check system load
top
uptime

# Check D-Bus health
systemctl status dbus
journalctl -u dbus -n 50

# Restart service if needed
docker restart health-checker
```

### High Memory Usage
```
Container using excessive memory
```

**Solution:**
```bash
# Check actual usage
docker stats health-checker

# Expected: ~64-128MB
# If higher, check for goroutine leaks
curl http://localhost:8080/debug/pprof/goroutine

# Restart container if needed
docker restart health-checker

# Check for resource limits in deployment
kubectl describe pod -n infra health-checker
```

### Certificate Issues (TLS)
```
x509: certificate signed by unknown authority
```

**For self-signed certificates:**
```bash
# Use -k flag with curl
curl -k https://localhost:8443/health

# Or add cert to trust store
sudo cp certs/server.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates
```

**For Let's Encrypt:**
```bash
# Check DNS points to your server
dig health.example.com

# Verify ports 80 and 443 are open
sudo netstat -tlnp | grep -E ':(80|443)'

# Check Let's Encrypt logs
journalctl -u health-checker | grep -i acme

# Verify cache directory permissions
ls -la /var/cache/health-checker
```

## Security Considerations

### Container Security
- Runs as non-root user (UID/GID 1000)
- Read-only D-Bus socket mount
- Minimal Alpine base image
- No unnecessary capabilities
- Regular security scans (Checkov + Trivy)

### Network Security
- TLS/HTTPS support with modern ciphers
- Let's Encrypt integration for valid certificates
- Internal-only access via Traefik middleware (Nomad/K8s)
- No external dependencies at runtime

### Recommendations
1. **Use TLS in production** - Enable HTTPS for all deployments
2. **Restrict access** - Use firewall rules or ingress policies
3. **Update regularly** - Keep base images and Go version current
4. **Monitor metrics** - Watch for unusual patterns
5. **Rotate credentials** - If using registry authentication
6. **Scan images** - Run security scans in CI/CD

## Performance Considerations

### Resource Usage
- **CPU**: ~0.1-0.2 cores idle, ~0.5 cores under load
- **Memory**: ~64-128MB typical usage
- **Disk**: Minimal (embedded dashboard, no persistence)
- **Network**: Low (periodic D-Bus queries only)

### Scaling
- **Horizontal**: One instance per node (monitors local systemd)
- **Vertical**: Minimal resources needed
- **Load**: Can handle thousands of requests/sec to health endpoint

### Optimization Tips
1. **Adjust check interval** - Longer intervals reduce load
2. **Use cache headers** - HTTP clients can cache responses
3. **Monitor metrics** - Use Prometheus to identify bottlenecks
4. **Rate limit** - Consider adding rate limiting for public exposure

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with tests
4. Run `make pull_request` to verify all checks pass
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style
- Follow Go standard formatting (`make fmt`)
- Pass all linter checks (`make lint`)
- Add tests for new features
- Update documentation as needed
- Keep commits focused and atomic

### Testing Requirements
- All new code must have unit tests
- Run tests with race detector (`go test -race`)
- Maintain existing test coverage
- Add integration tests for new features

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Author

Alex Freidah  
Email: alex.freidah@gmail.com

## Links

- **Repository**: https://github.com/afreidah/health-check-service
- **Issues**: https://github.com/afreidah/health-check-service/issues

## Acknowledgments

Built with:
- **Go** - https://golang.org
- **D-Bus** - https://www.freedesktop.org/wiki/Software/dbus/
- **systemd** - https://systemd.io
- **Prometheus** - https://prometheus.io
- **React** - https://react.dev
- **Docker** - https://docker.com
- **GitHub Actions** - https://github.com/features/actions

---

**Built with ❤️ for reliable service monitoring**
