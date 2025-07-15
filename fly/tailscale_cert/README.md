# Tailscale + Fly.io API

A simple Node.js API demonstrating Tailscale VPN integration with Fly.io deployment.

## Features

- **Express.js API Server**: RESTful API endpoints for testing
- **Tailscale Integration**: Secure VPN connectivity with status monitoring  
- **Fly.io Deployment**: Cloud deployment with single instance configuration
- **Ping Testing**: ICMP ping tests through Tailscale network
- **Real-time Monitoring**: Health checks and status updates

## API Endpoints

### Core Endpoints
- `GET /` - API overview and endpoint list
- `GET /api/status` - App status and system information
- `GET /api/health` - Simple health check endpoint

### Tailscale Endpoints
- `GET /api/tailscale` - Tailscale connection status and peer information
- `GET /api/tailscale/peers` - List all Tailscale peers
- `GET /api/tailscale/test` - Test connectivity to online peers
- `POST /api/tailscale/ping/:host` - Ping a specific host
- `POST /api/tailscale/ping-test` - Ping multiple hosts
- `POST /api/tailscale/test/:device` - Test connectivity to specific device

### Utility Endpoints
- `POST /api/echo` - Echo endpoint for testing

## Curl Examples

Replace `your-app.fly.dev` with your actual Fly.io app URL.

### Basic API Information
```bash
# Get API overview
curl https://your-app.fly.dev/

# App status and system info
curl https://your-app.fly.dev/api/status

# Health check
curl https://your-app.fly.dev/api/health
```

### Tailscale Status and Peers
```bash
# Get Tailscale connection status
curl https://your-app.fly.dev/api/tailscale

# List all Tailscale peers
curl https://your-app.fly.dev/api/tailscale/peers

# Test connectivity to online peers
curl https://your-app.fly.dev/api/tailscale/test
```

### Ping Testing
```bash
# Ping a specific Tailscale IP
curl -X POST https://your-app.fly.dev/api/tailscale/ping/100.97.31.62 \
  -H "Content-Type: application/json" \
  -d '{"count": 4, "timeout": 10}'

# Ping with custom packet count
curl -X POST https://your-app.fly.dev/api/tailscale/ping/100.97.31.62 \
  -H "Content-Type: application/json" \
  -d '{"count": 10}'

# Ping multiple hosts
curl -X POST https://your-app.fly.dev/api/tailscale/ping-test \
  -H "Content-Type: application/json" \
  -d '{"hosts": ["100.97.31.62", "100.124.64.128"], "count": 3}'
```

### Device Testing
```bash
# Ping test to specific device
curl -X POST https://your-app.fly.dev/api/tailscale/test/100.97.31.62 \
  -H "Content-Type: application/json" \
  -d '{"method": "ping"}'

# TCP connectivity test
curl -X POST https://your-app.fly.dev/api/tailscale/test/100.97.31.62 \
  -H "Content-Type: application/json" \
  -d '{"port": 22, "method": "tcp"}'
```

### Utility Testing
```bash
# Echo test
curl -X POST https://your-app.fly.dev/api/echo \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello from Tailscale!", "timestamp": "2025-07-15"}'
```

### Example Response Formats

#### Tailscale Status Response
```json
{
  "status": "connected",
  "tailscale": {
    "self": {
      "DNSName": "tailscale-fly-app.tail4cf751.ts.net.",
      "TailscaleIPs": ["100.124.64.128"]
    },
    "peers": 2,
    "magicDNSSuffix": "tail4cf751.ts.net",
    "currentTailnet": {
      "Name": "example.ts.net"
    }
  },
  "timestamp": "2025-07-15T14:30:00.000Z"
}
```

#### Ping Response
```json
{
  "success": true,
  "host": "100.97.31.62",
  "packets": {
    "transmitted": 4,
    "received": 4,
    "loss": "0%"
  },
  "timing": {
    "min": 25.2,
    "avg": 28.7,
    "max": 32.1,
    "unit": "ms"
  },
  "timestamp": "2025-07-15T14:30:00.000Z"
}
```

## Local Development

```bash
# Install dependencies
npm install

# Start the application
npm start
```

The API will be available at `http://localhost:8080`

## Deployment to Fly.io

The app is configured to deploy to Fly.io with Tailscale integration:

```bash
# Deploy to Fly.io
fly deploy

# Check app status
fly status

# View logs
fly logs

# Test the deployed API
curl https://your-app.fly.dev/
```

## Configuration

The app uses the following environment variables:

- `PORT` - Server port (default: 8080)
- `NODE_ENV` - Environment mode
- `TAILSCALE_AUTHKEY` - Tailscale authentication key (required)

## Tailscale Integration

The app integrates with Tailscale to provide:

- **Secure VPN connectivity** through userspace networking
- **Real-time status monitoring** via `/api/tailscale`
- **Peer discovery** and connectivity testing
- **ICMP ping testing** through the Tailscale network
- **Certificate generation** for HTTPS access
- **TCP serving** on port 8443 within the Tailnet

### Startup Sequence

The `start.sh` script handles Tailscale setup:

1. **Start Tailscale daemon** with userspace networking and SOCKS5 proxy
2. **Connect to Tailnet** using the provided auth key  
3. **Generate TLS certificates** for the device
4. **Start Node.js API** in background
5. **Enable Tailscale serve** on port 8443
6. **Monitor processes** and keep container running

### Access Methods

After deployment, you can access the API via:

- **Public Fly.io URL**: `https://your-app.fly.dev` (ports 80/443)
- **Tailscale Network**: `https://tailscale-fly-app.tail4cf751.ts.net:8443`
- **Direct Tailscale IP**: `https://100.124.64.128:8443` (with valid certificates)

## Quick Start

1. **Set up Tailscale auth key**:
   ```bash
   fly secrets set TAILSCALE_AUTHKEY=your-auth-key-here
   ```

2. **Deploy the app**:
   ```bash
   fly deploy
   ```

3. **Test basic connectivity**:
   ```bash
   curl https://your-app.fly.dev/api/health
   ```

4. **Check Tailscale status**:
   ```bash
   curl https://your-app.fly.dev/api/tailscale
   ```

5. **Test ping to another device**:
   ```bash
   curl -X POST https://your-app.fly.dev/api/tailscale/ping/100.97.31.62 \
     -H "Content-Type: application/json" \
     -d '{"count": 4}'
   ```

## File Structure

```
.
├── Dockerfile          # Container configuration
├── fly.toml           # Fly.io deployment configuration  
├── start.sh           # Startup script with Tailscale setup
├── package.json       # Node.js dependencies and scripts
├── app.js            # Main API application code
└── README.md         # This documentation
```

## Troubleshooting

### Common Issues

- **503 errors**: Check that the app is listening on `0.0.0.0:8080`
- **Tailscale connection issues**: Verify `TAILSCALE_AUTHKEY` is set correctly
- **Ping failures**: Ensure target devices are online and reachable

### Useful Commands

```bash
# Check app logs
fly logs

# SSH into the container
fly ssh console

# Check Tailscale status inside container
fly ssh console -C "/app/tailscale status"

# Restart the app
fly deploy --strategy immediate
```
