variable "tailscale_oauth_clientid" {
  description = "OAuth Client ID for Tailscale Operator"
}

variable "tailscale_oauth_clientsecret" {
  description = "OAuth Client Secret for Tailscale Operator"
  sensitive = true
}

variable "tailscale_auth_key" {
  description = "Tailscale Auth Key"
  sensitive = true
}