/*
-------------------------------------------------------------------------------
 Kubernetes Deployment Resources - Health Checker
-------------------------------------------------------------------------------
 This file defines all core Kubernetes resources:
   - Namespace
   - Deployment
   - Service
   - Optional Ingress (Traefik)
-------------------------------------------------------------------------------
*/

# -----------------------------------------------------------------------------
# Namespace Definition
# -----------------------------------------------------------------------------
resource "kubernetes_namespace" "ns" {
  metadata {
    name = var.namespace
  }
}

# -----------------------------------------------------------------------------
# Application Deployment
# -----------------------------------------------------------------------------
resource "kubernetes_deployment" "app" {
  metadata {
    name      = var.app_name
    namespace = kubernetes_namespace.ns.metadata[0].name
    labels = {
      app = var.app_name
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = var.app_name
      }
    }

    strategy {
      type = "RollingUpdate"

      rolling_update {
        max_surge       = "0"
        max_unavailable = "1"
      }
    }

    template {
      metadata {
        labels = {
          app = var.app_name
        }
      }

      spec {
        # --- Pod Security Context ---
        security_context {
          run_as_non_root = true
          run_as_user     = 1000
          run_as_group    = 1000
          # supplemental_groups = [106]  # Uncomment if needed (e.g., for D-Bus access)
        }

        # --- Application Container ---
        container {
          name              = var.app_name
          image             = "${var.image_repo}:${var.image_tag}"
          image_pull_policy = "IfNotPresent"

          args = [
            "--service", "k3s",
            "--port", tostring(var.container_port),
            "--interval", "10",
          ]

          port {
            name           = "http"
            container_port = var.container_port
            protocol       = "TCP"
            host_port      = var.hostport_enabled ? var.hostport_port : null
          }

          # --- Readiness Probe ---
          readiness_probe {
            http_get {
              path = "/health"
              port = "http"
            }
            initial_delay_seconds = 5
            period_seconds        = 10
            timeout_seconds       = 2
            failure_threshold     = 3
          }

          # --- Liveness Probe ---
          liveness_probe {
            http_get {
              path = "/health"
              port = "http"
            }
            initial_delay_seconds = 10
            period_seconds        = 20
            timeout_seconds       = 2
            failure_threshold     = 3
          }

          # --- Resource Constraints ---
          resources {
            limits = {
              cpu    = "500m"
              memory = "256Mi"
            }
            requests = {
              cpu    = "200m"
              memory = "128Mi"
            }
          }

          # --- Container Security Context ---
          security_context {
            allow_privilege_escalation = false
          }

          # --- Volume Mounts ---
          volume_mount {
            name       = "dbus-socket"
            mount_path = "/var/run/dbus/system_bus_socket"
            read_only  = true
          }
        }

        # --- Volumes ---
        volume {
          name = "dbus-socket"

          host_path {
            path = "/var/run/dbus/system_bus_socket"
            type = "Socket"
          }
        }
      }
    }
  }
}

# -----------------------------------------------------------------------------
# Service Definition
# -----------------------------------------------------------------------------
resource "kubernetes_service" "svc" {
  metadata {
    name      = var.app_name
    namespace = kubernetes_namespace.ns.metadata[0].name
    labels = {
      app = var.app_name
    }
  }

  spec {
    selector = {
      app = var.app_name
    }
    type = var.service_type

    port {
      name        = "http"
      port        = var.container_port
      target_port = "http"
      protocol    = "TCP"
      # node_port = 32081  # Uncomment if using NodePort and you want a fixed port
    }
  }
}

# -----------------------------------------------------------------------------
# Optional Ingress (Traefik)
# -----------------------------------------------------------------------------
resource "kubernetes_ingress_v1" "ing" {
  count = var.ingress_enabled ? 1 : 0

  metadata {
    name      = var.app_name
    namespace = kubernetes_namespace.ns.metadata[0].name

    annotations = {
      "traefik.ingress.kubernetes.io/router.entrypoints" = "websecure"
      "traefik.ingress.kubernetes.io/router.tls"         = "true"
    }
  }

  spec {
    ingress_class_name = "traefik"

    rule {
      host = var.ingress_host

      http {
        path {
          path      = "/"
          path_type = "Prefix"

          backend {
            service {
              name = kubernetes_service.svc.metadata[0].name

              port {
                name = "http"
              }
            }
          }
        }
      }
    }
  }
}
