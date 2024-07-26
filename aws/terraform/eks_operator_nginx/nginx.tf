resource "helm_release" "ingress_nginx" {
  name       = "ingress-nginx"
  repository = "https://kubernetes.github.io/ingress-nginx"
  chart      = "ingress-nginx"
  version    = "4.7.1"

  set {
    name  = "controller.service.type"
    value = "LoadBalancer"
  }

  set {
    name  = "controller.service.loadBalancerClass"
    value = "tailscale"
  }

  set {
    name  = "controller.service.annotations.tailscale\\.com/hostname"
    value = "ingress-nginx"
  }

  set {
    name  = "controller.config.use-forwarded-headers"
    value = "true"
  }

  set {
    name  = "controller.config.compute-full-forwarded-for"
    value = "true"
  }

  set {
    name  = "controller.config.use-proxy-protocol"
    value = "false"
  }

  set {
    name  = "controller.ingressClassResource.name"
    value = "tailscale-internal"
  }

  set {
    name  = "controller.ingressClassResource.enabled"
    value = "true"
  }

}