variable "tailscale_auth_key" {
  description = "Tailscale auth key for the VM"
  type        = string
  sensitive = true
}

variable "name" {
  description = "Hostname for the VM"
  type        = string
  default     = "lbr-perf"
}

variable "ssh_public_key_path" {
  description = "SSH public key for VM access"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "instance_size" {
  description = "Size of the VM instance"
  type        = string
  default     = "Standard_F4as_v6"
}