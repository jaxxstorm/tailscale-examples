module "lbr-vpc-west" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-nlb-west"
  cidr               = local.vpc_cidr_west
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets_west
  public_subnets  = local.vpc_public_subnets_west

}


data "aws_ami" "ubuntu-west" {
  provider    = aws.west
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

module "ubuntu-tailscale-client-west" {
  source         = "/Users/lbriggs/src/github/lbrlabs/terraform-cloudinit-tailscale"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = false
  hostname       = "ubuntu-nlb-west"
  advertise_tags = ["tag:west"]
  track          = "unstable"
  additional_parts = [
    {
      filename     = "tailscale_overrides.sh"
      content      = <<-EOF
        #!/bin/bash
        
        # Create the override directory if it doesn't exist
        mkdir -p /etc/systemd/system/tailscaled.service.d

        # Create or overwrite the override.conf file
        cat << EOT > /etc/systemd/system/tailscaled.service.d/override.conf
        [Service]
        Environment="TS_DEBUG_PRETENDPOINT=${data.dns_a_record_set.nlb_west.addrs[0]}:41641"
        EOT

        # Reload systemd to recognize the changes
        systemctl daemon-reload

        # Restart tailscaled to apply the new configuration
        systemctl restart tailscaled

        echo "Tailscale override applied with TS_DEBUG_PRETENDPOINT"
      EOF
      content_type = "text/x-shellscript"
    }
  ]
}

resource "aws_security_group" "west" {
  provider    = aws.west
  vpc_id      = module.lbr-vpc-west.vpc_id
  description = "Tailscale required traffic"

  ingress {
    from_port   = 41641
    to_port     = 41641
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Tailscale UDP port"
  }

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
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

resource "aws_instance" "west" {
  provider               = aws.west
  ami                    = data.aws_ami.ubuntu-west.id
  instance_type          = "t3.micro"
  subnet_id              = module.lbr-vpc-west.private_subnets[0]
  vpc_security_group_ids = [aws_security_group.west.id]
  ebs_optimized          = true
  iam_instance_profile   = aws_iam_instance_profile.ssm_instance_profile.name
  user_data_replace_on_change = true
  user_data_base64       = module.ubuntu-tailscale-client-west.rendered

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "lbr-nlb-west"
  }
}

data "dns_a_record_set" "nlb_west" {
  host = aws_lb.nlb_west.dns_name
}

resource "aws_lb" "nlb_west" {
  provider           = aws.west
  name               = "nlb-west"
  internal           = false
  load_balancer_type = "network"
  subnets            = module.lbr-vpc-west.public_subnets

  enable_deletion_protection = false

  tags = {
    Name = "nlb-west"
  }
}

# Create a target group
resource "aws_lb_target_group" "tg_west" {
  provider    = aws.west
  name        = "tg-west"
  port        = 41641
  protocol    = "UDP"
  target_type = "instance"
  vpc_id      = module.lbr-vpc-west.vpc_id
  health_check {
    enabled             = true
    interval            = 30
    port                = "22"
    protocol            = "TCP"
    timeout             = 10
    healthy_threshold   = 3
    unhealthy_threshold = 3

  }
  target_health_state {
    enable_unhealthy_connection_termination = false
  }
}

# Attach the instance to the target group
resource "aws_lb_target_group_attachment" "tga_west" {
  provider         = aws.west
  target_group_arn = aws_lb_target_group.tg_west.arn
  target_id        = aws_instance.west.id
  port             = 41641
}

# Create a listener for the NLB
resource "aws_lb_listener" "listener_west" {
  provider          = aws.west
  load_balancer_arn = aws_lb.nlb_west.arn
  port              = 41641
  protocol          = "UDP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg_west.arn
  }
}
