import pulumi
import lbrlabs_pulumi_eks as lbrlabs_eks
import pulumi_aws as aws
import pulumi_awsx as awsx
import pulumi_kubernetes as k8s

TAILSCALE_CONFIG = pulumi.Config("tailscale")
TAILSCALE_OAUTH_CLIENT_ID = TAILSCALE_CONFIG.require("oauth_client_id")
TAILSCALE_OAUTH_CLIENT_SECRET = TAILSCALE_CONFIG.require_secret("oauth_client_secret")


vpc = awsx.ec2.Vpc(
    "lbriggs",
    cidr_block="172.16.0.0/16",
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
cluster = lbrlabs_eks.Cluster(
    "lbriggs",
    cluster_subnet_ids=vpc.public_subnet_ids,
    system_node_subnet_ids=vpc.private_subnet_ids,
    system_node_instance_types=["t3.large"],
    system_node_desired_count=1,
    enable_external_ingress=False,
    enable_internal_ingress=False,
)

provider = k8s.Provider("provider", kubeconfig=cluster.kubeconfig)
pulumi.export("kubeconfig", cluster.kubeconfig)

workload = lbrlabs_eks.AutoscaledNodeGroup(
    "workload",
    node_role=cluster.karpenter_node_role.name,
    security_group_ids=[cluster.control_plane.vpc_config.cluster_security_group_id],
    subnet_ids=vpc.private_subnet_ids,
    requirements=[
        lbrlabs_eks.RequirementArgs(
            key="kubernetes.io/arch",
            operator="In",
            values=["amd64"],
        ),
        lbrlabs_eks.RequirementArgs(
            key="kubernetes.io/os",
            operator="In",
            values=["linux"],
        ),
        lbrlabs_eks.RequirementArgs(
            key="karpenter.k8s.aws/instance-family",
            operator="In",
            values=["t3"],
        ),
        lbrlabs_eks.RequirementArgs(
            key="karpenter.k8s.aws/instance-size",
            operator="In",
            values=["medium"],
        ),
        lbrlabs_eks.RequirementArgs(
            key="karpenter.sh/capacity-type",
            operator="In",
            values=["spot"],
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
        }
    },
    opts=pulumi.ResourceOptions(provider=provider, parent=tailscale_ns),
)

example_labels = {
    "name": "example-server",
}

example_ns = k8s.core.v1.Namespace(
    "example",
    metadata=k8s.meta.v1.ObjectMetaArgs(name="example"),
)

example = k8s.apps.v1.Deployment(
    "example",
    metadata=k8s.meta.v1.ObjectMetaArgs(
        namespace=example_ns.metadata.name,
    ),
    spec=k8s.apps.v1.DeploymentSpecArgs(
        replicas=1,
        selector=k8s.meta.v1.LabelSelectorArgs(
            match_labels=example_labels,
        ),
        template=k8s.core.v1.PodTemplateSpecArgs(
            metadata=k8s.meta.v1.ObjectMetaArgs(
                labels=example_labels,
            ),
            spec=k8s.core.v1.PodSpecArgs(
                containers=[
                    k8s.core.v1.ContainerArgs(
                        name="example",
                        image="jaxxstorm/grpc-example:latest",
                        image_pull_policy="Always",
                        #ports=[k8s.core.v1.ContainerPortArgs(container_port=50051)],
                    ),
                ],
            ),
        ),
    ),
    opts=pulumi.ResourceOptions(provider=provider, parent=example_ns),
)

svc = k8s.core.v1.Service(
    "example",
    metadata=k8s.meta.v1.ObjectMetaArgs(
        namespace=example_ns.metadata.name,
    ),
    spec=k8s.core.v1.ServiceSpecArgs(
        type="LoadBalancer",
        selector=example_labels,
        load_balancer_class="tailscale",
        ports=[k8s.core.v1.ServicePortArgs(port=50051, target_port=50051)],
    ),
    opts=pulumi.ResourceOptions(provider=provider, parent=example_ns),
)

ingress = k8s.networking.v1.Ingress(
    "example",
    metadata=k8s.meta.v1.ObjectMetaArgs(
        namespace=example_ns.metadata.name,
    ),
    spec=k8s.networking.v1.IngressSpecArgs(
        default_backend=k8s.networking.v1.IngressBackendArgs(
            service=k8s.networking.v1.IngressServiceBackendArgs(
                name=svc.metadata.name,
                port=k8s.networking.v1.ServiceBackendPortArgs(
                    number=50051,
                ),
            )
        ),
        ingress_class_name="tailscale",
        tls=[
            k8s.networking.v1.IngressTLSArgs(
                hosts=["example"],
            ),
        ],
    ),
    opts=pulumi.ResourceOptions(provider=provider, parent=svc),
)
