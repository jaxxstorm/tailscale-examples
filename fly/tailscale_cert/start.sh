#!/bin/sh
# testing simple ts start up
/app/tailscaled --tun=userspace-networking --socks5-server=localhost:1337 --outbound-http-proxy-listen=localhost:1337 &
#/app/tailscaled --tun=userspace-networking

# oauth key
# og device key
/app/tailscale up --auth-key=${TAILSCALE_AUTHKEY} --hostname=tailscale-fly-app
echo "running cert command"
/app/tailscale cert $(/app/tailscale status --json | jq -r .Self.DNSName | sed 's/.$//')

echo "running tailscale serve"
/app/tailscale serve --https=443 --bg http://localhost:8080

echo "checking tailscale serve status"
/app/tailscale serve status

echo "starting npm app"
npm config set proxy http://localhost:1337/
npm start


