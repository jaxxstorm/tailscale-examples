resource "aws_autoscaling_group" "main" {
  name                = var.name
  max_size            = 1
  min_size            = 1
  desired_capacity    = 1
  health_check_type   = "EC2"
  vpc_zone_identifier = [module.vpc.public_subnets[0]]

  launch_template {
    id      = aws_launch_template.main.id
    version = aws_launch_template.main.latest_version
  }

  tag {
    key                 = "Name"
    value               = var.name
    propagate_at_launch = true
  }

  dynamic "tag" {
    for_each = var.tags

    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }

  instance_refresh {
    strategy = "Rolling"
  }

  timeouts {
    delete = "15m"
  }
}

resource "aws_security_group" "main" {
  name        = var.name
  description = "Used in ${var.name} instance of subnet-router in subnet ${module.vpc.public_subnets[0]}"
  vpc_id      = module.vpc.vpc_id


  ingress {
    description = "Allow UDP traffic to Tailscale for direct connections"
    from_port   = 41641
    to_port     = 41641
    protocol    = "UDP"
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

resource "aws_network_interface" "main" {
  description     = "${var.name} reusable ENI for routing traffic between sites"
  subnet_id       = module.vpc.public_subnets[0]
  security_groups = [aws_security_group.main.id]

  source_dest_check = false

  tags = merge(var.tags, {
    Name = var.name
  })
}