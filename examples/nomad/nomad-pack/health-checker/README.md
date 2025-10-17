# Health Checker - Nomad Pack

Nomad Pack for deploying the health-checker service.

## Quick Start

```bash
# Default deployment (nginx)
nomad-pack run health-checker

# Monitor PostgreSQL
nomad-pack run health-checker -f examples/postgresql.hcl

# Multi-instance
nomad-pack run health-checker -f examples/multi-instance.hcl

# Development
nomad-pack run health-checker -f examples/dev.hcl
```

## Key Features

- Systemd service monitoring via D-Bus
- Consul service discovery
- Traefik HTTPS routing (optional)
- Prometheus metrics
- Canary deployments
- Auto-reconnect to D-Bus

## Variables

See `variables.hcl` for complete reference (35+ configurable options).

Common variables:
- `job_name` - Nomad job name
- `service_to_monitor` - Systemd service (default: nginx)
- `host_port` - Access port (default: 18080)
- `traefik_enabled` - Enable Traefik routing (default: true)
- `cpu` / `memory` - Resource allocation

## Outputs

After deployment, view outputs:

```bash
nomad-pack output health-checker
```

Includes:
- Health check endpoint
- Dashboard URL
- Metrics endpoint
- Service discovery info

## Examples

- **nginx.hcl** - Monitor nginx (basic)
- **postgresql.hcl** - Monitor PostgreSQL (production)
- **multi-instance.hcl** - Distributed 3-instance setup
- **dev.hcl** - Development/testing lightweight

## Documentation

Full documentation: See root pack README.md

## Support

- GitHub: https://github.com/afreidah/health-check-service
- Issues: https://github.com/afreidah/health-check-service/issues
