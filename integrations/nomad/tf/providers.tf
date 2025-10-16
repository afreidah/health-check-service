/*
-------------------------------------------------------------------------------
 Terraform & Provider Requirements - Nomad Module
-------------------------------------------------------------------------------
 Specifies the Terraform version and required Nomad provider for this module.
 Also exposes provider configuration via variables for flexibility.
-------------------------------------------------------------------------------
*/

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    nomad = {
      source  = "hashicorp/nomad"
      version = ">= 1.4.0"
    }
  }
}

# -----------------------------------------------------------------------------
# Nomad Provider Configuration
# -----------------------------------------------------------------------------
# Configure via variables (address/token), or use a provider block override at
# the root module if you need multiple Nomad clusters/environments.
# -----------------------------------------------------------------------------

provider "nomad" {
  address = var.nomad_address
  token   = var.nomad_token
}
