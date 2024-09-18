module "lbr-vpc-west" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-nat-vpc-west"
  cidr               = local.vpc_cidr_west
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets_west
  public_subnets  = local.vpc_public_subnets_west

}

data "aws_ami" "ubuntu-west" {

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

  name_prefix = "lbr-no-nat-west"
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

module "nonat-west" {
  source         = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
  hostname       = "nonat-west"
  advertise_tags = ["tag:no-nat"]
}

resource "aws_instance" "nonat-west" {

  provider               = aws.west
  ami                    = data.aws_ami.ubuntu-west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.nonat-west.rendered
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

locals {
  pretendpoints = join(",", [for eip in aws_eip.nlb_eip : "${eip.public_ip}:41641"])
}

module "hardnat-west" {
  source         = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
  hostname       = "hardnat-west"
  advertise_tags = ["tag:hard-nat"]
}

resource "aws_instance" "hardnat-west" {

  provider               = aws.west
  ami                    = data.aws_ami.ubuntu-west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.hardnat-west.rendered
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


# Hard NAT with LB

module "hardnat-west-lb" {
  source         = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
  hostname       = "hardnat-west-with-lb"
  advertise_tags = ["tag:hard-nat"]
  additional_parts = [
    {
      filename     = "tailscale_overrides.sh"
      content      = <<-EOF
        #!/bin/bash
        
        # Create the override directory if it doesn't exist
        mkdir -p /etc/systemd/system/tailscaled.service.d

        # Create or overwrite the override.conf file
        cat << EOT > /etc/systemd/system/tailscaled.service.d/override.conf
        [Service]
        Environment="TS_DEBUG_PRETENDPOINT=${local.pretendpoints}"
        EOT

        # Reload systemd to recognize the changes
        systemctl daemon-reload

        # Restart tailscaled to apply the new configuration
        systemctl restart tailscaled

        echo "Tailscale override applied with TS_DEBUG_PRETENDPOINT"
      EOF
      content_type = "text/x-shellscript"
    }
  ]
}

resource "aws_instance" "hardnat-west-lb" {

  provider               = aws.west
  ami                    = data.aws_ami.ubuntu-west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.hardnat-west-lb.rendered
  associate_public_ip_address = true
  user_data_replace_on_change = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-hardnat-with-lb"
  }
}

resource "aws_eip" "nlb_eip" {

  count = length(module.lbr-vpc-west.azs)

  tags = {
    Name = "Tailscale EC2 NLB EIP"
  }
}

resource "aws_lb" "nlb_west" {
  provider           = aws.west
  name               = "nlb-west"
  internal           = false
  load_balancer_type = "network"

  enable_deletion_protection = false

  # Use EIPs for the NLB
  dynamic "subnet_mapping" {
    for_each = module.lbr-vpc-west.public_subnets
    content {
      subnet_id     = subnet_mapping.value
      allocation_id = aws_eip.nlb_eip[subnet_mapping.key].id
    }
  }


  tags = {
    Name = "nlb-west"
  }
}

resource "aws_lb_target_group" "tg_west" {
  provider    = aws.west
  name        = "tg-west"
  port        = 41641
  protocol    = "UDP"
  target_type = "instance"
  vpc_id      = module.lbr-vpc-west.vpc_id
  health_check {
    enabled             = true
    interval            = 10
    port                = "22"
    protocol            = "TCP"
    timeout             = 10
    healthy_threshold   = 3
    unhealthy_threshold = 3

  }
  target_health_state {
    enable_unhealthy_connection_termination = false
  }
}

# Create a listener for the NLB
resource "aws_lb_listener" "listener_west" {
  provider          = aws.west
  load_balancer_arn = aws_lb.nlb_west.arn
  port              = 41641
  protocol          = "UDP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg_west.arn
  }
}

resource "aws_lb_target_group_attachment" "hardnat_west" {
  provider         = aws.west
  target_group_arn = aws_lb_target_group.tg_west.arn
  target_id        = aws_instance.hardnat-west-lb.id
  port             = 41641
}


module "lbr-vpc-west-no-nat-gateway" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name            = "lbr-easynat-vpc-west"
  cidr            = local.vpc_cidr_west
  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets_west
  public_subnets  = local.vpc_public_subnets_west

  create_igw         = true
  enable_nat_gateway = false

  # Disable route table creation
  manage_default_route_table = false
}

resource "aws_security_group" "easynat_west" {
  name_prefix = "lbr-easynat-west-sg"
  description = "Security group for easy NAT instance"
  vpc_id      = module.lbr-vpc-west-no-nat-gateway.vpc_id
  provider    = aws.west

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [local.vpc_cidr_west]
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
    Name = "lbr-easynat-west-sg"
  }
}

## EASY NAT CONFIG

module "easynat_tailscale_west" {
  source         = "git@github.com:lbrlabs/terraform-cloudinit-tailscale.git"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
  hostname       = "easynat-west"
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

resource "aws_instance" "easynat_west" {
  provider      = aws.west
  ami           = data.aws_ami.ubuntu-west.id
  instance_type = "t3.micro"

  key_name = "lbriggs"

  network_interface {
    network_interface_id = aws_network_interface.easynat_west.id
    device_index         = 0
  }

  user_data_base64            = module.easynat_tailscale_west.rendered
  user_data_replace_on_change = true

  iam_instance_profile = aws_iam_instance_profile.ssm_instance_profile.name

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-easynat-west"
  }
}

# Elastic IP for NAT Instance
resource "aws_eip" "easynat_west" {
  provider = aws.west
  instance = aws_instance.easynat_west.id
  domain   = "vpc"

  tags = {
    Name = "lbr-easynat-west"
  }
}

resource "aws_network_interface" "easynat_west" {
  provider = aws.west
  description       = "lbr NAT static private ENI"
  subnet_id         = module.lbr-vpc-west-no-nat-gateway.public_subnets[0]
  security_groups   = [aws_security_group.easynat_west.id]
  source_dest_check = false

  tags = {
    Name = "lbr-easynat-eni"
  }

}





