variable "tailscale_auth_key" {
  description = "Tailscale auth key for the subnet router"
  type        = string
  sensitive   = true
}