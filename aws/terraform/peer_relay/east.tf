locals {
  vpc_east_cidr            = "172.18.0.0/16"
  vpc_east_private_subnets = ["172.18.0.0/24", "172.18.1.0/24", "172.18.2.0/24"]
  vpc_east_public_subnets  = ["172.18.3.0/24", "172.18.4.0/24", "172.18.5.0/24"]
}

module "vpc_east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-peer-east"
  cidr               = local.vpc_east_cidr
  enable_nat_gateway = true

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = local.vpc_east_private_subnets
  public_subnets  = local.vpc_east_public_subnets

}

data "aws_ami" "amazon_linux_east" {
  provider = aws.east
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

# data "aws_region" "east" {}

# data "aws_caller_identity" "east" {}

data "aws_iam_policy_document" "east" {

  provider = aws.east  

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
      resources = [        "*"
      ]
    }

  }

}

resource "aws_iam_instance_profile" "east" {
  provider = aws.east  
  name = "${var.name}-east"
  role = aws_iam_role.east.name

  tags = var.tags
}

resource "aws_iam_role" "east" {
  provider = aws.east  
  name = "${var.name}-east"

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

resource "aws_iam_role_policy" "east" {
  provider = aws.east  
  name   = "${var.name}-east"
  role   = aws_iam_role.east.id
  policy = data.aws_iam_policy_document.east.json

  depends_on = [aws_iam_instance_profile.east]
}



module "amz_tailscale_client_amz_east" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "${var.name}-client-east"
  max_retries = 10
  retry_delay = 10
}

resource "aws_security_group" "east" {
  provider = aws.east  
  name        = "${var.name}-east"
  description = "Used in ${var.name} instance ${module.vpc_east.private_subnets[0]}"
  vpc_id      = module.vpc_east.vpc_id


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

resource "aws_placement_group" "east" {
  provider = aws.east  
  name     = "${var.name}-east-placement-group"
  strategy = "cluster"

  tags = merge(var.tags, {
    Name = "${var.name}-east-placement-group"
  })
}

resource "aws_launch_template" "east" {
  provider = aws.east  
  name_prefix = "${var.name}-east-lt"

  placement {
    group_name = aws_placement_group.east.name

  }

  network_interfaces {
    device_index   = 0
    interface_type = "interface"

    subnet_id              = module.vpc_east.private_subnets[0]
    security_groups        = [aws_security_group.east.id]

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


resource "aws_instance" "east" {
  provider = aws.east
  ami           = data.aws_ami.amazon_linux_east.id
  instance_type = var.instance_type
  placement_group = aws_placement_group.east.name

  launch_template {
    id      = aws_launch_template.east.id
    version = "$Latest"
  }

  iam_instance_profile = aws_iam_instance_profile.east.name

  monitoring = true

  user_data_base64            = module.amz_tailscale_client_amz_east.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-amz" }
  )
}


output "amz_instance_id_east" {
  value = aws_instance.east[*].id
}

