import lbrlabs_pulumi_eks as eks
import pulumi_kubernetes as k8s
import pulumi_aws as aws
import pulumi_awsx as awsx
import pulumi
import json

TAILSCALE_CONFIG = pulumi.Config("tailscale")
TAILSCALE_OAUTH_CLIENT_ID = TAILSCALE_CONFIG.require("oauth_client_id")
TAILSCALE_OAUTH_CLIENT_SECRET = TAILSCALE_CONFIG.require_secret("oauth_client_secret")

vpc = awsx.ec2.Vpc(
    "lbr-vector",
    cidr_block="192.168.0.0/16",
    subnet_strategy=awsx.ec2.SubnetAllocationStrategy.AUTO,
    subnet_specs=[
        awsx.ec2.SubnetSpecArgs(
            type=awsx.ec2.SubnetType.PUBLIC,
            cidr_mask=20,
            tags={
                "kubernetes.io/role/elb": "1",
            },
        ),
        awsx.ec2.SubnetSpecArgs(
            type=awsx.ec2.SubnetType.PRIVATE,
            cidr_mask=19,
            tags={
                "kubernetes.io/role/internal-elb": "1",
            },
        ),
    ],
)

cluster = eks.Cluster(
    "lbr-vector",
    cluster_subnet_ids=vpc.public_subnet_ids,
    system_node_subnet_ids=vpc.private_subnet_ids,
    system_node_instance_types=["t3.large"],
    system_node_desired_count=1,
    enable_external_ingress=False,
    enable_internal_ingress=False,
    enable_karpenter=True,
)

pulumi.export("kubeconfig", cluster.kubeconfig)

provider = k8s.Provider("provider", kubeconfig=cluster.kubeconfig)


nodes = eks.AttachedNodeGroup(
    "lbr-vector",
    cluster_name=cluster.cluster_name,
    subnet_ids=vpc.public_subnet_ids,
    instance_types=["t3.large"],
    scaling_config=aws.eks.NodeGroupScalingConfigArgs(
        desired_size=1,
        min_size=3,
        max_size=3,
    ),
    opts=pulumi.ResourceOptions(
        parent=cluster,
        providers={"kubernetes": provider},
    ),
)

autoscaled_nodes = eks.AutoscaledNodeGroup(
    "karpenter",
    node_role=cluster.karpenter_node_role.name,
    security_group_ids=[cluster.control_plane.vpc_config.cluster_security_group_id],
    subnet_ids=vpc.private_subnet_ids,
    requirements=[
        eks.RequirementArgs(
            key="kubernetes.io/arch",
            operator="In",
            values=["amd64"],
        ),
        eks.RequirementArgs(
            key="kubernetes.io/os",
            operator="In",
            values=["linux"],
        ),
        eks.RequirementArgs(
            key="karpenter.k8s.aws/instance-category",
            operator="In",
            values=["t"],
        ),
        eks.RequirementArgs(
            key="karpenter.k8s.aws/instance-generation",
            operator="In",
            values=["3"],
        ),
    ],
    opts=pulumi.ResourceOptions(
        provider=provider,
    ),
)

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
            "image": {},
            "hostname": f"eks-operator-vector",
            "tolerations": [
                {
                    "key": "node.lbrlabs.com/system",
                    "operator": "Equal",
                    "value": "true",
                    "effect": "NoSchedule",
                },
            ],
        },
    },
    opts=pulumi.ResourceOptions(provider=provider, parent=tailscale_ns),
)

monitoring_ns = k8s.core.v1.Namespace(
    "monitoring",
    metadata=k8s.meta.v1.ObjectMetaArgs(name="monitoring"),
    opts=pulumi.ResourceOptions(provider=provider, parent=provider),
)

loki_config = {
    "loki": {
        "auth_enabled": False,
        "commonConfig": {
            "replication_factor": 1,
            "schemaConfig": {
                "configs": [
                    {
                        "from": "2024-04-01",
                        "store": "tsdb",
                        "object_store": "s3",
                        "schema": "v13",
                        "index": {"prefix": "loki_index_", "period": "24h"},
                    }
                ]
            },
        },
        "ingester": {"chunk_encoding": "snappy"},
        "tracing": {"enabled": True},
        "querier": {"max_concurrent": 2},
        "deploymentMode": "SingleBinary",
        "singleBinary": {
            "replicas": 1,
            "resources": {
                "limits": {"cpu": 3, "memory": "4Gi"},
                "requests": {"cpu": 2, "memory": "2Gi"},
            },
            "extraEnv": [{"name": "GOMEMLIMIT", "value": "3750MiB"}],
            "chunksCache": {"writebackSizeLimit": "10MB"},
        },
        "minio": {"enabled": True},
        "backend": {"replicas": 0},
        "read": {"replicas": 0},
        "write": {"replicas": 0},
        "ingester": {"replicas": 0},
        "querier": {"replicas": 0},
        "queryFrontend": {"replicas": 0},
        "queryScheduler": {"replicas": 0},
        "distributor": {"replicas": 0},
        "compactor": {"replicas": 0},
        "indexGateway": {"replicas": 0},
        "bloomCompactor": {"replicas": 0},
        "bloomGateway": {"replicas": 0},
    }
}

loki = k8s.helm.v3.Release(
    "loki",
    repository_opts=k8s.helm.v3.RepositoryOptsArgs(
        repo="https://grafana.github.io/helm-charts"
    ),
    chart="loki",
    namespace=monitoring_ns.metadata.name,
    values=loki_config,
    opts=pulumi.ResourceOptions(provider=provider, parent=monitoring_ns),
)

vector_ns = k8s.core.v1.Namespace(
    "vector",
    metadata=k8s.meta.v1.ObjectMetaArgs(name="vector"),
    opts=pulumi.ResourceOptions(provider=provider, parent=provider),
)

loki_svc = k8s.core.v1.Service.get(
    "loki", pulumi.Output.concat(loki.status.namespace, "/", loki.status.name)
)

vector = k8s.helm.v3.Release(
    "vector",
    repository_opts=k8s.helm.v3.RepositoryOptsArgs(
        repo="https://helm.vector.dev",
    ),
    namespace=vector_ns.metadata.name,
    chart="vector",
    values={
        "service": {
            "annotations": {
                "tailscale.com/expose": "true",
            }
        },
        "customConfig": {
            "data_dir": "/vector-data-dir",
        },
        #     "api": {
        #         "enabled": "true",
        #         "address": "0.0.0.0:8686",
        #         "playground": "false",
        #     },
        #     "sources": [
        #         {
        #             "splunk_hec": {
        #                 "address": "0.0.0.0:8080",
        #                 "type": "splunk_hec",
        #             }
        #         }
        #     ],
        #     "sinks": [
        #         {
        #             "loki": {
        #                 "address": "splunk:8088",
        #                 "type": "loki",
        #                 "inputs": ["splunk_hec"],
        #                 "labels": {
        #                     "source": "tailscale",
        #                 },
        #                 "encoding": {
        #                     "codec": "json",
        #                 },
        #                 "endpoint": "http://loki-1178fa99.monitoring:3100",
        #             },
        #         }
        #     ],
        # },
    },
    opts=pulumi.ResourceOptions(provider=provider, parent=tailscale_ns),
)

grafana_config = {
    "enabled": True,
    "ingress": {
        "enabled": True,
        "hosts": [f"grafana-vector"],
        "ingressClassName": "tailscale",
        "annotations": {
            "tailscale.com/tags": "tag:grafana",
        },
        "tls": [
            {
                "hosts": [f"grafana-vector"],
            }
        ],
    },
    "tolerations": [
        {
            "key": "node.lbrlabs.com/system",
            "operator": "Equal",
            "value": "true",
            "effect": "NoSchedule",
        },
    ],
}

kube_prometheus = k8s.helm.v3.Release(
    "kube-prometheus",
    repository_opts=k8s.helm.v3.RepositoryOptsArgs(
        repo="https://prometheus-community.github.io/helm-charts",
    ),
    chart="kube-prometheus-stack",
    namespace=monitoring_ns.metadata.name,
    version="57.0.1",
    values={
        "grafana": grafana_config,
        "prometheus-node-exporter": {
            "affinity": {
                "nodeAffinity": {
                    "requiredDuringSchedulingIgnoredDuringExecution": {
                        "nodeSelectorTerms": [
                            {
                                "matchExpressions": [
                                    {
                                        "key": "eks.amazonaws.com/compute-type",
                                        "operator": "NotIn",
                                        "values": ["fargate"],
                                    }
                                ]
                            }
                        ]
                    }
                }
            }
        },
        "alertmanager": {
            "alertmanagerSpec": {
                "tolerations": [
                    {
                        "key": "node.lbrlabs.com/system",
                        "operator": "Equal",
                        "value": "true",
                        "effect": "NoSchedule",
                    },
                ],
            },
        },
        "admissionWebhooks": {
            "patch": {
                "tolerations": [
                    {
                        "key": "node.lbrlabs.com/system",
                        "operator": "Equal",
                        "value": "true",
                        "effect": "NoSchedule",
                    },
                ],
            }
        },
        "kubeStateMetrics": {
            "tolerations": [
                {
                    "key": "node.lbrlabs.com/system",
                    "operator": "Equal",
                    "value": "true",
                    "effect": "NoSchedule",
                },
            ],
        },
        "nodeExporter": {
            "tolerations": [
                {
                    "key": "node.lbrlabs.com/system",
                    "operator": "Equal",
                    "value": "true",
                    "effect": "NoSchedule",
                }
            ],
        },
        "prometheus": {
            "ingress": {
                "enabled": True,
                "hosts": [f"prometheus"],
                "ingressClassName": "tailscale",
                "tls": [
                    {
                        "hosts": [f"prometheus"],
                    }
                ],
            },
            "prometheusSpec": {
                "externalLabels": {"cluster": "lbr-vector"},
                "serviceMonitorSelector": {},
                "serviceMonitorSelectorNilUsesHelmValues": False,
                "tolerations": [
                    {
                        "key": "node.lbrlabs.com/system",
                        "operator": "Equal",
                        "value": "true",
                        "effect": "NoSchedule",
                    }
                ],
            },
        },
    },
    opts=pulumi.ResourceOptions(parent=monitoring_ns, provider=provider),
)
