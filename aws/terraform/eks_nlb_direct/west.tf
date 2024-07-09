module "lbr-vpc-west" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-eks-west"
  cidr               = local.vpc_cidr_west
  enable_nat_gateway = true

  azs             = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets = local.vpc_private_subnets_west
  public_subnets  = local.vpc_public_subnets_west

}

module "eks" {
  providers = {
    aws = aws.west
  }
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "lbriggs"
  cluster_version = "1.29"

  cluster_endpoint_public_access = true

  cluster_addons = {
    coredns = {
      most_recent = true
    }
    kube-proxy = {
      most_recent = true
    }
    vpc-cni = {
      most_recent = true
    }
  }

  vpc_id                   = module.lbr-vpc-west.vpc_id
  subnet_ids               = module.lbr-vpc-west.private_subnets
  control_plane_subnet_ids = module.lbr-vpc-west.public_subnets

  # EKS Managed Node Group(s)
  eks_managed_node_group_defaults = {
    instance_types = ["t3.medium"]
  }

  eks_managed_node_groups = {
    example = {
      min_size     = 1
      max_size     = 3
      desired_size = 1

      instance_types = ["t3.large"]
      capacity_type  = "SPOT"
    }
  }

  # Cluster access entry
  # To add the current caller identity as an administrator
  enable_cluster_creator_admin_permissions = true

  tags = {
    Environment = "dev"
    Terraform   = "true"
    Owner       = "lbriggs"
  }
}

resource "aws_security_group_rule" "metrics" {
    provider = aws.west
    type     = "ingress"
    from_port = 9001
    to_port = 9001
    protocol = "tcp"
    cidr_blocks = [local.vpc_cidr_west]
    security_group_id = module.eks.node_security_group_id
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

data "dns_a_record_set" "nlb_west" {
  host = aws_lb.nlb_west.dns_name
}

resource "aws_lb_target_group" "tg_west" {
  provider    = aws.west
  name        = "tg-west"
  port        = 41641
  protocol    = "UDP"
  target_type = "ip"
  vpc_id      = module.lbr-vpc-west.vpc_id
  health_check {
    enabled             = true
    interval            = 10
    port                = "9001" # port for metrics endpoint
    protocol            = "TCP"
    timeout             = 10
    healthy_threshold   = 3
    unhealthy_threshold = 3

  }
  target_health_state {
    enable_unhealthy_connection_termination = false
  }
}

resource "aws_lb_target_group_attachment" "tga_west" {
  provider         = aws.west
  target_group_arn = aws_lb_target_group.tg_west.arn
  target_id        = "172.17.2.150"
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

resource "aws_lb_target_group" "tg_west_metrics" {
  provider    = aws.west
  name        = "tg-west-metrics"
  port        = 9001
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = module.lbr-vpc-west.vpc_id
  health_check {
    enabled             = true
    interval            = 10
    port                = "9001" # port for metrics endpoint
    protocol            = "TCP"
    timeout             = 10
    healthy_threshold   = 3
    unhealthy_threshold = 3

  }
  target_health_state {
    enable_unhealthy_connection_termination = false
  }
}

resource "aws_lb_target_group_attachment" "tga_west_metrics" {
  provider         = aws.west
  target_group_arn = aws_lb_target_group.tg_west.arn
  target_id        = "172.17.2.150"
  port             = 9001
}

# Create a listener for the NLB
resource "aws_lb_listener" "listener_west_metrics" {
  provider          = aws.west
  load_balancer_arn = aws_lb.nlb_west.arn
  port              = 9001
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg_west_metrics.arn
  }
}











