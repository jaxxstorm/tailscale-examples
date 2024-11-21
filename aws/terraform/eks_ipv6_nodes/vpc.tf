data "aws_availability_zones" "available" {}

locals {
  name   = "lbr-${basename(path.cwd)}"
  region = "us-west-2"

  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)

  # Calculate subnet CIDRs
  private_subnet_cidrs = cidrsubnets(local.vpc_cidr, 4, 4, 4)
  public_subnet_cidrs  = cidrsubnets(cidrsubnet(local.vpc_cidr, 1, 1), 4, 4, 4)

  tags = {
    Name = local.name
  }
}

################################################################################
# VPC Module
################################################################################

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name = local.name
  cidr = local.vpc_cidr

  azs                                            = local.azs
  private_subnets                                = local.private_subnet_cidrs
  public_subnets                                 = local.public_subnet_cidrs
  public_subnet_assign_ipv6_address_on_creation  = true
  private_subnet_assign_ipv6_address_on_creation = true
  public_subnet_ipv6_prefixes                    = [0, 1, 2]
  private_subnet_ipv6_prefixes                   = [3, 4, 5]
  enable_nat_gateway                             = true
  enable_ipv6                                    = true

  public_subnet_tags = {
    "kubernetes.io/role/elb" = 1
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = 1
  }

  tags = local.tags
}
