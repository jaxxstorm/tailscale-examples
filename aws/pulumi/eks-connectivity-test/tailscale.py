import pulumi_kubernetes as k8s
import pulumi
from typing import Sequence


class Client(pulumi.ComponentResource):

    svc: k8s.core.v1.Service
    secret: k8s.core.v1.Secret
    service_account: k8s.core.v1.ServiceAccount
    role: k8s.rbac.v1.Role
    role_binding: k8s.rbac.v1.RoleBinding
    statefulset: k8s.apps.v1.StatefulSet

    def __init__(
        self,
        name,
        namespace: pulumi.Input[str],
        toleration_value: pulumi.Input[str],
        auth_key: pulumi.Input[str],
        host_network: pulumi.Input[bool] = False,
        expose_host_port: pulumi.Input[bool] = False,
        hostname: pulumi.Input[str] = None,
        port: pulumi.Input[int] = None,
        opts=None,
    ):
        super().__init__("jaxxstorm:index:TailscaleClient", name, {}, opts)

        self.svc = k8s.core.v1.Service(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                labels={"app": name},
                namespace=namespace,
            ),
            spec=k8s.core.v1.ServiceSpecArgs(
                cluster_ip="None",
                selector={"app": name},
            ),
            opts=pulumi.ResourceOptions(parent=self),
        )

        self.secret = k8s.core.v1.Secret(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                labels={"app": name},
                namespace=namespace,
            ),
            string_data={
                "TS_AUTH_KEY": auth_key,
            },
            opts=pulumi.ResourceOptions(parent=self),
        )

        self.service_account = k8s.core.v1.ServiceAccount(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                namespace=namespace,
            ),
            opts=pulumi.ResourceOptions(parent=self),
        )

        self.role = k8s.rbac.v1.Role(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                namespace=namespace,
            ),
            rules=[
                k8s.rbac.v1.PolicyRuleArgs(
                    verbs=["*"],
                    resources=["secrets"],
                    api_groups=[""],
                )
            ],
            opts=pulumi.ResourceOptions(parent=self),
        )

        self.role_binding = k8s.rbac.v1.RoleBinding(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                namespace=namespace,
            ),
            role_ref=k8s.rbac.v1.RoleRefArgs(
                api_group="rbac.authorization.k8s.io",
                kind="Role",
                name=self.role.metadata.name,
            ),
            subjects=[
                k8s.rbac.v1.SubjectArgs(
                    kind="ServiceAccount",
                    name=self.service_account.metadata.name,
                )
            ],
        )

        dns_policy: pulumi.Input[str] = "ClusterFirst"

        if host_network:
            dns_policy = "ClusterFirstWithHostNet"

        env: pulumi.Input[Sequence[pulumi.Input[pulumi.core.v1.EnvVarArgs]]] = [
            k8s.core.v1.EnvVarArgs(name="TS_USERSPACE", value="false"),
            k8s.core.v1.EnvVarArgs(
                name="POD_IP",
                value_from=k8s.core.v1.EnvVarSourceArgs(
                    field_ref=k8s.core.v1.ObjectFieldSelectorArgs(
                        field_path="status.podIP"
                    )
                ),
            ),
            k8s.core.v1.EnvVarArgs(
                name="TS_TAILSCALED_EXTRA_ARGS",
                value="--debug=0.0.0.0:8181",
            ),
            k8s.core.v1.EnvVarArgs(name="TS_DEBUG_FIREWALL_MODE", value="auto"),
            k8s.core.v1.EnvVarArgs(
                name="TS_EXTRA_ARGS",
                value="--advertise-tags=tag:connectivity-test",
            ),
            k8s.core.v1.EnvVarArgs(
                name="TS_KUBE_SECRET",
                value=self.secret.metadata.name,
            ),
            k8s.core.v1.EnvVarArgs(
                name="TS_AUTH_KEY",
                value_from=k8s.core.v1.EnvVarSourceArgs(
                    secret_key_ref=k8s.core.v1.SecretKeySelectorArgs(
                        name=self.secret.metadata.name,
                        key="TS_AUTH_KEY",
                    )
                ),
            ),
            k8s.core.v1.EnvVarArgs(
                name="TS_STATE_DIR",
                value="mem:",
            ),
        ]
        
        if hostname:
            env.append(
                k8s.core.v1.EnvVarArgs(
                    name="TS_HOSTNAME",
                    value=hostname,
                )
            )
            
        if port:
            env.append(
                k8s.core.v1.EnvVarArgs(
                    name="PORT",
                    value=str(port),
                )
            )
            
        else:
            port = 41641
        
        if expose_host_port:
            container_ports = [k8s.core.v1.ContainerPortArgs(container_port=port, host_port=port, protocol="UDP")]
        else:
            container_ports = None

        self.statefulset = k8s.apps.v1.StatefulSet(
            name,
            metadata=k8s.meta.v1.ObjectMetaArgs(
                namespace=namespace,
            ),
            spec=k8s.apps.v1.StatefulSetSpecArgs(
                replicas=1,
                selector=k8s.meta.v1.LabelSelectorArgs(match_labels={"app": name}),
                service_name=self.svc.metadata.name,
                template=k8s.core.v1.PodTemplateSpecArgs(
                    metadata=k8s.meta.v1.ObjectMetaArgs(labels={"app": name}),
                    spec=k8s.core.v1.PodSpecArgs(
                        service_account_name=self.service_account.metadata.name,
                        host_network=host_network,
                        dns_policy=dns_policy,
                        containers=[
                            k8s.core.v1.ContainerArgs(
                                name="tailscale",
                                image="tailscale/tailscale:v1.61.11",
                                env=env,
                                ports=container_ports,
                                security_context=k8s.core.v1.SecurityContextArgs(
                                    capabilities=k8s.core.v1.CapabilitiesArgs(
                                        add=["NET_ADMIN"]
                                    )
                                ),
                            )
                        ],
                        init_containers=[
                            k8s.core.v1.ContainerArgs(
                                name="sysctler",
                                image="tailscale/tailscale:v1.61.11",
                                command=["/bin/sh"],
                                args=[
                                    "-c",
                                    "sysctl -w net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1",
                                ],
                                security_context=k8s.core.v1.SecurityContextArgs(
                                    privileged=True
                                ),
                            )
                        ],
                        tolerations=[
                            k8s.core.v1.TolerationArgs(
                                key="tailscale.com",
                                value=toleration_value,
                                effect="NoSchedule",
                            )
                        ],
                    ),
                ),
            ),
            opts=pulumi.ResourceOptions(parent=self),
        )

        self.register_outputs({})
