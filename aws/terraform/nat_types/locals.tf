locals {
  vpc_cidr_east = "10.1.0.0/16"
  vpc_cidr_west = "10.0.0.0/16"

  vpc_cidr_no_nat_gateway_west = "10.2.0.0/16"

  vpc_subnets_east = cidrsubnets(local.vpc_cidr_east, 3, 3, 3, 3, 3, 3)
  vpc_subnets_west = cidrsubnets(local.vpc_cidr_west, 3, 3, 3, 3, 3, 3)
  vpc_subnets_no_nat_gateway_west = cidrsubnets(local.vpc_cidr_no_nat_gateway_west, 3, 3, 3, 3, 3, 3)

  vpc_private_subnets_east = slice(local.vpc_subnets_east, 0, 3)
  vpc_public_subnets_east  = slice(local.vpc_subnets_east, 3, 6)

  vpc_private_subnets_west = slice(local.vpc_subnets_west, 0, 3)
  vpc_public_subnets_west  = slice(local.vpc_subnets_west, 3, 6)

  vpc_no_nat_gateway_private_subnets_west = slice(local.vpc_subnets_no_nat_gateway_west, 0, 3)
  vpc_no_nat_gateway_public_subnets_west  = slice(local.vpc_subnets_west, 3, 6)

}