# -----------------------------------------------------------------------
# Terraform Variables - Kubernetes Health Checker
# -----------------------------------------------------------------------
#
# All configurable inputs for the health-checker deployment.
#
# -----------------------------------------------------------------------

variable "namespace" {
  type        = string
  default     = "infra"
  description = "Kubernetes namespace for deployment"

  validation {
    condition     = length(var.namespace) > 0
    error_message = "Namespace must not be empty."
  }
}

variable "app_name" {
  type        = string
  default     = "health-checker"
  description = "Application name for labels and resource naming"

  validation {
    condition     = length(var.app_name) > 0
    error_message = "App name must not be empty."
  }
}

variable "image_repo" {
  type        = string
  default     = "docker-mirror.service.consul:5000/health-checker"
  description = "Container image repository"

  validation {
    condition     = can(regex("^[^\\s]+$", var.image_repo))
    error_message = "Image repo must be non-empty without spaces."
  }
}

variable "image_tag" {
  type        = string
  default     = "latest"
  description = "Container image tag. Use image digest for production."

  validation {
    condition     = length(var.image_tag) > 0
    error_message = "Image tag must not be empty."
  }
}

variable "image_pull_policy" {
  type        = string
  default     = "IfNotPresent"
  description = "Image pull policy. Use 'Always' in production or with digest."

  validation {
    condition     = contains(["Always", "IfNotPresent", "Never"], var.image_pull_policy)
    error_message = "Must be Always, IfNotPresent, or Never."
  }
}

variable "container_port" {
  type        = number
  default     = 18081
  description = "Container HTTP port"

  validation {
    condition     = var.container_port > 0 && var.container_port < 65536
    error_message = "Port must be between 1-65535."
  }
}

variable "container_args" {
  type        = list(string)
  default     = ["--service", "k3s", "--port", "18081", "--interval", "10"]
  description = "Container entrypoint arguments"
}

variable "replicas" {
  type        = number
  default     = 1
  description = "Number of pod replicas. For multi-replica: set max_surge=1, max_unavailable=0"

  validation {
    condition     = var.replicas >= 1 && var.replicas <= 10
    error_message = "Replicas must be 1-10."
  }
}

variable "max_surge" {
  type        = string
  default     = "0"
  description = "Max pods created during rolling update. Use '1' for multi-replica."
}

variable "max_unavailable" {
  type        = string
  default     = "1"
  description = "Max unavailable pods during rolling update. Use '0' for multi-replica."
}

variable "cpu_request" {
  type        = string
  default     = "200m"
  description = "CPU request per pod"
}

variable "cpu_limit" {
  type        = string
  default     = "500m"
  description = "CPU limit per pod"
}

variable "memory_request" {
  type        = string
  default     = "128Mi"
  description = "Memory request per pod"
}

variable "memory_limit" {
  type        = string
  default     = "256Mi"
  description = "Memory limit per pod"
}

variable "hostport_enabled" {
  type        = bool
  default     = true
  description = "Expose container port on host (requires single replica per node)"
}

variable "hostport_port" {
  type        = number
  default     = 18081
  description = "Host port to bind when hostport_enabled=true"

  validation {
    condition     = var.hostport_port > 0 && var.hostport_port < 65536
    error_message = "Port must be between 1-65535."
  }
}

variable "service_type" {
  type        = string
  default     = "ClusterIP"
  description = "Service type (ClusterIP, NodePort, LoadBalancer)"

  validation {
    condition     = contains(["ClusterIP", "NodePort", "LoadBalancer"], var.service_type)
    error_message = "Must be ClusterIP, NodePort, or LoadBalancer."
  }
}

variable "network_policy_enabled" {
  type        = bool
  default     = true
  description = "Enable NetworkPolicy for restricted access. Requires CNI plugin support."
}

variable "traefik_namespace" {
  type        = string
  default     = "kube-system"
  description = "Kubernetes namespace where Traefik runs (for NetworkPolicy)"
}

variable "pdb_enabled" {
  type        = bool
  default     = true
  description = "Enable PodDisruptionBudget to prevent eviction during node maintenance"
}

variable "max_unavailable_pdb" {
  type        = number
  default     = 0
  description = "Max unavailable pods for PDB. Keep 0 for single-replica deployments."
}

variable "ingress_enabled" {
  type        = bool
  default     = true
  description = "Enable Ingress for external access"
}

variable "ingress_class" {
  type        = string
  default     = "traefik"
  description = "IngressClassName to use (traefik, nginx, etc.)"
}

variable "ingress_host" {
  type        = string
  default     = "health.example.com"
  description = "Hostname for Ingress. Update to your domain."

  validation {
    condition     = can(regex("^[a-zA-Z0-9.-]+$", var.ingress_host))
    error_message = "Ingress host must be a valid hostname."
  }
}
