locals {
  vpc_cidr_east = "172.16.0.0/16"
  vpc_cidr_west = "10.0.0.0/16"
  vpc_cidr_eu   = "172.20.0.0/16"

  vpc_subnets_east = cidrsubnets(local.vpc_cidr_east, 3, 3, 3, 3, 3, 3)
  vpc_subnets_west = cidrsubnets(local.vpc_cidr_west, 3, 3, 3, 3, 3, 3)
  vpc_subnets_eu   = cidrsubnets(local.vpc_cidr_eu, 3, 3, 3, 3, 3, 3)

  vpc_private_subnets_east = slice(local.vpc_subnets_east, 0, 3)
  vpc_public_subnets_east  = slice(local.vpc_subnets_east, 3, 6)

  vpc_private_subnets_west = slice(local.vpc_subnets_west, 0, 3)
  vpc_public_subnets_west  = slice(local.vpc_subnets_west, 3, 6)

  vpc_private_subnets_eu = slice(local.vpc_subnets_eu, 0, 3)
  vpc_public_subnets_eu  = slice(local.vpc_subnets_eu, 3, 6)
}