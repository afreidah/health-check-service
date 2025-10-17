job "${var.job_name}" {
  region      = "${var.region}"
  datacenters = ${jsonencode(var.datacenters)}
  type        = "service"
  node_pool   = "${var.node_pool}"

  group "app" {
    count = ${var.task_count}

    network {
      port "http" {
        static = ${var.host_port}
        to     = ${var.container_port}
      }
    }

    task "health-checker" {
      driver = "docker"

      config {
        image   = "${var.image_repo}:${var.image_tag}"
        ports   = ["http"]

%{if var.mount_dbus ~}
        volumes = [
          "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro"
        ]
%{else ~}
        volumes = []
%{endif ~}

        logging {
          type = "${var.log_driver}"
          config {
            tag = "${var.log_tag}"
          }
        }

        args = [
          "--service", "${var.service_to_monitor}",
          "--port", "${var.container_port}",
          "--interval", "${var.check_interval}"
        ]
      }

      resources {
        cpu    = ${var.cpu}
        memory = ${var.memory}
      }

      service {
        name = "${var.service_name}"
        port = "http"

        tags = ${jsonencode(concat(
          var.service_tags,
          var.traefik_enabled ? [
            "traefik.enable=true",
            "traefik.http.routers.health.rule=Host(\`${var.traefik_host}\`)",
            "traefik.http.routers.health.entrypoints=${var.traefik_entrypoints}",
            "traefik.http.routers.health.tls=${tostring(var.traefik_tls)}",
            var.traefik_middleware != "" ? "traefik.http.routers.health.middlewares=${var.traefik_middleware}" : "",
            "traefik.http.services.health.loadbalancer.server.port=${var.host_port}",
            "traefik.http.services.health.loadbalancer.server.scheme=http",
            "traefik.http.services.health.loadbalancer.healthcheck.path=/health",
            "traefik.http.services.health.loadbalancer.healthcheck.interval=30s",
            "traefik.http.services.health.loadbalancer.healthcheck.timeout=5s",
          ] : []
        ))}

%{if var.nomad_health_check_enabled ~}
        check {
          type     = "http"
          path     = "/health"
          interval = "${var.nomad_health_interval}"
          timeout  = "${var.nomad_health_timeout}"
        }
%{endif ~}
      }

      restart {
        attempts = ${var.restart_attempts}
        interval = "${var.restart_interval}"
        delay    = "${var.restart_delay}"
        mode     = "${var.restart_mode}"
      }

      update {
        max_parallel      = 1
        health_check      = "checks"
        min_healthy_time  = "10s"
        healthy_deadline  = "3m"
        progress_deadline = "10m"
        auto_revert       = true
        canary            = ${var.update_strategy == "canary" ? var.canary_count : 0}
      }

      labels = var.labels
    }
  }
}
