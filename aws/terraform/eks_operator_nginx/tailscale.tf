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