module "lbr-vpc-eu" {
  providers = {
    aws = aws.eu
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-regional-vpc-eu"
  cidr               = local.vpc_cidr_eu
  enable_nat_gateway = true

  azs             = ["eu-central-1a", "eu-central-1b", "eu-central-1c"]
  private_subnets = local.vpc_private_subnets_eu
  public_subnets  = local.vpc_public_subnets_eu

}

data "aws_ami" "eu" {

  provider    = aws.eu
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

resource "aws_security_group" "eu" {

  provider    = aws.eu
  vpc_id      = module.lbr-vpc-eu.vpc_id
  description = "Demo streamer traffic"

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all inbound traffic"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
}

resource "aws_instance" "eu-client" {

  provider               = aws.eu
  ami                    = data.aws_ami.eu.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-eu.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.eu.id]

  ebs_optimized = true
  associate_public_ip_address = false

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "eu-client"
  }
}

output "eu_client_ip" {
    value = aws_instance.eu-client.private_ip
}

