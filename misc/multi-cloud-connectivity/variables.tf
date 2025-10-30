variable "tailscale_auth_key" {
  description = "Tailscale auth key for the instance"
  type        = string
}

variable "tags" {
  description = "EC2 tags to apply to the instance"
  type        = map(string)
  default     = {}
}

variable "enable_aws_ssm" {
  description = "Enable AWS SSM for the instance"
  type        = bool
  default     = true
}

variable "ssh_public_key_path" {
  description = "SSH public key for VM access"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}