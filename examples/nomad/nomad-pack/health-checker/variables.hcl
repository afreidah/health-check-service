# -----------------------------------------------------------------------
# Nomad Pack Variables - Health Checker
# -----------------------------------------------------------------------

# Placement & Identity
variable "job_name" {
  type    = string
  default = "health-checker"
  validation {
    condition     = length(var.job_name) > 0 && length(var.job_name) <= 128
    error_message = "Job name must be 1-128 characters."
  }
}

variable "region" {
  type    = string
  default = "global"
}

variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
  validation {
    condition     = length(var.datacenters) >= 1
    error_message = "Must specify at least one datacenter."
  }
}

variable "node_pool" {
  type    = string
  default = "all"
}

variable "task_count" {
  type    = number
  default = 1
  validation {
    condition     = var.task_count >= 1 && var.task_count <= 50
    error_message = "Task count must be 1-50."
  }
}

# Container Image
variable "image_repo" {
  type    = string
  default = "docker-mirror.service.consul:5000/health-checker"
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9/.-]*[a-z0-9])?$", var.image_repo))
    error_message = "Must be valid container image repository."
  }
}

variable "image_tag" {
  type    = string
  default = "latest"
}

# Health Checker Configuration
variable "service_to_monitor" {
  type    = string
  default = "nginx"
  validation {
    condition     = length(var.service_to_monitor) > 0
    error_message = "Service name must not be empty."
  }
}

variable "check_interval" {
  type    = number
  default = 10
  validation {
    condition     = var.check_interval >= 1 && var.check_interval <= 3600
    error_message = "Check interval must be 1-3600 seconds."
  }
}

variable "container_port" {
  type    = number
  default = 8080
  validation {
    condition     = var.container_port > 1024 && var.container_port < 65536
    error_message = "Port must be 1025-65535."
  }
}

variable "host_port" {
  type    = number
  default = 18080
  validation {
    condition     = var.host_port > 0 && var.host_port < 65536
    error_message = "Host port must be 1-65535."
  }
}

# Docker Logging
variable "log_driver" {
  type    = string
  default = "journald"
  validation {
    condition     = contains(["journald", "json-file", "splunk", "awslogs", "awsfirelens"], var.log_driver)
    error_message = "Must be journald, json-file, splunk, awslogs, or awsfirelens."
  }
}

variable "log_tag" {
  type    = string
  default = "health-checker"
}

# D-Bus
variable "mount_dbus" {
  type    = bool
  default = true
}

# Service Discovery
variable "service_name" {
  type    = string
  default = "health-checker"
  validation {
    condition     = length(var.service_name) > 0
    error_message = "Service name must not be empty."
  }
}

variable "service_tags" {
  type    = list(string)
  default = ["monitoring", "systemd-health", "lang:go"]
}

# Health Checks
variable "nomad_health_check_enabled" {
  type    = bool
  default = true
}

variable "nomad_health_interval" {
  type    = string
  default = "15s"
}

variable "nomad_health_timeout" {
  type    = string
  default = "3s"
}

# Traefik
variable "traefik_enabled" {
  type    = bool
  default = true
}

variable "traefik_host" {
  type    = string
  default = "health.example.com"
}

variable "traefik_entrypoints" {
  type    = string
  default = "websecure"
}

variable "traefik_tls" {
  type    = bool
  default = true
}

variable "traefik_middleware" {
  type    = string
  default = ""
}

# Resources
variable "cpu" {
  type    = number
  default = 200
  validation {
    condition     = var.cpu >= 10 && var.cpu <= 4000
    error_message = "CPU must be 10-4000 MHz."
  }
}

variable "memory" {
  type    = number
  default = 128
  validation {
    condition     = var.memory >= 32 && var.memory <= 2048
    error_message = "Memory must be 32-2048 MB."
  }
}

# Restart Policy
variable "restart_attempts" {
  type    = number
  default = 2
  validation {
    condition     = var.restart_attempts >= 0 && var.restart_attempts <= 5
    error_message = "Restart attempts must be 0-5."
  }
}

variable "restart_mode" {
  type    = string
  default = "fail"
  validation {
    condition     = contains(["fail", "delay"], var.restart_mode)
    error_message = "Must be 'fail' or 'delay'."
  }
}

variable "restart_delay" {
  type    = string
  default = "5s"
}

variable "restart_interval" {
  type    = string
  default = "30s"
}

# Update Strategy
variable "update_strategy" {
  type    = string
  default = "rolling"
  validation {
    condition     = contains(["rolling", "canary"], var.update_strategy)
    error_message = "Must be 'rolling' or 'canary'."
  }
}

variable "canary_count" {
  type    = number
  default = 1
  validation {
    condition     = var.canary_count >= 0 && var.canary_count <= 5
    error_message = "Canary count must be 0-5."
  }
}

# Labels
variable "labels" {
  type = map(string)
  default = {
    "app"     = "health-checker"
    "version" = "1.0.0"
  }
}
