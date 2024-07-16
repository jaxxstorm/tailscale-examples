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

  cluster_name    = "lbr-eks"
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
      iam_role_additional_policies = {
        AmazonSSMManagedInstanceCore = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
      }
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

resource "aws_security_group_rule" "tailscale" {
    provider = aws.west
    type     = "ingress"
    from_port = 41641
    to_port = 41641
    protocol = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    security_group_id = module.eks.node_security_group_id
}

resource "aws_security_group_rule" "tailscale_metrics" {
    provider = aws.west
    type     = "ingress"
    from_port = 9100
    to_port = 9100
    protocol = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    security_group_id = module.eks.node_security_group_id
}















