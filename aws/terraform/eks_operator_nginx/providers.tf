terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.30"
    }
  }
}

provider "aws" {
  region = local.region
}

locals {
  cluster_endpoint = module.eks.cluster_endpoint
  cluster_ca_cert  = base64decode(module.eks.cluster_certificate_authority_data)
  exec_api_version = "client.authentication.k8s.io/v1beta1"
  exec_command     = "aws"
  exec_args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
}

provider "helm" {
  kubernetes {
    host                   = local.cluster_endpoint
    cluster_ca_certificate = local.cluster_ca_cert

    exec {
      api_version = local.exec_api_version
      command     = local.exec_command
      args        = local.exec_args
    }
  }
}

provider "kubernetes" {
  host                   = local.cluster_endpoint
  cluster_ca_certificate = local.cluster_ca_cert

  exec {
    api_version = local.exec_api_version
    command     = local.exec_command
    args        = local.exec_args
  }
}
