data "aws_ami" "main" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "owner-alias"
    values = ["amazon"]
  }

  filter {
    name   = "architecture"
    values = [var.architecture]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*"]
  }
}

module "amz-tailscale-client" {
  source           = "git::ssh://git@github.com/tailscale/terraform-cloudinit-tailscale.git?ref=main"
  enable_ssh       = true
  hostname         = var.hostname
  client_id        = var.tailscale_client_id
  id_token         = "$(aws sts get-web-identity-token --audience \"${var.tailscale_audience}\" --signing-algorithm ES384 --duration-seconds 300 --query WebIdentityToken --output text)"
  advertise_tags   = var.advertise_tags
  advertise_routes = [local.vpc_cidr]
  accept_routes    = false
  max_retries      = 10
  retry_delay      = 10
}

data "aws_key_pair" "main" {
  key_name = var.key_pair_name
}

resource "aws_security_group" "main" {
  name_prefix = "ec2-sg-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow SSH access from anywhere"
  }

  ingress {
    protocol = "icmp"
    from_port = -1
    to_port = -1
    cidr_blocks = [local.vpc_cidr]
    description = "Allow ICMP from within VPC"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }

}

resource "aws_launch_template" "main" {
  name                   = var.hostname
  image_id               = data.aws_ami.main.id
  instance_type          = var.instance_type
  key_name               = data.aws_key_pair.main.key_name
  vpc_security_group_ids = [aws_security_group.main.id]

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size = var.ebs_root_volume_size
      volume_type = "gp3"
    }
  }

  user_data = module.amz-tailscale-client.rendered

  iam_instance_profile {
    name = aws_iam_instance_profile.main.name
  }
  # Enforce IMDSv2
  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

}

output "user_data" {
  value = module.amz-tailscale-client.rendered
}

resource "aws_autoscaling_group" "main" {
  name                = var.hostname
  max_size            = 1
  min_size            = 1
  desired_capacity    = 1
  health_check_type   = "EC2"
  vpc_zone_identifier = module.vpc.private_subnets


  launch_template {
    id      = aws_launch_template.main.id
    version = aws_launch_template.main.latest_version
  }


  instance_refresh {
    strategy = "Rolling"
  }

  timeouts {
    delete = "15m"
  }
}