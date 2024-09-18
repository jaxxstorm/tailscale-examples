module "lbr-vpc-east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-easynat-vpc-east"
  cidr               = local.vpc_cidr_east
  enable_nat_gateway = true

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = local.vpc_private_subnets_east
  public_subnets  = local.vpc_public_subnets_east

}

data "aws_ami" "ubuntu-east" {

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

  name_prefix = "lbr-no-nat-east"
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

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [local.vpc_cidr_east]
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

module "nonat-east" {
  source           = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = "nonat-east"
  advertise_tags   = ["tag:no-nat"]
}

resource "aws_instance" "nonat-east" {

  provider               = aws.east
  ami                    = data.aws_ami.ubuntu-east.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-east.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.east.id]
  iam_instance_profile        = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.nonat-east.rendered
  associate_public_ip_address = true
  user_data_replace_on_change = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-nonat"
  }
}

module "hardnat-east" {
  source           = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = "hardnat-east"
  advertise_tags   = ["tag:hard-nat"]
}

resource "aws_instance" "hardnat-east" {

  provider               = aws.east
  ami                    = data.aws_ami.ubuntu-east.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-east.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.east.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.hardnat-east.rendered
  associate_public_ip_address = true
  user_data_replace_on_change = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-hardnat"
  }
}

## EASY NAT CONFIG

module "lbr-vpc-east-no-nat-gateway" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name            = "lbr-no-nat-vpc-east"
  cidr            = local.vpc_cidr_east
  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = local.vpc_private_subnets_east
  public_subnets  = local.vpc_public_subnets_east

  create_igw         = true
  enable_nat_gateway = false

  # Disable route table creation
  manage_default_route_table = false
}

resource "aws_security_group" "easynat_east" {
  name_prefix = "lbr-eastnat-east-sg"
  description = "Security group for NAT instance"
  vpc_id      = module.lbr-vpc-east-no-nat-gateway.vpc_id
  provider    = aws.east

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [local.vpc_cidr_east]
  }

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "lbr-easynat-sg"
  }
}

module "easynat_tailscale_east" {
  source         = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
  hostname       = "easynat-east"
  advertise_tags = ["tag:easy-nat"]
  additional_parts = [
    {
      filename     = "01-nat-setup.sh"
      content      = <<-EOF
        #!/bin/bash

        export DEBIAN_FRONTEND=noninteractive

        apt update
        apt install -y iptables-persistent miniupnpd

        # Enable IP forwarding
        echo 'net.ipv4.ip_forward=1' | tee -a /etc/sysctl.conf
        sysctl -p

        PRIMARY_INTERFACE=$(ip route | grep default | awk '{print $5}')

        # Configure NAT
        iptables -t nat -A POSTROUTING -o $PRIMARY_INTERFACE -j MASQUERADE

        netfilter-persistent save
      EOF
      content_type = "text/x-shellscript"
    },
    {
      filename     = "02-miniupnpd-setup.sh"
      content      = <<-EOF
        #!/bin/bash

        export DEBIAN_FRONTEND=noninteractive

        PRIMARY_INTERFACE=$(ip route | grep default | awk '{print $5}')

        echo 'enable_upnp=yes' >> /etc/miniupnpd/miniupnpd.conf
        echo "ext_ifname=$PRIMARY_INTERFACE" >> /etc/miniupnpd/miniupnpd.conf
        echo "listening_ip=$PRIMARY_INTERFACE" >> /etc/miniupnpd/miniupnpd.conf

        # Enable and start miniupnpd
        systemctl enable miniupnpd
        systemctl start miniupnpd

        # Configure iptables for miniupnpd
        iptables -t nat -A PREROUTING -i $PRIMARY_INTERFACE -p udp --dport 1900 -j REDIRECT --to-ports 1900
        iptables -t nat -A PREROUTING -i $PRIMARY_INTERFACE -p tcp --dport 5000 -j REDIRECT --to-ports 5000

        iptables -t nat -A POSTROUTING -o $PRIMARY_INTERFACE -j MASQUERADE
        netfilter-persistent save

        # Ensure services start on boot
        systemctl enable netfilter-persistent
      EOF
      content_type = "text/x-shellscript"
    }
  ]
}

resource "aws_instance" "easynat_east" {
  provider      = aws.east
  ami           = data.aws_ami.ubuntu-east.id
  instance_type = "t3.micro"


  network_interface {
    network_interface_id = aws_network_interface.easynat_east.id
    device_index         = 0
  }

  user_data_base64            = module.easynat_tailscale_east.rendered
  user_data_replace_on_change = true

  iam_instance_profile = aws_iam_instance_profile.ssm_instance_profile.name

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-easynat-east"
  }
}

# Elastic IP for NAT Instance
resource "aws_eip" "easynat_easy" {
  provider = aws.east
  instance = aws_instance.easynat_east.id
  domain   = "vpc"

  tags = {
    Name = "lbr-nat-instance-eip"
  }
}

resource "aws_network_interface" "easynat_east" {
  provider = aws.east
  description       = "lbr NAT static private ENI"
  subnet_id         = module.lbr-vpc-east-no-nat-gateway.public_subnets[0]
  security_groups   = [aws_security_group.easynat_east.id]
  source_dest_check = false

  tags = {
    Name = "lbr-nat-instance-eni"
  }

}

