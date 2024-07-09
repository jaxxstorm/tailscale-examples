module "lbr-vpc-east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-nlb-west"
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

module "ubuntu-tailscale-client-east" {
  source         = "/Users/lbriggs/src/github/lbrlabs/terraform-cloudinit-tailscale"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = false
  hostname       = "ubuntu-nlb-east"
  advertise_tags = ["tag:east"]
}

resource "aws_security_group" "east" {
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

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
}

resource "aws_instance" "east" {
  provider             = aws.east
  ami                  = data.aws_ami.ubuntu-east.id
  instance_type        = "t3.micro"
  subnet_id            = module.lbr-vpc-east.private_subnets[0]
  vpc_security_group_ids = [ aws_security_group.east.id]
  ebs_optimized        = true
  iam_instance_profile = aws_iam_instance_profile.ssm_instance_profile.name
  user_data_replace_on_change = true
  user_data_base64     = module.ubuntu-tailscale-client-east.rendered


  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-nlb-east"
  }

}
