output "server_ipv4" {
  description = "A-Record für APP_HOST und Wert für die Repo-Variable SSH_HOST"
  value       = hcloud_server.main.ipv4_address
}

output "server_ipv6" {
  description = "AAAA-Record für APP_HOST (optional)"
  value       = hcloud_server.main.ipv6_address
}
