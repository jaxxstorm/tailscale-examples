locals {
  vpc_cidr_east            = "10.0.0.0/16"
  vpc_private_subnets_east = ["10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24"]
  vpc_public_subnets_east  = ["10.0.3.0/24", "10.0.4.0/24", "10.0.5.0/24"]
  vpc_cidr_west            = "10.1.0.0/16"
  vpc_private_subnets_west = ["10.1.0.0/24", "10.1.1.0/24", "10.1.2.0/24"]
  vpc_public_subnets_west  = ["10.1.3.0/24", "10.1.4.0/24", "10.1.5.0/24"]
}