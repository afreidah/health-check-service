# -----------------------------------------------------------------------
# Nomad Job Specification - Health Checker
# -----------------------------------------------------------------------
#
# Deploys a containerized health-checker service via Docker driver.
# Monitors systemd services and registers with Consul for service discovery.
# Includes Traefik tags for internal HTTPS routing with LAN restriction.
#
# -----------------------------------------------------------------------

job "health-checker" {
  region      = "global"
  datacenters = ["dc1"]
  node_pool   = "all"
  type        = "service"

  group "app" {
    count = 1

    network {
      port "http" {
        static = 18080
        to     = 8080
      }
    }

    task "health-checker" {
      driver = "docker"

      config {
        image   = "docker-mirror.service.consul:5000/health-checker:latest"
        ports   = ["http"]
        volumes = [
          "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro"
        ]

        logging {
          type = "journald"
          config {
            tag = "health-checker"
          }
        }

        args = ["--service", "nginx", "--port", "8080", "--interval", "10"]
      }

      resources {
        cpu    = 200
        memory = 128
      }

      restart {
        attempts = 2
        interval = "30s"
        delay    = "5s"
        mode     = "fail"
      }

      service {
        name = "health-checker"
        port = "http"

        check {
          type     = "http"
          path     = "/health"
          interval = "15s"
          timeout  = "3s"
        }

        tags = [
          "traefik.enable=true",
          "traefik.http.routers.health.rule=Host(`health.munchbox`)",
          "traefik.http.routers.health.entrypoints=websecure",
          "traefik.http.routers.health.tls=true",
          "traefik.http.routers.health.middlewares=dashboard-allowlan@file",
          "traefik.http.services.health.loadbalancer.server.port=18080",
          "traefik.http.services.health.loadbalancer.server.scheme=http",
          "traefik.http.services.health.loadbalancer.healthcheck.path=/health",
          "traefik.http.services.health.loadbalancer.healthcheck.interval=30s",
          "traefik.http.services.health.loadbalancer.healthcheck.timeout=5s",
          "monitoring=true",
          "lang=go",
        ]
      }

      update {
        max_parallel      = 1
        health_check      = "checks"
        min_healthy_time  = "10s"
        healthy_deadline  = "3m"
        progress_deadline = "10m"
        auto_revert       = true
        canary            = 0
      }
    }
  }
}
