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

module "amz-tailscale-client" {
  source           = "lbrlabs/tailscale/cloudinit"
  version          = "0.0.7"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = var.name
  max_retries      = 10
  retry_delay      = 10
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

resource "aws_instance" "performance" {

  count         = var.performance_instance_count  
  ami           = data.aws_ami.amazon_linux.id
  instance_type = var.instance_type

  subnet_id              = module.vpc.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.main.id]

  iam_instance_profile = aws_iam_instance_profile.main.name

  monitoring = true

  user_data_base64 = module.amz-tailscale-client.rendered
  user_data_replace_on_change = true

  tags = merge(
    var.tags,
    { "Name" = "${var.name}-performance" }
  )
}

output "instance_id" {
  value = aws_instance.performance[*].id

}
