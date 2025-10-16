/*
-------------------------------------------------------------------------------
 Module Variables - Nomad Health Checker
-------------------------------------------------------------------------------
 Organized by:
   - Provider configuration
   - Job placement & identity
   - Container image & runtime
   - Networking & ports
   - Traefik routing & service registration
   - Health checks & restart policy
-------------------------------------------------------------------------------
*/

# -----------------------------------------------------------------------------
# Provider Configuration
# -----------------------------------------------------------------------------

variable "nomad_address" {
  type        = string
  description = "Nomad server address (e.g., http://nomad.service.consul:4646)."
  default     = "http://127.0.0.1:4646"

  validation {
    condition     = can(regex("^https?://", var.nomad_address))
    error_message = "nomad_address must be an http(s) URL, e.g., http://host:4646."
  }
}

variable "nomad_token" {
  type        = string
  description = "Nomad ACL token (optional)."
  default     = null
}

# -----------------------------------------------------------------------------
# Job Placement & Identity
# -----------------------------------------------------------------------------

variable "job_name" {
  type        = string
  description = "Nomad job name."
  default     = "health-checker"

  validation {
    condition     = length(var.job_name) > 0
    error_message = "job_name must not be empty."
  }
}

variable "region" {
  type        = string
  description = "Nomad region for the job."
  default     = "global"
}

variable "datacenters" {
  type        = list(string)
  description = "Nomad datacenters where the job is eligible to run."
  default     = ["pi-dc"]

  validation {
    condition     = length(var.datacenters) >= 1
    error_message = "Provide at least one datacenter."
  }
}

variable "node_pool" {
  type        = string
  description = "Nomad node pool constraint (optional)."
  default     = "all"
}

variable "group_name" {
  type        = string
  description = "Nomad task group name."
  default     = "app"
}

variable "task_name" {
  type        = string
  description = "Nomad task name."
  default     = "health-checker"
}

variable "count" {
  type        = number
  description = "Number of task instances."
  default     = 1

  validation {
    condition     = var.count >= 1 && var.count <= 50
    error_message = "count must be between 1 and 50."
  }
}

# -----------------------------------------------------------------------------
# Container Image & Runtime
# -----------------------------------------------------------------------------

variable "image_repo" {
  type        = string
  description = "Container image repository."
  default     = "docker-mirror.service.consul:5000/health-checker"

  validation {
    condition     = can(regex("^[^\\s]+$", var.image_repo))
    error_message = "image_repo must be a non-empty image reference (no spaces)."
  }
}

variable "image_tag" {
  type        = string
  description = "Container image tag."
  default     = "latest"

  validation {
    condition     = length(var.image_tag) > 0
    error_message = "image_tag must not be empty."
  }
}

variable "args" {
  type        = list(string)
  description = "Command arguments for the container."
  default     = ["--service", "k3s", "--port", "18080", "--interval", "10"]
}

variable "use_journald_logging" {
  type        = bool
  description = "Enable Docker journald logging driver with tag."
  default     = true
}

variable "journald_tag" {
  type        = string
  description = "journald log tag when use_journald_logging is true."
  default     = "health-checker"
}

variable "mount_dbus_socket" {
  type        = bool
  description = "Bind /var/run/dbus/system_bus_socket into container as read-only."
  default     = true
}

# -----------------------------------------------------------------------------
# Networking & Ports
# -----------------------------------------------------------------------------

variable "host_port" {
  type        = number
  description = "Static host port to publish (Nomad network.port \"http\")."
  default     = 18080

  validation {
    condition     = var.host_port > 0 && var.host_port < 65536
    error_message = "host_port must be between 1 and 65535."
  }
}

variable "container_port" {
  type        = number
  description = "Container's internal HTTP port."
  default     = 8080

  validation {
    condition     = var.container_port > 0 && var.container_port < 65536
    error_message = "container_port must be between 1 and 65535."
  }
}

# -----------------------------------------------------------------------------
# Traefik Routing & Service Registration
# -----------------------------------------------------------------------------

variable "service_name" {
  type        = string
  description = "Nomad/Consul service name to register."
  default     = "health-checker"

  validation {
    condition     = length(var.service_name) > 0
    error_message = "service_name must not be empty."
  }
}

variable "ingress_host" {
  type        = string
  description = "Traefik router host (internal DNS name)."
  default     = "health.munchbox"

  validation {
    condition     = can(regex("^[a-zA-Z0-9.-]+$", var.ingress_host))
    error_message = "ingress_host must be a valid hostname (letters, numbers, dashes, dots)."
  }
}

variable "traefik_entrypoints" {
  type        = string
  description = "Traefik entrypoints (comma-separated if multiple)."
  default     = "websecure"
}

variable "traefik_tls" {
  type        = bool
  description = "Enable TLS on the Traefik router."
  default     = true
}

variable "traefik_middleware" {
  type        = string
  description = "Traefik middleware reference for LAN restriction."
  default     = "dashboard-allowlan@file"
}

variable "extra_service_tags" {
  type        = list(string)
  description = "Additional service tags for metadata or routing."
  default     = ["go", "health", "monitoring"]
}

# -----------------------------------------------------------------------------
# Health Checks & Restart Policy
# -----------------------------------------------------------------------------

variable "health_path" {
  type        = string
  description = "HTTP path for Nomad health check."
  default     = "/health"
}

variable "health_interval" {
  type        = string
  description = "Interval for Nomad service check."
  default     = "15s"
}

variable "health_timeout" {
  type        = string
  description = "Timeout for Nomad service check."
  default     = "3s"
}

variable "restart_attempts" {
  type        = number
  description = "Restart attempts before marking as failed."
  default     = 2
}

variable "restart_interval" {
  type        = string
  description = "Time window for restart attempts."
  default     = "30s"
}

variable "restart_delay" {
  type        = string
  description = "Delay between restarts."
  default     = "5s"
}

variable "restart_mode" {
  type        = string
  description = "Restart mode (\"fail\" or \"delay\")."
  default     = "fail"

  validation {
    condition     = contains(["fail", "delay"], var.restart_mode)
    error_message = "restart_mode must be one of: fail, delay."
  }
}

variable "cpu" {
  type        = number
  description = "Nomad CPU shares for the task."
  default     = 200
}

variable "memory" {
  type        = number
  description = "Nomad memory (MB) for the task."
  default     = 128
}

