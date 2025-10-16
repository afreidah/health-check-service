/*
-------------------------------------------------------------------------------
 Terraform & Provider Requirements
-------------------------------------------------------------------------------
 Specifies the Terraform version and required providers for this module.
 Also configures the Kubernetes provider using the default kubeconfig.
-------------------------------------------------------------------------------
*/

terraform {
  # Require Terraform 1.6.0 or newer
  required_version = ">= 1.6.0"

  # Define required providers and their sources
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30" # Use Kubernetes provider 2.30.x
    }
  }
}

# -----------------------------------------------------------------------------
# Kubernetes Provider Configuration
# -----------------------------------------------------------------------------
# Uses the local kubeconfig for authentication (e.g., k3s or other local cluster)
# To override config_path or config_context, pass values in a provider block.
# For remote usage, override this configuration using provider aliasing or
# environment-based kubeconfig management.
# -----------------------------------------------------------------------------

provider "kubernetes" {
  config_path    = "~/.kube/config" # Default path to kubeconfig
  config_context = null             # Use current context (optional override)
}
