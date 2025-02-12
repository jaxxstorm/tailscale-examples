# Tailscale Envoy Demo

This demo repo showcases some of the possibilities when combining Tailscale with Envoy. It consists of 3 components:

- A control plane written using [control-plane-go](https://github.com/envoyproxy/go-control-plane) that runs inside a [tsnet](https://tailscale.com/kb/1244/tsnet) go process, and collects tags from Tailscale's API to add to the envoy cluster
- A simple webserver, that runs an envoy proxy and Tailscale as a sidecar. This allows the envoy instance to register with the above control plane and send traffic across a wide variety of infrastructure - it no longer matters where your applications are
- A client, who's only job is to send requests to the above server and report back.

## Step by Step Guide

### Prerequisites

You'll need a tailnet, a Tailscale auth key and some tags defined, for example:

```json
"tagOwners": {
		"tag:weight-1": ["autogroup:admin"],
		"tag:weight-2": ["autogroup:admin"],
		"tag:weight-3": ["autogroup:admin"],
	},
```

The weight here is used to determine how envoy routes traffic through your tailnet.

### Provision the infrastructure

The `deploy/infra` directory contains two EKS clusters, one in `us-east-1` and one in `us-west-2`. Envoy will be used to direct traffic seamlessly over the Tailnet.

You'll need to grab the kubeconfig from both clusters:

```bash
aws eks update-kubeconfig --name lbr-envoy-east --kubeconfig .kubeconfig
aws eks update-kubeconfig --name lbr-envoy-west --kubeconfig .kubeconfig
```

and you can use tools like `kubectx` to switch between them.

### Create a secret

Next, we need to create a couple of secrets. Create a Tailscale auth key and an API key, then create a secret in both clusters (you'll need to switch contexts) like so:

```bash
kubectl create secret generic tailscale-envoy \
  --from-literal=TS_ENVOY_AUTHKEY=tskey-auth-<redacted> \
  --from-literal=TS_ENVOY_API_TOKEN=tskey-api-<redacted>
```

### Deploy the control plane

Now we have the required secret, we can deploy the control plane. This should be a simple:

```
kubectl apply -f deploy/controlplane/manifest.yaml
```

You can validate the control exists like and view its logs:

```bash
k get pods 
NAME                                   READY   STATUS    RESTARTS   AGE
envoy-control-plane-666cc6848c-hzz58   1/1     Running   0          32m
```

```bash
k logs -l app=envoy-control-plane
```

You should see the control plane present in your tailnet, make a note of the IP address it has assigned, you'll need it for the next step

### Deploy the server in west

The server component is a simple webserver that has a Tailscale sidecar and an envoy sidecar. Envoy does all the magic here, and we'll see this later. 

Let's deploy the server component. It needs some templated values, so we'll use helm to do that:

```bash
helm install ts-envoy-server ./deploy/server --set controlPlaneHost="<your-controlplane-ip>"
```

This should deploy two servers in your tailnet, which should register with your Envoy control plane. You can view the logs for the server:

```bash
k logs -l app=tailscale-server
Defaulted container "example-server" out of: example-server, envoy, tailscale-sidecar
Defaulted container "example-server" out of: example-server, envoy, tailscale-sidecar
2025/02/12 02:59:33 Server starting on :8080, host = tailscale-servers-56cd5ddbfd-fhfpb
2025/02/12 02:59:33 Server starting on :8080, host = tailscale-servers-56cd5ddbfd-j4gpr
```

And you can also see the logs from envoy and the Tailscale sidecar


```bash
k logs -l app=tailscale-server -c envoy
[2025-02-12 03:07:23.445][1][info][upstream] [source/common/upstream/cds_api_helper.cc:32] cds: add 1 cluster(s), remove 1 cluster(s)
[2025-02-12 03:07:23.445][1][info][upstream] [source/common/upstream/cds_api_helper.cc:69] cds: added/updated 0 cluster(s), skipped 1 unmodified cluster(s)
```

Note: the `add 1 cluster` means things are working great

```bash
k logs -l app=tailscale-server -c tailscale-sidecar
2025/02/12 02:59:53 wgengine: Reconfig: configuring userspace WireGuard config (with 2/3 peers)
2025/02/12 02:59:53 magicsock: disco: node [jMEaP] d:4d603761fc7e1dd0 now using 10.1.2.121:58635 mtu=1360 tx=3d3704ebd3a5
2025/02/12 03:05:18 Received error: PollNetMap: unexpected EOF
2025/02/12 03:05:19 control: controlhttp: forcing port 443 dial due to recent noise dial
2025/02/12 03:05:19 control: controlhttp: forcing port 443 dial due to recent noise dial
2025/02/12 03:05:19 control: controlhttp: forcing port 443 dial due to recent noise dial
2025/02/12 03:05:19 control: controlhttp: forcing port 443 dial due to recent noise dial
2025/02/12 03:05:19 control: controlhttp: forcing port 443 dial due to recent noise dial
2025/02/12 03:05:19 [RATELIMIT] format("control: controlhttp: forcing port 443 dial due to recent noise dial")
2025/02/12 03:05:19 control: netmap: got new dial plan from control
2025/02/12 02:59:35 control: NetInfo: NetInfo{varies=true hairpin= ipv6=false ipv6os=true udp=true icmpv4=false derp=#10 portmap= link="" firewallmode="ipt-default"}
2025/02/12 02:59:35 magicsock: endpoints changed: 52.32.250.12:15157 (stun), 10.1.2.121:58635 (local)
2025/02/12 02:59:35 health(warnable=no-derp-connection): ok
2025/02/12 02:59:35 [RATELIMIT] format("health(warnable=%s): ok")
boot: 2025/02/12 02:59:35 Startup complete, waiting for shutdown signal
2025/02/12 02:59:35 magicsock: derp-10 connected; connGen=1
2025/02/12 02:59:35 wgengine: Reconfig: configuring userspace WireGuard config (with 1/3 peers)
2025/02/12 02:59:35 magicsock: disco: node [gMIp5] d:3d8b11d4c9c1f28c now using 10.1.2.175:43305 mtu=1360 tx=0042ccfc81e9
2025/02/12 02:59:53 wgengine: Reconfig: configuring userspace WireGuard config (with 2/3 peers)
2025/02/12 02:59:53 magicsock: disco: node [lDLkU] d:3b1144192d58c574 now using 10.1.2.205:35864 mtu=1360 tx=ed9f40a11024
```

This is the Tailscale sidecar that's able to see all the other endpoints.

### First Test

Now, because we've registered the server component with Tailscale, we can actually hit it over Tailscale.

You should see two clients in your tailnet:

```bash

tailscale status
100.84.60.2     lbr-macbook-pro      mail@        macOS   -
100.78.104.15   tailscale-servers-56cd5ddbfd-fhfpb userid:9e68eea0127a6 linux   - <-- pod 2
100.89.80.53    tailscale-servers-56cd5ddbfd-j4gpr userid:9e68eea0127a6 linux   - <-- pod 1
100.120.143.2   tsnet-dynamic-eds    mail@        linux   -
```

So let's hit our webserver directly over Tailscale:

```bash
curl http://100.78.104.15:8080
Hello from tailscale-servers-56cd5ddbfd-fhfpb! in west
```

That's great, but not particularly impressive. However, because we have envoy alongside this, we can start to do some more interesting stuff. There's a client in the `cmd/client` directory that showcases this. 

Try the following:

```bash
go run cmd/client/client.go http://100.78.104.15:8080 10
Hello from tailscale-servers-56cd5ddbfd-fhfpb! in west: 100.0% (10)
Failed: 0
```

Here, we sent 10 requests to the Tailscale pod and they all returned.

Now, here's where envoy comes in. Let's send those same requests to _envoy_ instead of directly to the webserver:

```bash
# note: 10000 is the envoy port
go run cmd/client/client.go http://100.78.104.15:10000 20
Hello from tailscale-servers-56cd5ddbfd-j4gpr! in west: 50.0% (10)
Hello from tailscale-servers-56cd5ddbfd-fhfpb! in west: 50.0% (10)
Failed: 0
```

This is more interesting! We've hit both pods because envoy has loadbalanced the requests for us.

### Deploy the same infra to east

Okay, so far all we've done is achieve what a standard k8s service can do. Let's make this interesting by deploying the server to a completely remote cluster disconnected from our infrastructure:


```bash
kubectx lbr-envoy-east
Switched to context "lbr-envoy-east".

helm install ts-envoy-server ./deploy/server --set controlPlaneHost="100.120.143.2" --set serverLocality=east --set advertiseTags=--advertise-tags=tag:weight-2
```

Notice how we've also changed the weight here, intentionally. 

Now, let's run our test again:

```bash
go run cmd/client/client.go http://100.78.104.15:10000 50
Hello from tailscale-servers-56cd5ddbfd-j4gpr! in west: 18.0% (9)
Hello from tailscale-servers-7f98889799-7bjts! in east: 32.0% (16)
Hello from tailscale-servers-56cd5ddbfd-fhfpb! in west: 16.0% (8)
Hello from tailscale-servers-7f98889799-9vjlc! in east: 34.0% (17)
Failed: 0
```

Would you look at that, I've been able to send all my traffic to _both_ clusters! Envoy has managed the weighting for me via the controlplane, and figured out how to route traffic for me. Tailscale is doing all the connectivity!

### One step further..

Okay, final thing. The server has an endpoint that means you can remove a node from service with the `/unhealthy` endpoint. So let's choose a server at random and remove it:

```bash
tailscale status
100.84.60.2     lbr-macbook-pro      mail@        macOS   -
100.78.104.15   tailscale-servers-56cd5ddbfd-fhfpb userid:9e68eea0127a6 linux   idle, tx 25324 rx 31568
100.89.80.53    tailscale-servers-56cd5ddbfd-j4gpr userid:9e68eea0127a6 linux   -
100.73.9.82     tailscale-servers-7f98889799-7bjts userid:9e68eea0127a6 linux   -
100.114.155.33  tailscale-servers-7f98889799-9vjlc userid:9e68eea0127a6 linux   -
100.120.143.2   tsnet-dynamic-eds    mail@        linux   -
```

`100.78.104.15` is our unlucky host, sorry!

```bash
curl http://100.78.104.15:8080/unhealthy
[tailscale-servers-56cd5ddbfd-fhfpb] Set to unhealthy
```

Okay, now what happens if we rerun our test?

```bash
go run cmd/client/client.go http://100.78.104.15:10000 50
Hello from tailscale-servers-7f98889799-9vjlc! in east: 38.0% (19)
Hello from tailscale-servers-7f98889799-7bjts! in east: 36.0% (18)
Hello from tailscale-servers-56cd5ddbfd-j4gpr! in west: 26.0% (13)
```

A much different weighting!

### Final notes

I'm sending the requests to a single Tailscale IP, but it doesn't actually matter, I can change the destination any time, Envoy takes in a request and routes it through the network, for example:

```bash
# Note the different IP
go run cmd/client/client.go http://100.114.155.33:10000 50
Hello from tailscale-servers-7f98889799-9vjlc! in east: 38.0% (19)
Hello from tailscale-servers-7f98889799-7bjts! in east: 36.0% (18)
Hello from tailscale-servers-56cd5ddbfd-j4gpr! in west: 26.0% (13)
Failed: 0
```

Same result!

## Enhancements

This is a very simple example, with a very simple weighting. I've (ab)used Tailscale tags for weighting, we should have metadata to program these tags where possible. 

Envoy supports a variety of different loadbalancing mechanisms, this one leverages [locality weight](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/locality_weight)

With this implementation, we get all the power of Envoy, without the connectivity of Tailscale!
