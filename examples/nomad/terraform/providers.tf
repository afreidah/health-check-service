# -----------------------------------------------------------------------
# Terraform & Provider Configuration
# -----------------------------------------------------------------------
#
# Specifies Terraform version and required Nomad provider.
#
# -----------------------------------------------------------------------

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    nomad = {
      source  = "hashicorp/nomad"
      version = ">= 1.4.0"
    }
  }
}

provider "nomad" {
  address   = var.nomad_address
  secret_id = var.nomad_token
}
