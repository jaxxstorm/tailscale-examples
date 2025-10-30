locals {
  vpc_west_cidr            = "172.16.0.0/16"
  vpc_west_private_subnets = ["172.16.0.0/24", "172.16.1.0/24", "172.16.2.0/24"]
  vpc_west_public_subnets  = ["172.16.3.0/24", "172.16.4.0/24", "172.16.5.0/24"]
}

module "vpc_west" {

  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-peer-west"
  cidr               = local.vpc_west_cidr
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_west_private_subnets
  public_subnets  = local.vpc_west_public_subnets

}

data "aws_ami" "amazon_linux_west" {

  provider    = aws.west
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# data "aws_region" "west" {}

# data "aws_caller_identity" "west" {}

data "aws_iam_policy_document" "west" {

  provider = aws.west
  dynamic "statement" {
    for_each = var.enable_aws_ssm ? ["x"] : []

    content {
      sid    = "SessionManager"
      effect = "Allow"
      actions = [
        "ssmmessages:CreateDataChannel",
        "ssmmessages:OpenDataChannel",
        "ssmmessages:CreateControlChannel",
        "ssmmessages:OpenControlChannel",
        "ssm:UpdateInstanceInformation",
      ]
      resources = ["*"
      ]
    }

  }

}

resource "aws_iam_instance_profile" "west" {

  provider = aws.west
  name     = "${var.name}-west"
  role     = aws_iam_role.west.name

  tags = var.tags
}

resource "aws_iam_role" "west" {
  provider = aws.west
  name     = "${var.name}-west"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "ssm.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "west" {
  provider = aws.west
  name     = "${var.name}-west"
  role     = aws_iam_role.west.id
  policy   = data.aws_iam_policy_document.west.json

  depends_on = [aws_iam_instance_profile.west]
}



module "amz_tailscale_client_amz" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "${var.name}-client-west"
  max_retries = 10
  retry_delay = 10
}

resource "aws_security_group" "west" {
  provider    = aws.west
  name        = "${var.name}-west"
  description = "Used in ${var.name} instance ${module.vpc_west.private_subnets[0]}"
  vpc_id      = module.vpc_west.vpc_id


  ingress {
    description = "Allow all VPC traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["172.16.0.0/16"]
  }

  egress {
    description      = "Unrestricted egress"
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = merge(var.tags, {
    Name = var.name
  })
}

resource "aws_placement_group" "west" {
  provider = aws.west
  name     = "${var.name}-west-placement-group"
  strategy = "cluster"

  tags = merge(var.tags, {
    Name = "${var.name}-west-placement-group"
  })
}

resource "aws_launch_template" "west" {
  provider    = aws.west
  name_prefix = "${var.name}-lt"

  placement {
    group_name = aws_placement_group.west.name

  }

  network_interfaces {
    device_index   = 0
    interface_type = "interface"

    subnet_id       = module.vpc_west.private_subnets[0]
    security_groups = [aws_security_group.west.id]

    dynamic "ena_srd_specification" {
      for_each = var.enable_ena_srd ? [1] : []
      content {
        ena_srd_enabled = true

        ena_srd_udp_specification {
          ena_srd_udp_enabled = true
        }
      }
    }
  }
}


resource "aws_instance" "west" {
  provider        = aws.west
  ami             = data.aws_ami.amazon_linux_west.id
  instance_type   = var.instance_type
  placement_group = aws_placement_group.west.name

  launch_template {
    id      = aws_launch_template.west.id
    version = "$Latest"
  }

  iam_instance_profile = aws_iam_instance_profile.west.name

  monitoring = true

  user_data_base64            = module.amz_tailscale_client_amz.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-amz" }
  )
}


output "amz_instance_id_west" {
  value = aws_instance.west[*].id
}

# Relay

resource "aws_security_group" "west_relay" {
  provider    = aws.west
  name        = "${var.name}-west-relay"
  description = "Used in ${var.name} instance ${module.vpc_west.public_subnets[0]}"
  vpc_id      = module.vpc_west.vpc_id


  ingress {
    description = "Allow all VPC traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["172.16.0.0/16"]
  }

  ingress {
    description = "Allow Tailscale traffic"
    from_port   = 8888
    to_port     = 8888
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description      = "Unrestricted egress"
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = merge(var.tags, {
    Name = var.name
  })
}

resource "aws_launch_template" "west_relay" {
  provider    = aws.west
  name_prefix = "${var.name}-lt-relay"

  placement {
    group_name = aws_placement_group.west.name

  }

  network_interfaces {
    device_index   = 0
    interface_type = "interface"

    subnet_id       = module.vpc_west.public_subnets[0]
    security_groups = [aws_security_group.west_relay.id]
    associate_public_ip_address = true

    dynamic "ena_srd_specification" {
      for_each = var.enable_ena_srd ? [1] : []
      content {
        ena_srd_enabled = true

        ena_srd_udp_specification {
          ena_srd_udp_enabled = true
        }
      }
    }
  }
}

module "amz_tailscale_client_relay" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "${var.name}-west"
  advertise_tags = ["tag:relay"]
  max_retries = 10
  retry_delay = 10
  additional_parts = [
    {
      filename     = "relay_set.sh"
      content_type = "text/x-shellscript"
      content = templatefile(
        "${path.module}/templates/relay_set.sh.tmpl",
        {
          RELAY_SERVER_PORT   = "8888"
          MAX_RETRIES = "10"
          RETRY_DELAY = "10"
        }
      )
    }
  ]
}

resource "aws_instance" "west_relay" {
  provider        = aws.west
  ami             = data.aws_ami.amazon_linux_west.id
  instance_type   = var.instance_type
  placement_group = aws_placement_group.west.name

  launch_template {
    id      = aws_launch_template.west_relay.id
    version = "$Latest"
  }

  iam_instance_profile = aws_iam_instance_profile.west.name

  monitoring = true

  user_data_base64            = module.amz_tailscale_client_relay.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-relay" }
  )
}

output "amz_instance_id_relay" {
  value = aws_instance.west_relay[*].id
}

