terraform {
  required_version = ">= 1.6.0"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
  }
}

# Uses your local kubeconfig/context (k3s)
provider "kubernetes" {
  config_path    = "~/.kube/config"
  config_context = null
}
