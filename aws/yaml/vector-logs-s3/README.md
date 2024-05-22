# Streaming Tailscale logs to S3

This is a values.yaml for the vector helm chart that will support streaming logs to S3

The Tailscale operator should be installed in your cluster for this to work correctly.

## Install the vector helm chart

Using the values.yaml, replace your AWS keys (or set up IRSA), your S3 bucket and install vector:

```
helm upgrade vector vector/vector --namespace vector --create-namespace --values values.yaml
```

## Configure log streaming in Tailscale

This creates a device in Tailscale that can be used for log streaming. You should use the device name (likely vector-vector) like so:

```
http://vector-vector:8080/services/collector/event
```

The token can be any string.