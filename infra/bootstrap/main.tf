# Einmaliger Bootstrap: legt den privaten Bucket für den Terraform-State von
# infra/hetzner/ an (Hetzner Object Storage, S3-kompatibel). Läuft mit LOKALEM
# State vom Laptop aus, siehe infra/README.md.
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

# Credentials über AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY (S3-Key aus der Hetzner-Konsole)
provider "aws" {
  region = var.region

  endpoints {
    s3 = "https://${var.region}.your-objectstorage.com"
  }

  # Hetzner ist kein AWS: keine STS-/IAM-/Metadata-Aufrufe
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

# Aufbewahrung (30 Tage) erledigt backup_db.sh per rclone, keine Bucket-Lifecycle-Regel nötig
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
