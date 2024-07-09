locals {
  vpc_cidr_east            = "172.16.0.0/16"
  vpc_private_subnets_east = ["172.16.0.0/24", "172.16.1.0/24", "172.16.2.0/24"]
  vpc_public_subnets_east  = ["172.16.3.0/24", "172.16.4.0/24", "172.16.5.0/24"]
  vpc_cidr_west            = "172.17.0.0/16"
  vpc_private_subnets_west = ["172.17.0.0/24", "172.17.1.0/24", "172.17.2.0/24"]
  vpc_public_subnets_west  = ["172.17.3.0/24", "172.17.4.0/24", "172.17.5.0/24"]
}