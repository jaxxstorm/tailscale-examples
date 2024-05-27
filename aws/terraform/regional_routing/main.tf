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
