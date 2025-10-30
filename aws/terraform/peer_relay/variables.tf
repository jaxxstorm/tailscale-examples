variable "name" {
  description = "Name of the subnet router"
  type        = string
  default     = "lbr-relay"
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

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "c6i.8xlarge"
}

variable "tailscale_auth_key" {
  description = "Tailscale auth key for the instance"
  type        = string
}

variable "enable_ena_srd" {
  description = "Enable ENA Express (SRD)."
  type        = bool
  default     = false
}
