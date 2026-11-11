terraform {
  required_version = ">= 1.6"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.50"
    }
  }

  # Bucket, Region und Endpoint kommen per `terraform init -backend-config=backend.hcl`
  # (siehe infra/README.md); der Bucket wird von infra/bootstrap angelegt.
  backend "s3" {
    key = "hetzner/terraform.tfstate"

    # Hetzner Object Storage ist S3-kompatibel, aber kein AWS
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_s3_checksum            = true
    use_path_style              = true
  }
}

provider "hcloud" {
  token = var.hcloud_token
}
