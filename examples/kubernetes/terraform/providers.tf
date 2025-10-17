# -----------------------------------------------------------------------
# Terraform & Provider Configuration
# -----------------------------------------------------------------------
#
# Specifies Terraform version and required Kubernetes provider.
# Uses local kubeconfig for authentication.
#
# -----------------------------------------------------------------------

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
  }
}

provider "kubernetes" {
  config_path    = "~/.kube/config"
  config_context = null  # Uses current-context
}
