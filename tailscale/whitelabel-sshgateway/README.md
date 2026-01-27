# Whitelabeled SSH Gateway with Tailscale

A secure, web-based SSH gateway that uses Tailscale's tsnet to provide browser-based SSH access to machines in your tailnet. Features custom authentication and a modern terminal interface built with xterm.js.

## Features

- 🔐 **Tailscale Authentication**: Uses Tailscale's native SSH authentication - no passwords needed!
- 🌐 **Web-Based Terminal**: Full-featured terminal using xterm.js
- 🔒 **Secure by Default**: All connections through Tailscale's encrypted network with zero-trust security
- 🎨 **Whitelabeled**: Easy to customize and brand
- 📱 **Responsive Design**: Works on desktop and mobile browsers
- 🚀 **Zero Configuration Networking**: Uses Tailscale's tsnet for automatic mesh networking
- 🔑 **No SSH Keys or Passwords**: Authentication handled entirely by Tailscale's identity-aware proxy

## Prerequisites

- Go 1.24.7 or later
- A Tailscale account and auth kTailscale SSH enabled
  - Enable SSH on target machines: `tailscale up --ssh`
  - Or configure via admin console: https://login.tailscale.com/admin/acls
- Machines in your tailnet with SSH servers running

## Quick Start

### 1. Download Dependencies

```bash
go mod download
```

### 2. Set Environment Variables (Optional)

```bash
export WEB_USERNAME="admin"                # Optional: Web UI username (default: admin)
export WEB_PASSWORD="your-secure-password" # Optional: Web UI password (default: changeme)
export HOSTNAME="ssh-gateway"              # Optional: Tailscale hostname (default: ssh-gateway)
export LISTEN_PORT="8080"                  # Optional: HTTP listen port (default: 8080)
export STATE_DIR="./tsnet-state"           # Optional: tsnet state directory
```

### 3. Run the Gateway

```bash
go run .
```

On first run, the gateway will display a login URL:
```
To authenticate, visit:
	https://login.tailscale.com/a/xxxxxxxxxxxxx
```

Visit this URL in your browser and authenticate with your Tailscale account. The gateway will automatically connect once authenticated.

### 4. Access the Web Terminal

Once running, access the gateway at:
```
https://ssh-gateway:8080
```

Or use the full Tailscale DNS name shown in the logs.

## Configuration

All configuration is done via environment variables:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `WEB_USERNAME` | Web UI basic auth username | `admin` | No |
| `WEB_PASSWORD` | Web UI basic auth password | `changeme` | No |
| `HOSTNAME` | Tailscale hostname for this gateway | `ssh-gateway` | No |
| `LISTEN_PORT` | HTTP listen port | `8080` | No |
| `STATE_DIR` | Directory for tsnet state | `./tsnet-state` | No |

## How It Works
Tailscale Authentication**: Users authenticate via Tailscale's identity-aware proxy
3. **WebSocket Proxy**: Browser connects via WebSocket, gateway proxies raw SSH traffic over the tailnet
4. **Host Discovery**: Automatically discovers available hosts in your tailnet
5. **Terminal Emulation**: xterm.js provides a full-featured terminal in the browser
6. **Zero-Password SSH**: Tailscale SSH handles authentication - no SSH keys or passwords needed!
4. **Host Discovery**: Automatically discovers available hosts in your tailnet
5. **Terminal Emulation**: xterm.js provides a full-featured terminal in the browser

## ArchitectureTailscale    ┌─────────────┐
│   Web Browser   │ ◄─────────────────► │   SSH Gateway    │      SSH       │ Target Host │
│   (xterm.js)    │   HTTP Basic Auth    │   (tsnet node)   │ ◄───────────► │  (Tailnet)  │
└─────────────────┘                      └──────────────────┘   Encrypted    └─────────────┘
                                                ▲                Connection
                                                │
                                         Tailscale Auth
                                      (No passwords needed)
│   Web Browser   │ ◄─────────────────► │   SSH Gateway    │ ◄───────────► │ Target Host │
│   (xterm.js)    │   HTTP Basic Auth    │   (tsnet node)   │   Tailscale   │  (tailnet)  │
└─────────────────┘                      └──────────────────┘                └─────────────┘ for the web UI
2. **Configure Tailscale ACLs**: Set up proper access controls in your Tailscale admin console
   - Define who can access which SSH hosts
   - Use Tailscale's SSH ACL rules for fine-grained control
3. **Implement CORS**: Update `CheckOrigin` in the WebSocket upgrader for production origins
4. **Add Rate Limiting**: Protect against abuse
5. **Audit Logging**: Add comprehensive logging for security audits
6. **TLS Configuration**: tsnet provides automatic HTTPS via Tailscale's infrastructure
7. **Enable Tailscale SSH**: Ensure target hosts have Tailscale SSH enabled
   ```bash
   tailscale up --ssh
   ```
1. **Change Default Credentials**: Always set `WEB_USERNAME` and `WEB_PASSWORD` to strong values
2. **Enable Host Key Verification**: Replace `ssh.InsecureIgnoreHostKey()` with proper host key verification
3. **Implement CORS**: Update `CheckOrigin` in the WebSocket upgrader
4. **Add Rate Limiting**: Protect against brute force attacks
5. **Audit Logging**: Add comprehensive logging for security audits
6. **TLS Configuration**: tsnet provides automatic HTTPS via Tailscale's infrastructure
7. **Password Handling**: Current implementation uses password auth; consider using SSH keys instead

## Customization

### Branding

Edit [web.go](web.go) to customize:
- Page title and header
- Color scheme (CSS variables)
- Logo and branding elements
- Terminal theme colors

### Authentication

Extend the `basicAuth` method in [main.go](main.go) to:
- Integrate with OAuth/OIDC providers
- Use JWT tokens
- Implement MFA
- Connect to external auth services

### Terminal Features

The xterm.js terminal supports:
- Copy/paste
- Search
- Unicode and emoji
- Multiple color themes
- Customizable fonts and sizes

## Building for Production

### Build binary:
```bash
go build -o ssh-gateway .
```

### Run as a service (systemd example):

```ini
[Unit]
Description=SSH Gateway
After=network.target

[Service]
Type=simple
User=sshgateway
WorkingDirectory=/opt/ssh-gateway
Environment="WEB_USERNAME=admin"
Environment="WEB_PASSWORD=your-secure-password"
ExecStart=/opt/ssh-gateway/ssh-gateway
Restart=always

[Install]
WantedBy=multi-user.target
```

### Docker:

```dockerfile
FROM golang:1.24.7-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o ssh-gateway .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ssh-gateway .
CMD ["./ssh-gateway"]
```

## Troubleshooting

### Can't connect to tailnet
- Visit the login URL displayed on first run
- Authenticate with your Tailscale account
- Check that you have permission to add machines to your tailnet
- Ensure network connectivity
- State is persisted in `STATE_DIR` - delete it to re-authenticate

### SSH connection fails
- Verify target host has Tailscale SSH enabled: `tailscale up --ssh`
- Check Tailscale ACLs allow SSH access between nodes
- Verify target host is accessible in your tailnet
- Check that you have permission to SSH to the target user in Tailscale ACLs

### WebSocket connection fails
- Ensure you're accessing via the Tailscale network
- Check browser console for errors
- Verify basic auth credentials

## API Endpoints

- `GET /` - Web terminal interface
- `GET /api/hosts` - List available hosts in tailnet (requires auth)
- `GET /ws/ssh?host=<host>&user=<user>` - WebSocket SSH proxy (requires auth)

## License

MIT

## Contributing

Contributions welcome! Please open issues or pull requests.

## Resources

- [Tailscale Documentation](https://tailscale.com/kb/)
- [tsnet Package](https://pkg.go.dev/tailscale.com/tsnet)
- [xterm.js](https://xtermjs.org/)
- [Go SSH Package](https://pkg.go.dev/golang.org/x/crypto/ssh)
