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
        node_count=1,
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

wordpress = k8s.helm.v3.Release(
    "wordpress",
    chart="wordpress",
    repository_opts=k8s.helm.v3.RepositoryOptsArgs(
        repo="https://charts.bitnami.com/bitnami",
    ),
    values={
        "wordpressUsername": "lbrlabs",
        "wordpressPassword": "correct-horse-battery-stable",
        "wordpressEmail": "mail@lbrlabs.com",
        "service": {
          "type": "ClusterIP", # we don't want a digitalocean load balancer  
        },
        "ingress": {
            "enabled": False,
        },
    },
    opts=pulumi.ResourceOptions(provider=provider),
)

svc = k8s.core.v1.Service.get("wordpress", pulumi.Output.concat(wordpress.status.namespace, "/", wordpress.status.name))

# we create our own ingress

ingress = k8s.networking.v1.Ingress(
    "wordpress",
    metadata=k8s.meta.v1.ObjectMetaArgs(
        namespace="default",
        annotations={
            "tailscale.com/funnel": "true"
        }
    ),
    spec=k8s.networking.v1.IngressSpecArgs(
        default_backend=k8s.networking.v1.IngressBackendArgs(
            service=k8s.networking.v1.IngressServiceBackendArgs(
                name=svc.metadata.name,
                port=k8s.networking.v1.ServiceBackendPortArgs(
                    name="https",
                ),
            )
        ),
        ingress_class_name="tailscale",
        tls=[
            k8s.networking.v1.IngressTLSArgs(
                hosts=["wordpress"],
            ),
        ],
    ),
    opts=pulumi.ResourceOptions(provider=provider, parent=svc, depends_on=[tailscale_operator]),
)
