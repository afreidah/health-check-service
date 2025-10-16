/*
-------------------------------------------------------------------------------
 Terraform Variables - Health Checker Service
-------------------------------------------------------------------------------
 This file defines the configurable inputs for the Health Checker application.
 Organized by:
   - Metadata and naming
   - Container image configuration
   - Networking (ports, ingress, service exposure)
-------------------------------------------------------------------------------
*/

# -----------------------------------------------------------------------------
# Metadata and Naming
# -----------------------------------------------------------------------------

variable "namespace" {
  type    = string
  default = "infra"

  validation {
    condition     = length(var.namespace) > 0
    error_message = "Namespace must not be empty."
  }
}

variable "app_name" {
  type    = string
  default = "health-checker"

  validation {
    condition     = length(var.app_name) > 0
    error_message = "App name must not be empty."
  }
}

# -----------------------------------------------------------------------------
# Container Image Configuration
# -----------------------------------------------------------------------------

variable "image_repo" {
  type    = string
  default = "docker-mirror.service.consul:5000/health-checker"

  validation {
    condition     = can(regex("^[^\\s]+$", var.image_repo))
    error_message = "Image repo must be a non-empty, valid string without spaces."
  }
}

variable "image_tag" {
  type    = string
  default = "latest"

  validation {
    condition     = length(var.image_tag) > 0
    error_message = "Image tag must not be empty."
  }
}

# -----------------------------------------------------------------------------
# Port and HostPort Configuration
# -----------------------------------------------------------------------------

variable "container_port" {
  type    = number
  default = 18081

  validation {
    condition     = var.container_port > 0 && var.container_port < 65536
    error_message = "Container port must be between 1 and 65535."
  }
}

variable "hostport_enabled" {
  type    = bool
  default = true
}

variable "hostport_port" {
  type    = number
  default = 18081

  validation {
    condition     = var.hostport_port > 0 && var.hostport_port < 65536
    error_message = "HostPort must be between 1 and 65535."
  }
}

# -----------------------------------------------------------------------------
# Ingress and Service Exposure
# -----------------------------------------------------------------------------

variable "ingress_enabled" {
  type    = bool
  default = true
}

variable "ingress_host" {
  type    = string
  default = "health.munchbox"

  validation {
    condition     = can(regex("^[a-zA-Z0-9.-]+$", var.ingress_host))
    error_message = "Ingress host must be a valid hostname (letters, numbers, dashes, and dots)."
  }
}

variable "service_type" {
  type    = string
  default = "ClusterIP" # could be NodePort

  validation {
    condition     = contains(["ClusterIP", "NodePort", "LoadBalancer", "ExternalName"], var.service_type)
    error_message = "Service type must be one of: ClusterIP, NodePort, LoadBalancer, ExternalName."
  }
}
