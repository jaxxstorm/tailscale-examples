module "lbr-vpc-east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.8.1"

  name               = "lbr-envoy-east"
  cidr               = local.vpc_cidr_east
  enable_nat_gateway = true

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = local.vpc_private_subnets_east
  public_subnets  = local.vpc_public_subnets_east

}

module "eks-east" {
  providers = {
    aws = aws.east
  }
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "lbr-envoy-east"

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

  vpc_id                   = module.lbr-vpc-east.vpc_id
  subnet_ids               = module.lbr-vpc-east.private_subnets
  control_plane_subnet_ids = module.lbr-vpc-east.public_subnets

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

