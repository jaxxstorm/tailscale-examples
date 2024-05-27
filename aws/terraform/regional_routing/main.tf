data "aws_caller_identity" "current" {}


locals {
  east_route_tables = concat(
    module.lbr-vpc-east.private_route_table_ids,
    module.lbr-vpc-east.public_route_table_ids
  )

  west_route_tables = concat(
    module.lbr-vpc-west.private_route_table_ids,
    module.lbr-vpc-west.public_route_table_ids
  )

  eu_route_tables = concat(
    module.lbr-vpc-eu.private_route_table_ids,
    module.lbr-vpc-eu.public_route_table_ids
  )
}

resource "aws_vpc_peering_connection" "west" {
  provider      = aws.west
  vpc_id        = module.lbr-vpc-west.vpc_id
  peer_vpc_id   = module.lbr-vpc-eu.vpc_id
  peer_region   = "eu-central-1"
  peer_owner_id = data.aws_caller_identity.current.account_id
  tags = {
    "Name" = "lbr-vpc-peering-west-to-eu"
  }
}

resource "aws_vpc_peering_connection_accepter" "west" {
  provider                  = aws.eu
  vpc_peering_connection_id = aws_vpc_peering_connection.west.id
  auto_accept               = true
  tags = {
    "Name" = "lbr-vpc-peering-west-to-eu"
  }
}

resource "aws_route" "west-to-eu" {
  count                     = length(local.west_route_tables)
  provider                  = aws.west
  route_table_id            = local.west_route_tables[count.index]
  destination_cidr_block    = module.lbr-vpc-eu.vpc_cidr_block
  vpc_peering_connection_id = aws_vpc_peering_connection.west.id
}

resource "aws_route" "eu-to-west" {
  count                     = length(local.eu_route_tables)
  provider                  = aws.eu
  route_table_id            = local.eu_route_tables[count.index]
  destination_cidr_block    = module.lbr-vpc-west.vpc_cidr_block
  vpc_peering_connection_id = aws_vpc_peering_connection.west.id
}

resource "aws_vpc_peering_connection" "east" {
  provider      = aws.east
  vpc_id        = module.lbr-vpc-east.vpc_id
  peer_vpc_id   = module.lbr-vpc-eu.vpc_id
  peer_region   = "eu-central-1"
  peer_owner_id = data.aws_caller_identity.current.account_id
  tags = {
    "Name" = "lbr-vpc-peering-east-to-eu"
  }
}

resource "aws_vpc_peering_connection_accepter" "east" {
  provider                  = aws.eu
  vpc_peering_connection_id = aws_vpc_peering_connection.east.id
  auto_accept               = true
  tags = {
    "Name" = "lbr-vpc-peering-east-to-eu"
  }
}

resource "aws_route" "east-to-eu" {
  count                     = length(local.east_route_tables)
  provider                  = aws.east
  route_table_id            = local.east_route_tables[count.index]
  destination_cidr_block    = module.lbr-vpc-eu.vpc_cidr_block
  vpc_peering_connection_id = aws_vpc_peering_connection.east.id
}

resource "aws_route" "eu-to-easy" {
  count                     = length(local.eu_route_tables)
  provider                  = aws.eu
  route_table_id            = local.eu_route_tables[count.index]
  destination_cidr_block    = module.lbr-vpc-east.vpc_cidr_block
  vpc_peering_connection_id = aws_vpc_peering_connection.east.id
}
