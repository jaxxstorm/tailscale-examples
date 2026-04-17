locals {
  vpc_cidr            = "172.16.0.0/16"
  vpc_private_subnets = ["172.16.0.0/24", "172.16.1.0/24", "172.16.2.0/24"]
  vpc_public_subnets  = ["172.16.3.0/24", "172.16.4.0/24", "172.16.5.0/24"]
}

module "vpc" {

  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-vpc-west"
  cidr               = local.vpc_cidr
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets
  public_subnets  = local.vpc_public_subnets

}