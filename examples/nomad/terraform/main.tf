/*
-------------------------------------------------------------------------------
 Nomad Job - Health Checker (Terraform-managed)
-------------------------------------------------------------------------------
 Renders a Nomad jobspec from module variables and registers it via nomad_job.
 Mirrors the original intent:
   - Single Dockerized task
   - Host /var/run/dbus bind-mounted (optional)
   - Container port -> published static node port
   - Service registration with Traefik tags (internal-only)
   - Health checks and restart policy
-------------------------------------------------------------------------------
*/

# -----------------------------------------------------------------------------
# Service Tags (Traefik + metadata)
# -----------------------------------------------------------------------------
locals {
  base_traefik_tags = [
    "traefik.enable=true",
    "traefik.http.routers.health.rule=Host(`${var.ingress_host}`)",
    "traefik.http.routers.health.entrypoints=${var.traefik_entrypoints}",
    "traefik.http.routers.health.tls=${tostring(var.traefik_tls)}",
    "traefik.http.routers.health.middlewares=${var.traefik_middleware}",
    "traefik.http.services.health.loadbalancer.server.port=${var.host_port}",
    "traefik.http.services.health.loadbalancer.server.scheme=http",
    "traefik.http.services.health.loadbalancer.healthcheck.path=${var.health_path}",
    "traefik.http.services.health.loadbalancer.healthcheck.interval=${var.health_interval}",
    "traefik.http.services.health.loadbalancer.healthcheck.timeout=${var.health_timeout}",
  ]

  service_tags = concat(local.base_traefik_tags, var.extra_service_tags)

  docker_volumes = var.mount_dbus_socket ? [
    "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro",
  ] : []

  # --- Heredoc rendered once, gated by boolean separately ---
  logging_block_rendered = <<-HCL
    logging {
      type = "journald"
      config {
        tag = "${var.journald_tag}"
      }
    }
  HCL

  logging_block = var.use_journald_logging ? local.logging_block_rendered : ""
}

# -----------------------------------------------------------------------------
# Nomad Job Resource
# -----------------------------------------------------------------------------
resource "nomad_job" "this" {
  jobspec = <<-EOT
    job "${var.job_name}" {
      region      = "${var.region}"
      datacenters = ${jsonencode(var.datacenters)}
      node_pool   = "${var.node_pool}"
      type        = "service"

      group "${var.group_name}" {
        count = ${var.task_count}

        # --- Networking (publish node port static, map to container port) ---
        network {
          port "http" {
            static = ${var.host_port}
            to     = ${var.container_port}
          }
        }

        task "${var.task_name}" {
          driver = "docker"

          config {
            image   = "${var.image_repo}:${var.image_tag}"
            ports   = ["http"]
            volumes = ${jsonencode(local.docker_volumes)}
            args    = ${jsonencode(var.args)}
            ${local.logging_block}
          }

          # --- Resources ---
          resources {
            cpu    = ${var.cpu}
            memory = ${var.memory}
          }

          # --- Service Registration (Traefik + Health) ---
          service {
            name = "${var.service_name}"
            port = "http"

            check {
              type     = "http"
              path     = "${var.health_path}"
              interval = "${var.health_interval}"
              timeout  = "${var.health_timeout}"
            }

            tags = ${jsonencode(local.service_tags)}
          }

          # --- Restart Policy ---
          restart {
            attempts = ${var.restart_attempts}
            interval = "${var.restart_interval}"
            delay    = "${var.restart_delay}"
            mode     = "${var.restart_mode}"
          }
        }
      }
    }
  EOT

  deregister_on_destroy   = true
  deregister_on_id_change = true
}

