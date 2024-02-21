
locals {
  cluster_endpoint    = module.eks.cluster_endpoint
  cluster_ca_cert     = base64decode(module.eks.cluster_certificate_authority_data)
  exec_api_version    = "client.authentication.k8s.io/v1beta1"
  exec_command        = "aws"
  exec_args           = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
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

resource "kubernetes_namespace" "tailscale" {
  metadata {
    name = "tailscale"
  }
}

resource "helm_release" "tailscale_operator" {
  name = "tailscale-operator"

  repository = "https://pkgs.tailscale.com/helmcharts"
  chart      = "tailscale-operator"
  namespace = kubernetes_namespace.tailscale.metadata[0].name

  set {
    name  = "oauth.clientId"
    value = var.tailscale_oauth_clientid
  }

  set {
    name  = "oauth.clientSecret"
    value = var.tailscale_oauth_clientsecret
  }

  set {
    name = "operatorConfig.hostname"
    value = format("tailscale-operator-%s", module.eks.cluster_name)
  }

}

resource "kubernetes_manifest" "test-configmap" {
  manifest = {
    "apiVersion" = "tailscale.com/v1alpha1"
    "kind"       = "Connector"
    "metadata" = {
      "name"      = "vpc-cidr"
    }
    "spec" = {
        "hostname" = format("example-%s-cluster-subnet-router", module.eks.cluster_name)
        "subnetRouter" = {
            "advertiseRoutes" = [
                local.vpc_cidr
            ]
        }
    }
  }
}
