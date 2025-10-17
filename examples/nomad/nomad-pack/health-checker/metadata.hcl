# -----------------------------------------------------------------------
# Nomad Pack Metadata - Health Checker
# -----------------------------------------------------------------------

pack {
  name        = "health-checker"
  description = "Systemd service health checker with D-Bus integration and Prometheus metrics"
  url         = "https://github.com/afreidah/health-check-service"
  version     = "1.0.0"

  requirements {
    nomad_version = ">= 1.4.0"
    pack_version  = ">= 0.0.1"
  }
}
