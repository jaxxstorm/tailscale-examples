// variables.tf

variable "role_prefix" {
  description = "Prefix for the IAM role and instance profile names."
  type        = string
  default     = "ts-"
}

variable "name" {
  description = "Base name for resources (e.g., instance, security group)."
  type        = string
  default     = "tailscale-oidc-example"
}

variable "instance_type" {
  description = "EC2 instance type for the performance instance."
  type        = string
  default     = "t3.micro" 
}

variable "tailscale_audience" {
  description = "The audience claim for Tailscale OIDC tokens. Default is 'tailscale'."
  type        = string
  default     = "tailscale"
}

variable "oidc_certificate_arn" {
  description = "ACM certificate ARN for the OIDC domain (required if oidc_domain_name is set)"
  type        = string
  default     = ""
}

variable "oidc_tags" {
  description = "Tags to include in JWT tokens (comma-separated). Example: 'tag:aws,tag:production'"
  type        = string
  default     = "tag:aws"
}