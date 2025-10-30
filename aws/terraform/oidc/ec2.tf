// ec2.tf
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

}

resource "aws_instance" "oidc" {

  count         = 1
  ami           = data.aws_ami.amazon_linux.id
  instance_type = var.instance_type

  subnet_id              = module.vpc.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.main.id]

  iam_instance_profile = aws_iam_instance_profile.tailscale_ec2_oidc.name

  monitoring = true

  user_data_replace_on_change = true

  metadata_options {
    http_tokens = "required"
    http_put_response_hop_limit = 1
    http_endpoint = "enabled"
  }


}

output "instance_id" {
  value = aws_instance.oidc[*].id

}