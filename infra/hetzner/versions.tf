terraform {
  required_version = ">= 1.6"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.50"
    }
  }

  # Bucket, region and endpoint are passed via `terraform init -backend-config=backend.hcl`
  # (see infra/README.md); the bucket is created by infra/bootstrap.
  backend "s3" {
    key = "hetzner/terraform.tfstate"

    # Hetzner Object Storage is S3-compatible, but not AWS
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
