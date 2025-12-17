variable "architecture" {
  description = "Architecture of the instance"
  type        = string
  default     = "x86_64"
}

variable "hostname" {
  description = "Hostname for the Tailscale client"
  type        = string
  default     = "tailscale-oidc-client"
}

variable "advertise_tags" {
  description = "Tags to advertise for the subnet routers"
  type        = list(string)
  default     = []
}

variable "instance_type" {
  description = "The instance type of the EC2 instances"
  type        = string
  default     = "t3.small"
}

variable "ebs_root_volume_size" {
  description = "The size of the EBS root volume in GB"
  type        = number
  default     = 20
}

variable "key_pair_name" {
  description = "The name of the key pair to use for EC2 instances"
  type        = string
}

variable "enable_aws_ssm" {
  description = "Enable AWS SSM permissions for the instance"
  type        = bool
  default     = true
}

variable "tailscale_audience" {
  description = "The Tailscale OIDC audience"
  type        = string
}

variable "tailscale_client_id" {
    description = "The Tailscale OIDC client ID"
    type        = string
  
}

