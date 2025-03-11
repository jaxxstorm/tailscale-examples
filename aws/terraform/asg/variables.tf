

variable "name" {
  description = "Name of the subnet router"
  type        = string
}

variable "enable_aws_ssm" {
  description = "Enable AWS SSM for the instance"
  type        = bool
  default     = true
  
}

variable "tags" {
  description = "EC2 tags to apply to the instance"
  type        = map(string)
  default     = {}
}

variable "architecture" {
  description = "Architecture of the instance"
  type        = string
  default     = "x86_64"
}

variable "instance_type" {
  description = "Instance type to use for the subnet routers"
  default     = "t3.medium"
  type        = string
}

variable "ebs_root_volume_size" {
  description = "Size of the root volume in GB"
  default     = 20
  type        = number
}


variable "tailscale_auth_key" {
  description = "Tailscale authentication key"
  type        = string
  sensitive   = true
}

variable "advertise_tags" {
  description = "Tags to advertise for the subnet routers"
  type        = list(string)
  default     = []
}