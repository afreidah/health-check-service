# Health Check Service

A lightweight, production-ready Go service that monitors systemd services via D-Bus and exposes health status through HTTP endpoints with Prometheus metrics and a real-time React dashboard.

## Features

- **Systemd Integration** via D-Bus with auto-reconnect and exponential backoff
- **Real-time Dashboard** with React frontend, live status updates, and check history
- **HTTP Health Endpoint** returning appropriate status codes (200/503/500)
- **RESTful API** for programmatic access to service status
- **Prometheus Metrics** for monitoring and alerting
- **Stale Data Detection** with automatic warnings
- **Graceful Shutdown** with proper cleanup
- **Flexible Configuration** via flags, environment variables, or config file
- **Thread-Safe** concurrent access patterns
- **TLS/HTTPS Support** with manual certificates or Let's Encrypt
- **Containerized** with multi-arch support (AMD64, ARM64)
- **Security Scanning** via Checkov and Trivy
- **Deployment Examples** for Kubernetes and Nomad with Terraform modules

## Prerequisites

**Local Development:**
- Go 1.25.1+
- Make
- systemd and D-Bus (for service monitoring)

**Docker/Container:**
- Linux with D-Bus socket access
- systemd available on host

**Tools (auto-installed via Makefile):**
- golangci-lint, gotestsum, Checkov, Trivy

## Quick Start

```bash
# Clone and setup
git clone https://github.com/afreidah/health-check-service.git
cd health-check-service
make init

# Build
make build

# Run (monitor nginx on port 8080)
make run SERVICE=nginx PORT=8080

# Access
curl http://localhost:8080/health
open http://localhost:8080  # Dashboard
```

## Configuration

Configuration follows this precedence (highest to lowest):
1. Command-line flags: `--service nginx --port 8080 --interval 10`
2. Environment variables: `HEALTH_SERVICE=nginx HEALTH_PORT=8080`
3. Config file (YAML): `--config config.yaml`
4. Defaults

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `--service` | string | required | Systemd service name (without .service suffix) |
| `--port` | int | 8080 | HTTP listening port |
| `--interval` | int | 10 | Check interval in seconds |
| `--config` | string | - | Optional YAML config file path |

### TLS/HTTPS

Three modes available:

```bash
# Manual certificates
make run-tls SERVICE=nginx PORT=8443

# Let's Encrypt (requires public DNS and ports 80/443)
HEALTH_TLS_AUTOCERT_DOMAIN=health.example.com make run-autocert

# Or pass flags directly to binary
./bin/health-checker --service nginx --tls-autocert --tls-autocert-domain health.example.com
```

## API Endpoints

| Endpoint | Purpose | Returns |
|----------|---------|---------|
| `GET /` | React dashboard | HTML |
| `GET /health` | Health check | 200/503/500 with optional Warning header |
| `GET /api/status` | JSON status | Detailed status for dashboard/clients |
| `GET /metrics` | Prometheus metrics | Formatted text |

### Health Endpoint

Returns appropriate HTTP status codes:
- `200 OK` - Service is active
- `503 Service Unavailable` - Service is inactive/failed/transitioning
- `500 Internal Server Error` - Error checking status
- Includes `Warning` header if cached data is >30s old

### Status API Response

```json
{
  "service": "nginx",
  "status": "healthy",
  "state": "active",
  "status_code": 200,
  "last_checked": "2025-10-15T12:34:56Z",
  "uptime": 99.9,
  "healthy": true,
  "stale": false,
  "staleness_s": 5
}
```

## D-Bus Auto-Reconnection

The service automatically recovers from D-Bus connection failures without manual intervention:

- Detects connection failures
- Reconnects with exponential backoff (1s → 30s max)
- Context-aware waits during graceful shutdown
- Continues serving last-known-good status

Monitor in logs for reconnection events.

## Systemd Service States

Only `active` returns 200 OK. All other states return 503:

| State | HTTP Code |
|-------|-----------|
| active | 200 |
| inactive | 503 |
| failed | 503 |
| activating | 503 |
| deactivating | 503 |
| reloading | 503 |

## Docker

```bash
# Build image
make docker-build

# Run container with D-Bus access
make docker-run SERVICE=nginx

# Or directly
docker run --rm \
  -v /var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro \
  --network host \
  docker-mirror.service.consul:5000/health-checker:latest \
  --service nginx --port 8080

# Multi-arch build and push to registry
make docker-release DOCKER_TAG=v1.2.3
```

## Deployment

Pre-built configurations are provided in `examples/`:

### Kubernetes

Separate manifests in `examples/kubernetes/`:
- `namespace.yaml` - Isolated namespace
- `deployment.yaml` - Pod specification with D-Bus mounting
- `service.yaml` - Internal service discovery
- `ingress.yaml` - Traefik routing (update hostname)
- `network-policy.yaml` - Restricted access via Traefik
- `pod-disruption-budget.yaml` - Prevent eviction during maintenance

Or use Terraform module in `examples/kubernetes/terraform/`:
```bash
cd examples/kubernetes/terraform
terraform apply -var="ingress_host=health.example.com"
```

### Nomad

- `examples/nomad/health-checker.nomad.hcl` - Full job specification
- Static port binding (18080) for load balancer integration
- Traefik routing tags included
- Auto-reconnect to Consul after failures

Or use Terraform module in `examples/nomad/terraform/`:
```bash
cd examples/nomad/terraform
terraform apply -var="ingress_host=health.example.com"
```

## Prometheus Metrics

Available at `/metrics` in Prometheus text format:

- **health_check_requests_total** - Counter of requests by status code
- **monitored_service_status** - Gauge (1=active, 0=not active)
- **health_check_request_duration_seconds** - Histogram of response times
- **health_check_failures_total** - Counter by error type (dbus_error, type_error)
- **health_checker_healthy** - Gauge (1=checker responsive, 0=stuck)
- **health_checker_last_check_timestamp_seconds** - Unix timestamp of last check

Example Prometheus query:
```promql
# Is service down?
monitored_service_status{service="nginx"} == 0

# Service uptime percentage
100 * avg_over_time(monitored_service_status{service="nginx"}[5m])

# Error rate
rate(health_check_failures_total[5m])
```

## Development

### Build & Test

```bash
make build          # Build binary
make test           # Run tests
make test -race     # With race detector
make fmt            # Format code
make lint           # Linting
make lint-fix       # Auto-fix linting issues
```

### Full PR Pipeline

```bash
# Run all checks that GitHub Actions runs
make pull_request
```

### Development Workflow

1. Create feature branch
2. Make changes and test locally: `make pull_request`
3. Commit and push to origin
4. Create pull request
5. After approval, merge to main
6. GitHub Actions automatically runs merge pipeline and pushes multi-arch images

## Makefile

Most common targets:

```bash
make help                   # Show all targets
make init                   # First-time setup
make build                  # Build binary
make run SERVICE=nginx      # Build and run
make test                   # Run tests
make docker-build           # Build single-arch image
make docker-scan            # Security scans
make clean                  # Clean build artifacts
```

Full target list available with `make help`.

## Troubleshooting

### Service Not Found
```bash
systemctl status myservice    # Verify service exists
systemctl list-units --type=service  # List all services
```

### D-Bus Connection Failed
```bash
systemctl status dbus         # Verify D-Bus is running
ls -la /var/run/dbus/system_bus_socket
# For Docker: ensure socket is mounted correctly
```

### Port Already in Use
```bash
make run SERVICE=nginx PORT=8081  # Use different port
lsof -i :8080                    # Find process using port
```

### Stale Data Warning
If you see `Warning: 199 - Stale health check data` in health endpoint responses:
- Check D-Bus connectivity
- Check system load
- Look for checker reconnection attempts in logs
- Restart container if needed

### Dashboard Not Loading
- Verify service is running: `curl http://localhost:8080/health`
- Check firewall/network access
- Check browser console for errors

## Security

- Runs as non-root user (1000) with minimal capabilities
- Read-only D-Bus socket mount
- TLS support with modern ciphers
- Regular security scanning (Checkov, Trivy)
- All dependencies tracked in go.mod with checksums

## Performance

- **CPU**: ~0.1-0.2 cores idle, ~0.5 cores under load
- **Memory**: 64-128MB typical
- **Throughput**: Thousands of requests/sec to health endpoint
- **Latency**: <100ms for typical health checks

## CI/CD

GitHub Actions pipeline on self-hosted runners:

**PR Pipeline:** Format check → Lint → Tests → Build → Docker scan
**Merge Pipeline:** All PR checks → Multi-arch build → Tag with commit SHA → Push to registry

See `.github/workflows/main.yml` for details.

## Contributing

1. Fork repository
2. Create feature branch
3. Make changes and run `make pull_request`
4. Commit and push
5. Create pull request

## License

Apache License 2.0 - See [LICENSE](LICENSE)

## Author

Alex Freidah  
Email: alex.freidah@gmail.com

## Links

- **Repository**: https://github.com/afreidah/health-check-service
- **Issues**: https://github.com/afreidah/health-check-service/issues
- **Documentation**: https://docs.claude.com (for API/SDK info)
