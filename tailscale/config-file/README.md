# About

Tailscale is generally configured with the command `tailscale up` which has flags for configuring the behaviour of the `tailscaled` daemon.

If you require more declarative behaviour, you can also use a config file, which supports many of the flags.

You can see the config struct [here](https://github.com/tailscale/tailscale/blob/179745b83ed6d687bdc9d501ccdbfdec1cb3f9d7/ipn/conf.go#L15-L55)

You pass this config file to the `tailscaled` process - **NOT** the `tailscale` CLI command (remember, the tailscale CLI command just configures `tailscaled`)

To configure this, the easiest way is to update the `defaults` file on your Linux operating system. For example, the `systemd` unit file for Tailscale on Ubuntu looks like this:

```
systemctl cat tailscaled
# /usr/lib/systemd/system/tailscaled.service
[Unit]
Description=Tailscale node agent
Documentation=https://tailscale.com/kb/
Wants=network-pre.target
After=network-pre.target NetworkManager.service systemd-resolved.service

[Service]
EnvironmentFile=/etc/default/tailscaled
ExecStart=/usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/run/tailscale/tailscaled.sock --port=${PORT} $FLAGS
ExecStopPost=/usr/sbin/tailscaled --cleanup

Restart=on-failure

RuntimeDirectory=tailscale
RuntimeDirectoryMode=0755
StateDirectory=tailscale
StateDirectoryMode=0700
CacheDirectory=tailscale
CacheDirectoryMode=0750
Type=notify

[Install]
WantedBy=multi-user.target
```

All environment variables are sourced from `EnvironmentFile=/etc/default/tailscaled` and in the `ExecStart` we specify a `$FLAGS` env var. Looking at the default `/etc/default/tailscaled`:

```
# Set the port to listen on for incoming VPN packets.
# Remote nodes will automatically be informed about the new port number,
# but you might want to configure this in order to set external firewall
# settings.
PORT="41641"

# Extra flags you might want to pass to tailscaled.
FLAGS="-config /etc/tailscale/ts-config.hujson" # you can also use .json obviously
```


