locals {
  any_ipv4 = "0.0.0.0/0"
  any_ipv6 = "::/0"
}

resource "hcloud_ssh_key" "deploy" {
  name       = "${var.name}-deploy"
  public_key = var.ssh_public_key
}

# Only SSH, HTTP (Let's Encrypt challenge + redirect) and HTTPS. PostgreSQL (5432)
# stays closed; the compose file additionally binds the port to 127.0.0.1 only.
resource "hcloud_firewall" "main" {
  name = "${var.name}-fw"

  rule {
    description = "SSH"
    direction   = "in"
    protocol    = "tcp"
    port        = "22"
    source_ips  = var.ssh_allowed_cidrs
  }

  rule {
    description = "HTTP"
    direction   = "in"
    protocol    = "tcp"
    port        = "80"
    source_ips  = [local.any_ipv4, local.any_ipv6]
  }

  rule {
    description = "HTTPS"
    direction   = "in"
    protocol    = "tcp"
    port        = "443"
    source_ips  = [local.any_ipv4, local.any_ipv6]
  }

  rule {
    description = "ICMP"
    direction   = "in"
    protocol    = "icmp"
    source_ips  = [local.any_ipv4, local.any_ipv6]
  }
}

resource "hcloud_server" "main" {
  name         = var.name
  server_type  = var.server_type
  image        = var.image
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.deploy.id]
  firewall_ids = [hcloud_firewall.main.id]

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    ssh_public_key = trimspace(var.ssh_public_key)
    repo_url       = var.repo_url
  })

  lifecycle {
    # cloud-init only runs on first boot; later changes should not replace the server
    ignore_changes = [user_data, image, ssh_keys]
  }
}

# PostgreSQL data lives on its own volume and survives a server rebuild.
resource "hcloud_volume" "pgdata" {
  name     = "${var.name}-pgdata"
  size     = var.volume_size_gb
  location = var.location
  format   = "ext4"

  lifecycle {
    prevent_destroy = true
  }
}

resource "hcloud_volume_attachment" "pgdata" {
  volume_id = hcloud_volume.pgdata.id
  server_id = hcloud_server.main.id
  automount = false
}
