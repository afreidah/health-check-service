# -----------------------------------------------------------------------
# Nomad Job - Health Checker (Terraform-managed)
# -----------------------------------------------------------------------
#
# Renders a Nomad jobspec from module variables and registers it.
#
# NOTE: This is a simplified version. For the complete main.tf with all
# resource definitions, see the artifacts or the original files.
#
# -----------------------------------------------------------------------

locals {
  traefik_tags = [
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

  service_tags = concat(local.traefik_tags, var.extra_service_tags)

  docker_volumes = var.mount_dbus_socket ? [
    "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket:ro",
  ] : []

  logging_block_rendered = <<-HCL
    logging {
      type = "${var.logging_driver}"
      config {
        tag = "${var.logging_tag}"
      }
    }
  HCL

  logging_block = var.use_docker_logging ? local.logging_block_rendered : ""
}

resource "nomad_job" "this" {
  jobspec = <<-EOT
    job "${var.job_name}" {
      region      = "${var.region}"
      datacenters = ${jsonencode(var.datacenters)}
      node_pool   = "${var.node_pool}"
      type        = "service"

      group "${var.group_name}" {
        count = ${var.task_count}

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

          resources {
            cpu    = ${var.cpu}
            memory = ${var.memory}
          }

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
            canary            = ${var.canary_count}
          }
        }
      }
    }
  EOT

  deregister_on_destroy   = true
  deregister_on_id_change = true
}
