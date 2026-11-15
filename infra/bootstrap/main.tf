# One-time bootstrap: creates the private bucket for the Terraform state of
# infra/hetzner/ (Hetzner Object Storage, S3-compatible). Runs with LOCAL
# state from a laptop, see infra/README.md.
terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

variable "bucket" {
  description = "Name des State-Buckets (in Hetzner Object Storage eindeutig)"
  type        = string
  default     = "soziodata-tfstate"
}

variable "backup_bucket" {
  description = "Name des Buckets für die nächtlichen DB-Backups (Snapshots + Metadaten), siehe backend/scripts/backup_db.sh"
  type        = string
  default     = "soziodata-backup"
}

variable "region" {
  description = "Object-Storage-Location, z. B. fsn1, nbg1 oder hel1"
  type        = string
  default     = "fsn1"
}

# Credentials via AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY (S3 key from the Hetzner console)
provider "aws" {
  region = var.region

  endpoints {
    s3 = "https://${var.region}.your-objectstorage.com"
  }

  # Hetzner is not AWS: no STS/IAM/metadata calls
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
  skip_region_validation      = true
  s3_use_path_style           = true
}

resource "aws_s3_bucket" "state" {
  bucket = var.bucket

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

# Retention (30 days) is handled by backup_db.sh via rclone; no bucket lifecycle rule needed
resource "aws_s3_bucket" "backup" {
  bucket = var.backup_bucket

  lifecycle {
    prevent_destroy = true
  }
}

output "bucket" {
  value = aws_s3_bucket.state.bucket
}

output "backup_bucket" {
  value = aws_s3_bucket.backup.bucket
}

output "endpoint" {
  value = "https://${var.region}.your-objectstorage.com"
}
