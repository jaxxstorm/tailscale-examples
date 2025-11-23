# Tailscale Workload Identity Federation with tsnet

A minimal example demonstrating how to use workload identity federation to authenticate a tsnet server with Tailscale.

## What it does

This program automates the process of exchanging a JWT token for Tailscale credentials:

1. Takes a JWT token (from your identity provider)
2. Exchanges the JWT for a Tailscale access token via the OAuth token exchange endpoint
3. Uses the access token to create an auth key
4. Starts a tsnet server using the auth key

This is useful when running in environments like Kubernetes, GitHub Actions, or cloud platforms where you can obtain OIDC tokens but want to avoid storing long-lived Tailscale credentials.

## Usage

```bash
./tsnet-wif \
  --client-id=<your-tailscale-oauth-client-id> \
  --jwt=<your-jwt-token> \
  --tag=tag:your-tag \
  --hostname=my-service \
  --port=:8080
```

### Environment Variables

You can also use environment variables:

- `TS_WIF_CLIENT_ID`: Tailscale workload identity client ID (required)
- `TS_WIF_JWT`: JWT token from your identity provider (required)
- `TS_TAG`: Tailscale tag for the device (default: `tag:tsnet-wif-demo`)
- `TS_HOSTNAME`: Hostname for the tsnet server (default: `tsnet-wif-demo`)
- `TS_PORT`: Port to listen on (default: `:8080`)

## Prerequisites

1. A Tailscale account with a configured OIDC client for workload identity federation
2. A JWT token from your identity provider (GitHub, GCP, AWS, etc.)
3. Appropriate ACL tags configured in your Tailscale policy

## Building

```bash
go build -o tsnet-wif .
```

## Testing

```bash
go test -v
```

The test suite includes:

- JWT to access token exchange validation
- Auth key creation with proper Authorization headers
- Error handling for invalid tokens and failed requests
