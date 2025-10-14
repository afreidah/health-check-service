# Health Check Service

A lightweight, production-ready Go service that monitors systemd services via D-Bus and exposes their health status through HTTP endpoints with Prometheus metrics.

## Features

- **Systemd Integration**: Monitors any systemd service via D-Bus
- **Health Endpoint**: HTTP endpoint returns service status with appropriate status codes
- **Prometheus Metrics**: Built-in metrics for monitoring and alerting
- **Graceful Shutdown**: Proper cleanup on SIGTERM/SIGINT
- **Flexible Configuration**: Supports command-line flags, environment variables, and config files
- **Thread-Safe**: Concurrent-safe status caching
- **Containerized**: Multi-stage Docker build for minimal image size
- **CI/CD Ready**: GitHub Actions workflow for self-hosted runners
- **Security Scanned**: Automated Checkov and Trivy security scanning
- **Well Tested**: Comprehensive unit tests with 100% coverage goals

## Architecture

```
health-check-service/
├── cmd/health-checker/        # Application entry point
│   └── main.go
├── internal/
│   ├── cache/                 # Thread-safe status cache
│   ├── checker/               # Systemd service checker
│   ├── config/                # Configuration management
│   ├── handlers/              # HTTP handlers
│   └── metrics/               # Prometheus metrics
├── .github/workflows/         # CI/CD pipeline
├── Dockerfile                 # Multi-stage Docker build
├── docker-compose.yml         # Local development setup
├── Makefile                   # Comprehensive build system
└── config.yaml                # Example configuration
```

## Prerequisites

### Local Development
- **Go**: 1.25.1 or later
- **Make**: For using the Makefile
- **Docker**: For containerized builds (optional)
- **golangci-lint**: For linting (auto-installed via `make install-golangci-lint`)
- **gotestsum**: For test output (auto-installed via `make install-gotestsum`)

### Security Scanning (Optional)
- **Checkov**: For Dockerfile scanning (`make install-checkov`)
- **Trivy**: For image vulnerability scanning (`make install-trivy`)

### Runtime Requirements
- **systemd**: Must be available on the host
- **D-Bus**: Required for systemd communication
- **Linux**: Service requires D-Bus system socket access

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

### 4. Test
```bash
# Check health endpoint
curl http://localhost:8080/health

# Check Prometheus metrics
curl http://localhost:8080/metrics
```

## Configuration

The service supports three configuration methods with the following precedence (highest to lowest):
1. Command-line flags
2. Environment variables
3. Configuration file
4. Defaults

### Command-Line Flags
```bash
./bin/health-checker \
  --service nginx \           # Systemd service to monitor
  --port 8080 \               # HTTP port to listen on
  --interval 10 \             # Check interval in seconds
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

### Docker
| Target | Description |
|--------|-------------|
| `make docker-build` | Build Docker image |
| `make docker-scan-checkov` | Scan Dockerfile with Checkov |
| `make docker-scan-trivy-config` | Scan Dockerfile with Trivy (config) |
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

# Run with custom service
make docker-run SERVICE=postgresql
```

### CI/CD Pipelines
| Target | Description |
|--------|-------------|
| `make pull_request` | Full PR pipeline: fmt → lint → test → build → security scans |
| `make merge` | Merge pipeline: PR checks → tag → push to registry |

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
# → Tags image with commit SHA
# → Pushes to registry
```

### Cleanup
| Target | Description |
|--------|-------------|
| `make clean` | Remove build artifacts |
| `make clean-all` | Remove build artifacts and Go cache |

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
  docker-mirror.service.consul:5000/health-checker:latest \
  --service nginx --port 8080 --interval 10
```

**Important**: The container requires access to the host's D-Bus socket to communicate with systemd:
- Mount: `/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro`
- Network mode: `host` (for accessing systemd on the host)

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
   - Tag image with commit SHA
   - Push to registry: `docker-mirror.service.consul:5000/health-checker:<commit-sha>`

### Self-Hosted Runner Configuration

The workflow runs on your self-hosted Linux runners with the labels:
```yaml
runs-on: [self-hosted, linux]
```

**Runner Requirements:**
- Docker installed
- Go tooling support
- Python 3 with pipx (for Checkov)
- Access to your private registry: `docker-mirror.service.consul:5000`

### Registry

Images are pushed to your private Docker registry:
```
docker-mirror.service.consul:5000/health-checker:<tag>
```

The registry is configured in the Makefile:
```makefile
REGISTRY_HOST ?= docker-mirror.service.consul
REGISTRY_PORT ?= 5000
```

## API Endpoints

### Health Check Endpoint
```
GET /health
```

**Response Codes:**
- `200 OK` - Service is active
- `503 Service Unavailable` - Service is inactive, failed, or in transition
- `500 Internal Server Error` - Error checking service status

**Example:**
```bash
curl -i http://localhost:8080/health

HTTP/1.1 200 OK
Date: Tue, 14 Oct 2025 12:00:00 GMT
Content-Length: 0
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

## Prometheus Metrics

The service exposes the following Prometheus metrics:

### `health_check_requests_total`
**Type**: Counter  
**Description**: Total number of health check requests by HTTP status code  
**Labels**: `status_code`

```
health_check_requests_total{status_code="200"} 1523
health_check_requests_total{status_code="503"} 42
```

### `monitored_service_status`
**Type**: Gauge  
**Description**: Status of the monitored systemd service (1=active, 0=not active)  
**Labels**: `service`, `state`

```
monitored_service_status{service="nginx",state="active"} 1
monitored_service_status{service="postgresql",state="inactive"} 0
```

### `health_check_request_duration_seconds`
**Type**: Histogram  
**Description**: Duration of health check requests in seconds

```
health_check_request_duration_seconds_bucket{le="0.005"} 1234
health_check_request_duration_seconds_bucket{le="0.01"} 1456
...
```

## Testing

### Run All Tests
```bash
make test
```

### Run Tests with Coverage
```bash
go test -cover ./...
```

### Run Specific Package Tests
```bash
go test ./internal/cache
go test ./internal/handlers
```

### Test Files
The project includes comprehensive unit tests:
- `internal/cache/cache_test.go` - Cache concurrency and thread safety
- `internal/checker/checker_test.go` - State mapping validation
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

### 4. Commit and Push
```bash
git add .
git commit -m "Add feature"
git push origin feature/my-feature
```

### 5. Create Pull Request
- GitHub Actions will automatically run the PR pipeline
- Checks: formatting, linting, tests, build, security scans
- Review required checks in the PR

### 6. Merge to Main
- After PR approval, merge to main
- GitHub Actions will run the merge pipeline
- Image will be tagged with commit SHA and pushed to registry

## Systemd Service States

The service maps systemd ActiveState values to HTTP status codes:

| Systemd State | HTTP Status Code | Description |
|--------------|------------------|-------------|
| `active` | 200 OK | Service is running |
| `inactive` | 503 Service Unavailable | Service is stopped |
| `failed` | 503 Service Unavailable | Service has failed |
| `activating` | 503 Service Unavailable | Service is starting |
| `deactivating` | 503 Service Unavailable | Service is stopping |
| `reloading` | 503 Service Unavailable | Service is reloading |

## Troubleshooting

### Service Not Found
```
Service 'myservice' not found in systemd
```
**Solution**: Verify the service exists:
```bash
systemctl status myservice
```

### D-Bus Connection Failed
```
Failed to connect to D-Bus
```
**Solution**: Ensure D-Bus is running and you have permission to access the system bus:
```bash
systemctl status dbus
# For Docker, ensure socket is mounted correctly
```

### Permission Denied
```
Error checking service: permission denied
```
**Solution**: The service may require elevated permissions to access certain systemd units. Run as root or configure appropriate permissions.

### Port Already in Use
```
listen tcp :8080: bind: address already in use
```
**Solution**: Use a different port:
```bash
make run SERVICE=nginx PORT=8081
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Run `make pull_request` to verify all checks pass
5. Submit a pull request

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Author

Alex Freidah (alex.freidah@gmail.com)

## Links

- **Repository**: https://github.com/afreidah/health-check-service
- **Docker Registry**: docker-mirror.service.consul:5000
- **Issues**: https://github.com/afreidah/health-check-service/issues

---

**Built with ❤️ using Go, Docker, and GitHub Actions on self-hosted Nomad runners**
