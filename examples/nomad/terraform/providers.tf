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
provider "nomad" {
  address   = var.nomad_address
  secret_id = var.nomad_token
}
