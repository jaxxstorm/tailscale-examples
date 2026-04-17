data "aws_ami" "ubuntu" {

  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"] # Canonical
}

module "ubuntu-tailscale-client" {
  source = "git@github.com:tailscale/terraform-cloudinit-tailscale.git?ref=main"
  #source         = "/Users/lbriggs/src/github/tailscale/terraform-cloudinit-tailscale"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = "subnet-router"
  advertise_routes = [local.vpc_cidr]
  advertise_tags   = ["tag:subnet-router"]
}

data "cloudinit_config" "private_nginx" {
  gzip          = false
  base64_encode = true

  part {
    content_type = "text/cloud-config"
    content      = <<-EOF
      #cloud-config
      apt:
        conf: |
          Acquire::ForceIPv4 "true";
          Acquire::Retries "5";

      package_update: true
      packages:
        - nginx

      write_files:
        - path: /var/www/html/index.html
          owner: root:root
          permissions: "0644"
          content: |
            <!DOCTYPE html>
            <html lang="en">
            <head>
              <meta charset="utf-8">
              <title>subnet-router-nginx</title>
            </head>
            <body>
              <h1>Private nginx instance</h1>
              <p>This host is running in a private subnet.</p>
            </body>
            </html>

      runcmd:
        - systemctl enable --now nginx
    EOF
  }
}

resource "aws_security_group" "sg" {

  name_prefix = "lbr-subnet-nginx-"
  vpc_id      = module.vpc.vpc_id
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
    cidr_blocks = [local.vpc_cidr]
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

resource "aws_security_group" "private_nginx" {

  name_prefix = "lbr-private-nginx-"
  vpc_id      = module.vpc.vpc_id
  description = "Access to the private nginx instance"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = [local.vpc_cidr]
    description = "Allow HTTP from the VPC"
  }

  ingress {
    from_port   = -1
    to_port     = -1
    protocol    = "icmp"
    cidr_blocks = [local.vpc_cidr]
    description = "Allow ICMP from the VPC"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
}

resource "aws_instance" "subnet-router" {

  ami                    = data.aws_ami.ubuntu.id
  instance_type          = "t3.micro"
  subnet_id              = module.vpc.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.sg.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized     = true
  source_dest_check = false

  user_data_base64            = module.ubuntu-tailscale-client.rendered
  associate_public_ip_address = true
  user_data_replace_on_change = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-subnet-router-nginx"
  }
}

resource "aws_instance" "private-nginx" {

  ami                    = data.aws_ami.ubuntu.id
  instance_type          = "t3.micro"
  subnet_id              = module.vpc.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.private_nginx.id]
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name

  ebs_optimized               = true
  associate_public_ip_address = false
  user_data_replace_on_change = true
  user_data_base64            = data.cloudinit_config.private_nginx.rendered

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-private-nginx"
  }
}
