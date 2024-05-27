module "lbr-vpc-west" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-regional-vpc-west"
  cidr               = local.vpc_cidr_west
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets_west
  public_subnets  = local.vpc_public_subnets_west

}

module "ubuntu-tailscale-west" {
  source           = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = "subnet-router-west"
  advertise_tags   = ["tag:subnet-router"]
  advertise_routes = [local.vpc_cidr_eu, local.vpc_cidr_west]
}

data "aws_ami" "west" {

  provider    = aws.west
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"] # Canonical
}

resource "aws_security_group" "west" {

  name_prefix = "lbr-west-subnet-router-west"
  provider    = aws.west
  vpc_id      = module.lbr-vpc-west.vpc_id
  description = "Tailscale required traffic"

  ingress {
    from_port   = 41641
    to_port     = 41641
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Tailscale UDP port"
  }

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [local.vpc_cidr_west]
    description = "Allow all for testing"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
}

resource "aws_instance" "west" {

  provider               = aws.west
  ami                    = data.aws_ami.west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.ubuntu-tailscale-west.rendered
  associate_public_ip_address = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-subnet-router-west"
  }
}

resource "aws_instance" "client" {

  provider               = aws.west
  ami                    = data.aws_ami.west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]

  ebs_optimized     = true
  source_dest_check = false

  associate_public_ip_address = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-client-west"
  }
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
