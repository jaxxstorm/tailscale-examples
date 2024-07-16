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

resource "aws_eip" "nlb_eip" {

  count = length(module.lbr-vpc-west.azs)

  tags = {
    Name = "Tailscale EC2 NLB EIP"
  }
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
  source         = "lbrlabs/tailscale/cloudinit"
  version        = "0.0.4"
  auth_key       = var.tailscale_auth_key
  enable_ssh     = true
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
        Environment="TS_DEBUG_PRETENDPOINT=${local.pretendpoints}"
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

locals {
  pretendpoints = join(",", [for eip in aws_eip.nlb_eip : "${eip.public_ip}:41641"])
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

resource "aws_ssm_parameter" "asg_state" {
  name        = "/lbr-nlb-west/asg-state"
  description = "Tailscale state info information for lbr-nlb-west ASG"
  type        = "String"
  value       = "{}"  # Initial empty JSON object

  tags = {
    Project     = "lbr-nlb-west"
  }
  lifecycle {
    ignore_changes = [ value ]
  }
}

resource "aws_iam_policy" "ssm_parameter_access" {
  name        = "SSMParameterAccess"
  path        = "/"
  description = "IAM policy for accessing SSM Parameter"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ssm:GetParameter",
          "ssm:PutParameter"
        ]
        Resource = aws_ssm_parameter.asg_state.arn
      }
    ]
  })
}

# Attach the SSM Parameter access policy to the existing IAM role
resource "aws_iam_role_policy_attachment" "ssm_parameter_policy_attachment" {
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
  role       = aws_iam_role.tailscale.name  
}


module "asg" {
  source = "terraform-aws-modules/autoscaling/aws"
  version = "6.10.0" 

  # Autoscaling group
  name = "lbr-nlb-west-asg"

  min_size                  = 1
  max_size                  = 1
  desired_capacity          = 1
  wait_for_capacity_timeout = 0
  health_check_type         = "ELB"
  vpc_zone_identifier       = module.lbr-vpc-west.private_subnets

  # Launch template
  launch_template_name        = "lbr-nlb-west-lt"
  launch_template_description = "Launch template for Tailscale NLB instance"
  update_default_version      = true

  image_id          = data.aws_ami.ubuntu-west.id
  instance_type     = "t3.micro"
  ebs_optimized     = true
  enable_monitoring = true

  # Security group
  security_groups = [aws_security_group.west.id]

  # IAM instance profile
  create_iam_instance_profile = false
  iam_instance_profile_name   = aws_iam_instance_profile.ssm_instance_profile.name

  # User data
  user_data = module.ubuntu-tailscale-client-west.rendered

  # Target group attachment
  target_group_arns = [aws_lb_target_group.tg_west.arn]

  # Metadata options
  metadata_options = {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  # Tags
  tags = {
    Name = "lbr-nlb-west"
  }
}


resource "aws_lb" "nlb_west" {
  provider           = aws.west
  name               = "nlb-west"
  internal           = false
  load_balancer_type = "network"

  enable_deletion_protection = false

  # Use EIPs for the NLB
  dynamic "subnet_mapping" {
    for_each = module.lbr-vpc-west.public_subnets
    content {
      subnet_id     = subnet_mapping.value
      allocation_id = aws_eip.nlb_eip[subnet_mapping.key].id
    }
  }


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
    interval            = 10
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
