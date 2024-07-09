
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

resource "kubernetes_namespace" "tailscale" {
  metadata {
    name = "tailscale"
  }
}

resource "helm_release" "tailscale_operator" {
  name = "tailscale-operator"

  repository = "https://pkgs.tailscale.com/helmcharts"
  chart      = "tailscale-operator"
  namespace  = kubernetes_namespace.tailscale.metadata[0].name

  set {
    name  = "oauth.clientId"
    value = var.tailscale_oauth_clientid
  }

  set {
    name  = "oauth.clientSecret"
    value = var.tailscale_oauth_clientsecret
  }

  set {
    name  = "operatorConfig.hostname"
    value = format("tailscale-operator-%s", module.eks.cluster_name)
  }

}

resource "kubernetes_secret" "tailscale" {
  metadata {
    name      = "subnet-router"
    labels = {
      app = "subnet-router"
    }
  }

}

resource "kubernetes_service_account" "tailscale" {
  metadata {
    name      = "subnet-router"
  }
}

resource "kubernetes_role" "tailscale" {
  metadata {
    name      = "subnet-router"
  }

  rule {
    api_groups = [""]
    resources  = ["secrets"]
    verbs      = ["*"]
  }
}

resource "kubernetes_role_binding" "tailscale" {
  metadata {
    name      = "subnet-router"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.tailscale.metadata[0].name
  }

  subject {
    kind = "ServiceAccount"
    name = kubernetes_service_account.tailscale.metadata[0].name
  }
}


resource "kubernetes_pod" "subnet_router" {
  metadata {
    name = "subnet-router"
    labels = {
      app = "tailscale"
    }
  }

  spec {

    service_account_name = kubernetes_service_account.tailscale.metadata[0].name

    host_network = true
    dns_policy   = "ClusterFirstWithHostNet"

    container {
      name              = "tailscale"
      image             = "tailscale/tailscale:unstable"
      image_pull_policy = "Always"

      port {
        container_port = 41641
      }



      env {
        name  = "TS_USERSPACE"
        value = "false"
      }

      env {
        name  = "TS_DEBUG_FIREWALL_MODE"
        value = "auto"
      }

      env {
        name  = "TS_USERSPACE"
        value = "true"
      }

      env {
        name= "TS_TAILSCALED_EXTRA_ARGS"
        value= "--debug=0.0.0.0:9001"
      }

      env {
        name  = "TS_AUTH_KEY"
        value = var.tailscale_auth_key
      }

      env {
        name  = "TS_ROUTES"
        value = local.vpc_cidr_west
      }

      env {
        name  = "PORT"
        value = "41641"
      }

      env {
        name  = "TS_STATE_DIR"
        value = "mem:"
      }

      env {
        name  = "TS_DEBUG_PRETENDPOINT"
        value = "${data.dns_a_record_set.nlb_west.addrs[0]}:41641"
      }

      env {
        name  = "TS_HOSTNAME"
        value = "subnet-router"
      }

      security_context {
        capabilities {
          add = ["NET_ADMIN"]
        }
      }
    }
  }
}

data "kubernetes_pod_v1" "subnet_router" {
  metadata {
    name = kubernetes_pod.subnet_router.metadata[0].name
  }
}

output "subnet_router_ip" {
  value = data.kubernetes_pod_v1.subnet_router
}




