# -----------------------------------------------------------------------
# Kubernetes Deployment Resources - Health Checker
# -----------------------------------------------------------------------
#
# Defines all core Kubernetes resources: Namespace, Deployment, Service,
# NetworkPolicy, PodDisruptionBudget, and optional Ingress.
#
# NOTE: This is a simplified version. For the complete main.tf with all
# resource definitions, see the artifacts or the original files.
#
# -----------------------------------------------------------------------

resource "kubernetes_namespace" "ns" {
  metadata {
    name = var.namespace
  }
}

resource "kubernetes_deployment" "app" {
  metadata {
    name      = var.app_name
    namespace = kubernetes_namespace.ns.metadata[0].name
    labels = {
      app = var.app_name
    }
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = {
        app = var.app_name
      }
    }

    strategy {
      type = "RollingUpdate"
      rolling_update {
        max_surge       = var.max_surge
        max_unavailable = var.max_unavailable
      }
    }

    template {
      metadata {
        labels = {
          app = var.app_name
        }
      }

      spec {
        security_context {
          run_as_non_root = true
          run_as_user     = 1000
          run_as_group    = 1000
          supplemental_groups = [106]
        }

        container {
          name              = var.app_name
          image             = "${var.image_repo}:${var.image_tag}"
          image_pull_policy = var.image_pull_policy

          args = var.container_args

          port {
            name           = "http"
            container_port = var.container_port
            host_port      = var.hostport_enabled ? var.hostport_port : null
            protocol       = "TCP"
          }

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

          resources {
            limits = {
              cpu    = var.cpu_limit
              memory = var.memory_limit
            }
            requests = {
              cpu    = var.cpu_request
              memory = var.memory_request
            }
          }

          security_context {
            allow_privilege_escalation = false
          }

          volume_mount {
            name       = "dbus-socket"
            mount_path = "/var/run/dbus/system_bus_socket"
            read_only  = true
          }
        }

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
    }
  }
}

resource "kubernetes_pod_disruption_budget_v1" "pdb" {
  count = var.pdb_enabled ? 1 : 0

  metadata {
    name      = var.app_name
    namespace = kubernetes_namespace.ns.metadata[0].name
  }

  spec {
    max_unavailable = var.max_unavailable_pdb

    selector {
      match_labels = {
        app = var.app_name
      }
    }
  }
}

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
    ingress_class_name = var.ingress_class

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

    tls {
      hosts = [var.ingress_host]
    }
  }
}
