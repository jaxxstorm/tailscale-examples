
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



resource "kubernetes_secret" "tailscale" {
  metadata {
    name = "subnet-router"
    labels = {
      app = "subnet-router"
    }
  }
  data = {
    tsstate = ""
  }

}

resource "kubernetes_service_account" "tailscale" {
  metadata {
    name = "subnet-router"
  }
}

resource "kubernetes_role" "tailscale" {
  metadata {
    name = "subnet-router"
  }

  rule {
    api_groups = [""]
    resources  = ["secrets"]
    verbs      = ["*"]
  }
}

resource "kubernetes_role_binding" "tailscale" {
  metadata {
    name = "subnet-router"
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

resource "aws_eip" "nlb_eip" {
  
  count = 3

  tags = {
    Name = "Tailscale NLB EIP"
  }
}

locals {
  pretendpoints = join(",", [for eip in aws_eip.nlb_eip : "${eip.public_ip}:41641"])
}

resource "kubernetes_stateful_set" "subnet_router" {
  metadata {
    name = "subnet-router"
    labels = {
      app = "tailscale"
    }
  }

  spec {
    service_name = "subnet-router"
    replicas     = 1

    selector {
      match_labels = {
        app = "tailscale"
      }
    }

    template {
      metadata {
        labels = {
          app = "tailscale"
        }
      }

      spec {
        service_account_name = kubernetes_service_account.tailscale.metadata[0].name

        container {
          name              = "tailscale"
          image             = "tailscale/tailscale:unstable"
          image_pull_policy = "Always"

          port {
            container_port = 41641
            name = "tailscaled"
            protocol = "UDP"
          }

          port {
            container_port = 9100
            name = "metrics"
          }

          env {
            name  = "TS_DEBUG_FIREWALL_MODE"
            value = "auto"
          }

          env {
            name  = "TS_USERSPACE"
            value = "false"
          }

          env {
            name  = "TS_TAILSCALED_EXTRA_ARGS"
            value = "--debug=0.0.0.0:9100"
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
            name  = "TS_EXTRA_ARGS"
            value = "--advertise-tags=tag:subnet-router"
          }

          env {
            name  = "PORT"
            value = "41641"
          }

          env {
            name = "TS_KUBE_SECRET"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.tailscale.metadata[0].name
                key  = "tsstate"
              }
            }
          }

          env {
            name  = "TS_HOSTNAME"
            value = "subnet-router"
          }

          # env {
          #   name  = "TS_DEBUG_PRETENDPOINT"
          #   value = local.pretendpoints
          # }

          security_context {
            capabilities {
              add = ["NET_ADMIN"]
            }
          }



        }
      }
    }
  }
}

resource "kubernetes_service" "tailscale_nlb" {
  metadata {
    name = "tailscale-nlb"
    annotations = {
      "service.beta.kubernetes.io/aws-load-balancer-type" = "nlb"
      "service.beta.kubernetes.io/aws-load-balancer-nlb-target-type" = "instance"
      "service.beta.kubernetes.io/aws-load-balancer-scheme" = "internet-facing"
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol" = "tcp"
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval" = "10"
      "service.beta.kubernetes.io/aws-load-balancer-eip-allocations" = join(",", aws_eip.nlb_eip[*].id)
    }
  }

  spec {
    selector = {
      app = "tailscale"
    }

    port {
      name        = "tailscaled"
      port        = 41641
      target_port = 41641
      protocol    = "UDP"
    }


    type = "LoadBalancer"
  }
}




