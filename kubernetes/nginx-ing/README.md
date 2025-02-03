# Ingress Nginx with Tailscale

Tailscale offers an ingress controller which can be used to create a L7 ingress and offers managed DNS and automatic creation of certificates via Lets-Encrypt.

However, this _only_ allows using hostnames that are present on Tailscale's MagicDNS. Some organizations require using already existing DNS names - this directory shows one way this can be achieved.

## Install ingress-nginx

The first thing you need to do is install `ingress-nginx`. You'll create a new ingress class - I'd highly recommend not using the className `tailscale` because that's also used by the Tailscale kubernetes operator.

```bash
helm upgrade --install tailscale-ingress ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --namespace kube-system \
  --values helm/ingress-nginx/values.yml
```

Validate the ingress controller was installed, you should see a service of type LoadBalancer with a Tailscale IP address:

```bash
kubectl get svc -n kube-system -l app.kubernetes.io/name=ingress-nginx
NAME                                                        TYPE           CLUSTER-IP       EXTERNAL-IP                                                                       PORT(S)                      AGE
demo-us-west-nginx-ext-ingress-nginx-controller             LoadBalancer   10.100.55.158    <redacted>                                                                        80:31834/TCP,443:31367/TCP   3d2h
demo-us-west-nginx-ext-ingress-nginx-controller-admission   ClusterIP      10.100.3.68      <none>                                                                            443/TCP                      3d2h
tailscale-ingress-ingress-nginx-controller                  LoadBalancer   10.100.109.171   100.104.140.5,kube-system-tailscale-ingress-ingress-nginx-controller.lbr.ts.net   80:32468/TCP,443:30872/TCP   27m
tailscale-ingress-ingress-nginx-controller-admission        ClusterIP      10.100.177.12    <none>                            
```

Note, the `100.104.140.5` address here. If you're on the same tailnet, you should be able to hit the nginx and get a 404, because you have no ingress yet

```
curl 100.104.140.5
<html>
<head><title>404 Not Found</title></head>
<body>
<center><h1>404 Not Found</h1></center>
<hr><center>nginx</center>
</body>
</html>
```

You'll also have a new ingressclasses:

```bash
kubectl get ingressclass
NAME        CONTROLLER                      PARAMETERS   AGE
external    k8s.io/ingress-nginx/external   <none>       3d2h
tailscale   tailscale.com/ts-ingress        <none>       3d2h
ts          k8s.io/ingress-nginx/ts         <none>       24m
```

## Use the Ingress

Now we need to deploy a new app that uses this. You can see an example [nginx deployment]](app/nginx.yaml), the most important part is to set the `ingressClassName` to whatever your ingress name is, so for this example: `ts`.

Once that's done, you can see a new ingress like so:

```bash
kubectl get ing
NAME          CLASS   HOSTS               ADDRESS                                                                           PORTS   AGE
example-app   ts      nginx.briggs.work   100.104.140.5,kube-system-tailscale-ingress-ingress-nginx-controller.lbr.ts.net   80      24m
```

## Next Steps

Now you have a valid ingress, you can point your DNS provider at the Tailscale IP address, or if you have [External-DNS](https://kubernetes-sigs.github.io/external-dns/latest/) configured, it should automatically add a DNS entry for the hosts field.

### Cert Manager

This path will _also_ work with cert-manager, but it should be noted, you _must_ use [DNS-01](https://cert-manager.io/docs/configuration/acme/dns01/) challenges, as with `HTTP-01` lets-encrypt requires hitting the server for validation, and obviously it doesn't have access to your private tailnet.




