"""A Python Pulumi program"""

import pulumi
import pulumi_kubernetes as k8s
import pulumi_digitalocean as do

TAILSCALE_CONFIG = pulumi.Config("tailscale")
TAILSCALE_OAUTH_CLIENT_ID = TAILSCALE_CONFIG.require("oauth_client_id")
TAILSCALE_OAUTH_CLIENT_SECRET = TAILSCALE_CONFIG.require_secret("oauth_client_secret")

PROJECT = pulumi.get_project()

doks_version = do.get_kubernetes_versions().latest_version

cluster = do.KubernetesCluster(
    "lbrlabs",
    region="sfo3",
    version=doks_version,
    node_pool=do.KubernetesClusterNodePoolArgs(
        name="default",
        auto_scale=False,
        node_count=3,
        size="s-1vcpu-2gb",
    ),
)

pulumi.export("kubeconfig", cluster.kube_configs[0].raw_config)

provider = k8s.Provider("do", kubeconfig=cluster.kube_configs[0].raw_config)

tailscale_ns = k8s.core.v1.Namespace(
    "tailscale",
    metadata=k8s.meta.v1.ObjectMetaArgs(name="tailscale"),
    opts=pulumi.ResourceOptions(provider=provider, parent=provider),
)

tailscale_operator = k8s.helm.v3.Release(
    "tailscale",
    repository_opts=k8s.helm.v3.RepositoryOptsArgs(
        repo="https://pkgs.tailscale.com/helmcharts",
    ),
    namespace=tailscale_ns.metadata.name,
    chart="tailscale-operator",
    values={
        "oauth": {
            "clientId": TAILSCALE_OAUTH_CLIENT_ID,
            "clientSecret": TAILSCALE_OAUTH_CLIENT_SECRET,
            
        },
        "operatorConfig": {
            "hostname": f"{PROJECT}-tailscale-operator",
        }
    },
    opts=pulumi.ResourceOptions(provider=provider, parent=tailscale_ns),
)

external_ingress_ns = k8s.core.v1.Namespace(
    "nginx",
    metadata=k8s.meta.v1.ObjectMetaArgs(
        name="nginx",
    ),
    opts=pulumi.ResourceOptions(provider=provider),
)

external_ingress = k8s.helm.v3.Release(
    "nginx",
    args=k8s.helm.v3.ReleaseArgs(
        chart="ingress-nginx",
        namespace=external_ingress_ns.metadata.name,
        repository_opts=k8s.helm.v3.RepositoryOptsArgs(
            repo="https://kubernetes.github.io/ingress-nginx"
        ),
        values={
            "controller": {
                "ingressClass": "external",
                "service": {
                    "loadBalancerClass": "tailscale",
                    "targetPorts": {
                        "https": 80,
                    },
                },
            }
        },
    ),
    opts=pulumi.ResourceOptions(provider=provider, parent=external_ingress_ns, depends_on=[tailscale_operator]),
)