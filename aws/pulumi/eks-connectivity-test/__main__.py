import pulumi
import pulumi_aws as aws
import pulumi_awsx as awsx
import lbrlabs_pulumi_eks as eks
import pulumi_kubernetes as k8s
import tailscale as ts


CONFIG = pulumi.Config()
TS_AUTH_KEY = CONFIG.require("ts_auth_key")

vpc = awsx.ec2.Vpc(
    "lbr-conn-test",
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
    "lbr-conn-test",
    cluster_subnet_ids=vpc.public_subnet_ids,
    system_node_subnet_ids=vpc.private_subnet_ids,
    system_node_instance_types=["t3.large"],
    system_node_desired_count=1,
    enable_external_ingress=False,
    enable_internal_ingress=False,
)

cluster_sg = cluster.control_plane.vpc_config.cluster_security_group_id

aws.ec2.SecurityGroupRule(
    "tailscale",
    to_port=41641,
    from_port=41641,
    protocol="udp",
    type="ingress",
    cidr_blocks=["0.0.0.0/0"],
    security_group_id=cluster_sg,
)

aws.ec2.SecurityGroupRule(
    "tailscale-conn-port",
    to_port=12345,
    from_port=12345,
    protocol="udp",
    type="ingress",
    cidr_blocks=["0.0.0.0/0"],
    security_group_id=cluster_sg,
)

provider = k8s.Provider("provider", kubeconfig=cluster.kubeconfig)


public = eks.AttachedNodeGroup(
    "lbr-conn-test-public",
    cluster_name=cluster.cluster_name,
    subnet_ids=vpc.public_subnet_ids,
    instance_types=["t3.large"],
    scaling_config=aws.eks.NodeGroupScalingConfigArgs(
        desired_size=1,
        min_size=1,
        max_size=1,
    ),
    taints=[
        aws.eks.NodeGroupTaintArgs(
            key="tailscale.com",
            value="public",
            effect="NO_SCHEDULE",
        )
    ],
    opts=pulumi.ResourceOptions(
        parent=cluster,
        providers={"kubernetes": provider},
    ),
)

private = eks.AttachedNodeGroup(
    "lbr-conn-test-private",
    cluster_name=cluster.cluster_name,
    subnet_ids=vpc.private_subnet_ids,
    instance_types=["t3.large"],
    scaling_config=aws.eks.NodeGroupScalingConfigArgs(
        desired_size=1,
        min_size=1,
        max_size=1,
    ),
    taints=[
        aws.eks.NodeGroupTaintArgs(
            key="tailscale.com",
            value="private",
            effect="NO_SCHEDULE",
        )
    ],
    opts=pulumi.ResourceOptions(
        parent=cluster,
        providers={"kubernetes": provider},
    ),
)

ts.Client(
    "public",
    namespace="default",
    toleration_value="public",
    auth_key=TS_AUTH_KEY,
    host_network=False,
    hostname="public",
    port=12345,
    expose_host_port=False,
    opts=pulumi.ResourceOptions(
        provider=provider,
    )
)

ts.Client(
    "private",
    namespace="default",
    toleration_value="private",
    auth_key=TS_AUTH_KEY,
    host_network=False,
    hostname="private",
    port=12345,
    expose_host_port=False,
    opts=pulumi.ResourceOptions(
        provider=provider,
    )
)

pulumi.export("kubeconfig", cluster.kubeconfig)

