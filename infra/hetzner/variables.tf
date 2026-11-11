variable "hcloud_token" {
  description = "Hetzner-Cloud-API-Token (Read & Write)"
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "Öffentlicher Deploy-Key (Gegenstück zum GitHub-Secret SSH_PRIVATE_KEY)"
  type        = string
}

variable "name" {
  description = "Präfix für alle Ressourcen"
  type        = string
  default     = "soziodata"
}

variable "server_type" {
  description = "Hetzner-Servertyp. cpx12 = 1 vCPU / 2 GB RAM / 40 GB NVMe (knapp für Postgres + Pipeline; cloud-init legt Swap an), cpx22 = 2 vCPU / 4 GB"
  type        = string
  default     = "cpx12"
}

variable "location" {
  description = "Hetzner-Location, z. B. fsn1, nbg1, hel1"
  type        = string
  default     = "nbg1"
}

variable "image" {
  description = "Betriebssystem-Image"
  type        = string
  default     = "ubuntu-24.04"
}

variable "volume_size_gb" {
  description = "Größe des Volumes für die PostgreSQL-Daten"
  type        = number
  default     = 50
}

variable "ssh_allowed_cidrs" {
  description = "Quell-Adressen für SSH. GitHub-Runner haben wechselnde IPs, daher offen; Schutz über Key-Login."
  type        = list(string)
  default     = ["0.0.0.0/0", "::/0"]
}

variable "repo_url" {
  description = "Git-URL des Repos, das der Server nach /opt/soziooekonomische-daten klont"
  type        = string
  default     = "https://github.com/BjoernFuertges/Socioeconomic-Data.git"
}
