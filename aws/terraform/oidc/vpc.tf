locals {
  vpc_cidr            = "172.16.0.0/16"
  vpc_private_subnets = ["172.16.0.0/24", "172.16.1.0/24", "172.16.2.0/24"]
  vpc_public_subnets  = ["172.16.3.0/24", "172.16.4.0/24", "172.16.5.0/24"]
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-oidc-vpc"
  cidr               = local.vpc_cidr
  enable_nat_gateway = true
  single_nat_gateway = true  # Use a single NAT gateway to save costs

  azs              = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets  = local.vpc_private_subnets
  public_subnets   = local.vpc_public_subnets

  tags = {
    Project = "lbr-oidc"
  }
}

output "vpc_id" {
  description = "The ID of the VPC"
  value       = module.vpc.vpc_id
}

output "private_subnets" {
  description = "List of IDs of private subnets"
  value       = module.vpc.private_subnets
}

output "public_subnets" {
  description = "List of IDs of public subnets"
  value       = module.vpc.public_subnets
}