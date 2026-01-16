# Tailscale Route Monitor

A simple tool that monitors your local Tailscale daemon for route advertisement changes in real time.

## What it does

- Connects to your local Tailscale daemon via the IPN bus
- Watches for changes to advertised routes on your node
- Shows which nodes in your network are advertising routes
- Displays route changes as they happen

## Usage

Run the monitor:
```bash
go run .
```

With debug output to see all notifications:
```bash
go run . --debug
```

Use a custom socket path:
```bash
go run . --socket /path/to/tailscaled.sock
```

## Output

The tool will show:
- Local route configuration changes when you run `tailscale set --advertise-routes`
- Network-wide view of all nodes advertising routes
- Real-time updates as routes are added or removed

Press Ctrl+C to stop monitoring.