# -----------------------------------------------------------------------
# Terraform Variables - Nomad Health Checker
# -----------------------------------------------------------------------
#
# All configurable inputs for the health-checker job.
#
# -----------------------------------------------------------------------

variable "nomad_address" {
  type        = string
  description = "Nomad server address"
  default     = "http://127.0.0.1:4646"

  validation {
    condition     = can(regex("^https?://", var.nomad_address))
    error_message = "Must be http(s) URL."
  }
}

variable "nomad_token" {
  type        = string
  description = "Nomad ACL token (optional)"
  default     = null
  sensitive   = true
}

variable "job_name" {
  type        = string
  default     = "health-checker"
  description = "Nomad job name"

  validation {
    condition     = length(var.job_name) > 0
    error_message = "Job name must not be empty."
  }
}

variable "region" {
  type        = string
  default     = "global"
  description = "Nomad region for job scheduling"
}

variable "datacenters" {
  type        = list(string)
  default     = ["dc1"]
  description = "Nomad datacenters where job can run"

  validation {
    condition     = length(var.datacenters) >= 1
    error_message = "Provide at least one datacenter."
  }
}

variable "node_pool" {
  type        = string
  default     = "all"
  description = "Nomad node pool"
}

variable "group_name" {
  type        = string
  default     = "app"
  description = "Task group name"
}

variable "task_name" {
  type        = string
  default     = "health-checker"
  description = "Task name"
}

variable "task_count" {
  type        = number
  default     = 1
  description = "Number of task instances"

  validation {
    condition     = var.task_count >= 1 && var.task_count <= 50
    error_message = "Task count must be 1-50."
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
  description = "Container image tag"

  validation {
    condition     = length(var.image_tag) > 0
    error_message = "Image tag must not be empty."
  }
}

variable "args" {
  type        = list(string)
  default     = ["--service", "nginx", "--port", "8080", "--interval", "10"]
  description = "Container entrypoint arguments"
}

variable "use_docker_logging" {
  type        = bool
  default     = true
  description = "Enable Docker logging driver configuration"
}

variable "logging_driver" {
  type        = string
  default     = "journald"
  description = "Docker logging driver"

  validation {
    condition     = contains(["journald", "json-file", "splunk", "awslogs"], var.logging_driver)
    error_message = "Must be journald, json-file, splunk, or awslogs."
  }
}

variable "logging_tag" {
  type        = string
  default     = "health-checker"
  description = "Logging tag"
}

variable "mount_dbus_socket" {
  type        = bool
  default     = true
  description = "Mount D-Bus socket"
}

variable "host_port" {
  type        = number
  default     = 18080
  description = "Static host port"

  validation {
    condition     = var.host_port > 0 && var.host_port < 65536
    error_message = "Port must be 1-65535."
  }
}

variable "container_port" {
  type        = number
  default     = 8080
  description = "Container port"

  validation {
    condition     = var.container_port > 0 && var.container_port < 65536
    error_message = "Port must be 1-65535."
  }
}

variable "service_name" {
  type        = string
  default     = "health-checker"
  description = "Consul service name"

  validation {
    condition     = length(var.service_name) > 0
    error_message = "Service name must not be empty."
  }
}

variable "health_path" {
  type        = string
  default     = "/health"
  description = "Health check path"
}

variable "health_interval" {
  type        = string
  default     = "15s"
  description = "Health check interval"
}

variable "health_timeout" {
  type        = string
  default     = "3s"
  description = "Health check timeout"
}

variable "ingress_host" {
  type        = string
  default     = "health.example.com"
  description = "Hostname for Traefik routing"

  validation {
    condition     = can(regex("^[a-zA-Z0-9.-]+$", var.ingress_host))
    error_message = "Host must be valid hostname."
  }
}

variable "traefik_entrypoints" {
  type        = string
  default     = "websecure"
  description = "Traefik entrypoints"
}

variable "traefik_tls" {
  type        = bool
  default     = true
  description = "Enable TLS"
}

variable "traefik_middleware" {
  type        = string
  default     = "dashboard-allowlan@file"
  description = "Traefik middleware"
}

variable "extra_service_tags" {
  type        = list(string)
  default     = ["monitoring=true", "lang=go"]
  description = "Additional service tags"
}

variable "cpu" {
  type        = number
  default     = 200
  description = "CPU in MHz"
}

variable "memory" {
  type        = number
  default     = 128
  description = "Memory in MB"
}

variable "restart_attempts" {
  type        = number
  default     = 2
  description = "Restart attempts"
}

variable "restart_interval" {
  type        = string
  default     = "30s"
  description = "Restart interval"
}

variable "restart_delay" {
  type        = string
  default     = "5s"
  description = "Restart delay"
}

variable "restart_mode" {
  type        = string
  default     = "fail"
  description = "Restart mode"

  validation {
    condition     = contains(["fail", "delay"], var.restart_mode)
    error_message = "Must be 'fail' or 'delay'."
  }
}

variable "canary_count" {
  type        = number
  default     = 0
  description = "Canary instances for deployments"

  validation {
    condition     = var.canary_count >= 0 && var.canary_count <= 5
    error_message = "Canary count must be 0-5."
  }
}
