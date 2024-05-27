module "lbr-vpc-east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-regional-vpc-east"
  cidr               = local.vpc_cidr_east
  enable_nat_gateway = true

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = local.vpc_private_subnets_east
  public_subnets  = local.vpc_public_subnets_east

}

module "ubuntu-tailscale-east" {
  source           = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = "subnet-router-east"
  advertise_tags   = ["tag:subnet-router"]
  advertise_routes = [local.vpc_cidr_eu, local.vpc_cidr_east]
}

data "aws_ami" "east" {

  provider    = aws.east
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

resource "aws_security_group" "east" {

  provider    = aws.east
  vpc_id      = module.lbr-vpc-east.vpc_id
  description = "Tailscale required traffic"

  ingress {
    from_port   = 41641
    to_port     = 41641
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Tailscale UDP port"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
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


resource "aws_instance" "east" {

  provider               = aws.east
  ami                    = data.aws_ami.east.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-east.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.east.id]

  ebs_optimized = true

  user_data_base64            = module.ubuntu-tailscale-east.rendered
  associate_public_ip_address = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-subnet-router-east"
  }
}
