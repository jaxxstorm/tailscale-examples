data "aws_ami" "amazon_linux" {
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

data "aws_ami" "ubuntu_2404" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
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

module "amz-tailscale-client-ubuntu" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "ubuntu"
  max_retries = 10
  retry_delay = 10
}

module "amz-tailscale-client-amz" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "amz"
  max_retries = 10
  retry_delay = 10
}

resource "aws_security_group" "main" {
  name        = var.name
  description = "Used in ${var.name} instance ${module.vpc.public_subnets[0]}"
  vpc_id      = module.vpc.vpc_id


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

resource "aws_placement_group" "main" {
  name     = "${var.name}-placement-group"
  strategy = "cluster"

  tags = merge(var.tags, {
    Name = "${var.name}-placement-group"
  })
}

resource "aws_launch_template" "perf" {
  name_prefix = "${var.name}-lt"

  placement {
    group_name = aws_placement_group.main.name
    
  }

  network_interfaces {
    device_index   = 0
    interface_type = "interface"

    subnet_id              = module.vpc.private_subnets[0]
    security_groups        = [aws_security_group.main.id]

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

resource "aws_instance" "ubuntu" {

  count         = var.performance_instance_count
  ami           = data.aws_ami.ubuntu_2404.id
  instance_type = var.instance_type
  placement_group = aws_placement_group.main.name

  launch_template {
    id      = aws_launch_template.perf.id
    version = "$Latest"
  }

  

  iam_instance_profile = aws_iam_instance_profile.main.name

  monitoring = true

  user_data_base64            = module.amz-tailscale-client-ubuntu.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-ubuntu-performance" }
  )
}

resource "aws_instance" "amz" {

  count         = var.performance_instance_count
  ami           = data.aws_ami.amazon_linux.id
  instance_type = var.instance_type
  placement_group = aws_placement_group.main.name

  launch_template {
    id      = aws_launch_template.perf.id
    version = "$Latest"
  }

  iam_instance_profile = aws_iam_instance_profile.main.name

  monitoring = true

  user_data_base64            = module.amz-tailscale-client-amz.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-amz-performance" }
  )
}

output "ubuntu_instance_id" {
  value = aws_instance.ubuntu[*].id

}

output "amz_instance_id" {
  value = aws_instance.amz[*].id
}
