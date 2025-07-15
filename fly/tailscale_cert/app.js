const express = require('express');
const { execSync } = require('child_process');
const http = require('http');
const https = require('https');
const { SocksProxyAgent } = require('socks-proxy-agent');
const app = express();
const port = process.env.PORT || 8080;

// Middleware
app.use(express.json());
app.use(express.static('public'));

// Helper function to get Tailscale status
function getTailscaleStatus() {
    try {
        const status = execSync('/app/tailscale status --json', { encoding: 'utf8' });
        return JSON.parse(status);
    } catch (error) {
        console.error('Error getting Tailscale status:', error.message);
        return null;
    }
}

// Helper function to test network connectivity via SOCKS5 proxy
async function testTailscaleConnectivity(targetHost, targetPort = 22, timeout = 5000) {
    return new Promise((resolve) => {
        try {
            const net = require('net');
            const { SocksClient } = require('socks');
            
            const options = {
                proxy: {
                    host: 'localhost',
                    port: 1337,
                    type: 5
                },
                command: 'connect',
                destination: {
                    host: targetHost,
                    port: targetPort
                }
            };

            SocksClient.createConnection(options, (err, info) => {
                if (err) {
                    resolve({
                        success: false,
                        error: err.message,
                        code: err.code || 'SOCKS_ERROR',
                        timestamp: new Date().toISOString()
                    });
                    return;
                }

                // Connection successful - close it immediately
                info.socket.destroy();
                resolve({
                    success: true,
                    message: 'Connection established successfully',
                    timestamp: new Date().toISOString()
                });
            });

            // Set timeout
            setTimeout(() => {
                resolve({
                    success: false,
                    error: 'Connection timeout',
                    code: 'TIMEOUT',
                    timestamp: new Date().toISOString()
                });
            }, timeout);

        } catch (error) {
            resolve({
                success: false,
                error: error.message,
                code: 'PROXY_ERROR',
                timestamp: new Date().toISOString()
            });
        }
    });
}

// Helper function to test multiple common ports on a device
async function testMultiplePorts(targetHost, timeout = 3000) {
    const commonPorts = [22, 80, 443, 8080, 3389, 5900]; // SSH, HTTP, HTTPS, Alt-HTTP, RDP, VNC
    const results = [];

    for (const port of commonPorts) {
        const result = await testTailscaleConnectivity(targetHost, port, timeout);
        results.push({
            port,
            ...result
        });
        
        // If we find one working port, that's enough to prove connectivity
        if (result.success) {
            break;
        }
    }

    return results;
}

// Helper function to ping a Tailscale device
async function pingTailscaleDevice(deviceName) {
    return new Promise((resolve) => {
        try {
            const result = execSync(`/app/tailscale ping --until-direct=false ${deviceName}`, { 
                encoding: 'utf8', 
                timeout: 10000 
            });
            resolve({
                success: true,
                output: result.trim(),
                timestamp: new Date().toISOString()
            });
        } catch (error) {
            resolve({
                success: false,
                error: error.message,
                output: error.stdout ? error.stdout.trim() : '',
                timestamp: new Date().toISOString()
            });
        }
    });
}

// Helper function to get all Tailscale peers
function getTailscalePeers() {
    try {
        const status = getTailscaleStatus();
        if (!status || !status.Peer) {
            return [];
        }
        
        return Object.entries(status.Peer).map(([id, peer]) => ({
            id,
            dnsName: peer.DNSName,
            hostName: peer.HostName,
            tailscaleIPs: peer.TailscaleIPs || [],
            online: peer.Online,
            lastSeen: peer.LastSeen,
            os: peer.OS
        }));
    } catch (error) {
        console.error('Error getting Tailscale peers:', error.message);
        return [];
    }
}

// Routes
app.get('/', (req, res) => {
    res.json({
        name: 'Tailscale + Fly.io API',
        version: '1.0.0',
        description: 'Simple API for testing Tailscale connectivity on Fly.io',
        endpoints: {
            status: '/api/status',
            health: '/api/health',
            tailscale: {
                status: '/api/tailscale',
                peers: '/api/tailscale/peers',
                test: '/api/tailscale/test',
                ping: '/api/tailscale/ping/:host',
                'ping-test': '/api/tailscale/ping-test'
            },
            echo: '/api/echo'
        },
        timestamp: new Date().toISOString()
    });
});

// API Routes
app.get('/api/status', (req, res) => {
    res.json({
        status: 'ok',
        timestamp: new Date().toISOString(),
        uptime: process.uptime(),
        memory: process.memoryUsage(),
        pid: process.pid,
        platform: process.platform,
        nodeVersion: process.version,
        environment: process.env.NODE_ENV || 'development'
    });
});

app.get('/api/health', (req, res) => {
    res.json({
        status: 'healthy',
        timestamp: new Date().toISOString(),
        service: 'tailscale-fly-app'
    });
});

app.get('/api/tailscale', (req, res) => {
    const status = getTailscaleStatus();
    if (status) {
        res.json({
            status: 'connected',
            tailscale: {
                self: status.Self,
                peers: Object.keys(status.Peer || {}).length,
                magicDNSSuffix: status.MagicDNSSuffix,
                currentTailnet: status.CurrentTailnet
            },
            timestamp: new Date().toISOString()
        });
    } else {
        res.status(500).json({
            status: 'error',
            message: 'Unable to retrieve Tailscale status',
            timestamp: new Date().toISOString()
        });
    }
});

// Get Tailscale peers
app.get('/api/tailscale/peers', (req, res) => {
    const peers = getTailscalePeers();
    res.json({
        peers,
        count: peers.length,
        timestamp: new Date().toISOString()
    });
});

// Test Tailscale connectivity
app.get('/api/tailscale/test', async (req, res) => {
    const peers = getTailscalePeers();
    const results = {
        tailscaleStatus: 'unknown',
        connectivityTests: [],
        timestamp: new Date().toISOString()
    };

    // Check basic Tailscale status
    const status = getTailscaleStatus();
    results.tailscaleStatus = status ? 'connected' : 'disconnected';

    // Test connectivity to online peers using ping (more reliable than SOCKS5)
    const onlinePeers = peers.filter(peer => peer.online).slice(0, 3); // Test up to 3 peers
    
    for (const peer of onlinePeers) {
        if (peer.tailscaleIPs && peer.tailscaleIPs.length > 0) {
            const pingResult = await pingTailscaleDevice(peer.tailscaleIPs[0]);
            results.connectivityTests.push({
                peer: {
                    hostname: peer.hostName,
                    dnsName: peer.dnsName,
                    ip: peer.tailscaleIPs[0]
                },
                method: 'ping',
                ...pingResult
            });
        }
    }

    res.json(results);
});

// Test connectivity to a specific device
app.post('/api/tailscale/test/:device', async (req, res) => {
    const { device } = req.params;
    const { port = 80, method = 'http' } = req.body;

    if (method === 'ping') {
        const result = await pingTailscaleDevice(device);
        res.json(result);
    } else {
        const result = await testTailscaleConnectivity(device, parseInt(port), 10000);
        res.json({
            device,
            port: parseInt(port),
            method,
            ...result
        });
    }
});

// ICMP ping test for a specific host
app.post('/api/tailscale/ping/:host', async (req, res) => {
    const { host } = req.params;
    const { count = 4, timeout = 10 } = req.body;
    
    try {
        // Use Tailscale's ping command which works through the VPN
        // Note: Tailscale ping doesn't support -w flag, so we use process timeout instead
        const result = execSync(`/app/tailscale ping --until-direct=false -c ${count} ${host}`, { 
            encoding: 'utf8', 
            timeout: timeout * 1000 // Convert to milliseconds
        });
        
        // Parse the ping output for useful information
        const lines = result.trim().split('\n');
        const pingResults = {
            success: true,
            host: host,
            packets: {
                transmitted: 0,
                received: 0,
                loss: '0%'
            },
            timing: {
                min: null,
                avg: null,
                max: null,
                unit: 'ms'
            },
            rawOutput: result.trim(),
            timestamp: new Date().toISOString()
        };
        
        // Parse ping statistics
        for (const line of lines) {
            if (line.includes('packets transmitted')) {
                const match = line.match(/(\d+) packets transmitted, (\d+) (?:packets )?received, ([0-9.]+)% packet loss/);
                if (match) {
                    pingResults.packets.transmitted = parseInt(match[1]);
                    pingResults.packets.received = parseInt(match[2]);
                    pingResults.packets.loss = match[3] + '%';
                }
            }
            if (line.includes('min/avg/max')) {
                const match = line.match(/min\/avg\/max\/[a-z]+ = ([0-9.]+)\/([0-9.]+)\/([0-9.]+)\/[0-9.]+ ms/);
                if (match) {
                    pingResults.timing.min = parseFloat(match[1]);
                    pingResults.timing.avg = parseFloat(match[2]);
                    pingResults.timing.max = parseFloat(match[3]);
                }
            }
        }
        
        res.json(pingResults);
        
    } catch (error) {
        res.json({
            success: false,
            host: host,
            error: error.message,
            output: error.stdout ? error.stdout.trim() : '',
            timestamp: new Date().toISOString()
        });
    }
});

// Simplified ICMP test for multiple hosts
app.post('/api/tailscale/ping-test', async (req, res) => {
    const { hosts = [], count = 3 } = req.body;
    
    if (!Array.isArray(hosts) || hosts.length === 0) {
        return res.status(400).json({
            error: 'Invalid request',
            message: 'Please provide an array of hosts to ping',
            timestamp: new Date().toISOString()
        });
    }
    
    const results = [];
    
    for (const host of hosts.slice(0, 5)) { // Limit to 5 hosts max
        try {
            const result = execSync(`/app/tailscale ping --until-direct=false -c ${count} ${host}`, { 
                encoding: 'utf8', 
                timeout: 8000
            });
            
            // Simple success check
            const isSuccess = !result.includes('100% packet loss') && !result.includes('no route');
            
            results.push({
                host: host,
                success: isSuccess,
                summary: result.split('\n').pop().trim(), // Last line usually has summary
                timestamp: new Date().toISOString()
            });
            
        } catch (error) {
            results.push({
                host: host,
                success: false,
                error: error.message,
                timestamp: new Date().toISOString()
            });
        }
    }
    
    res.json({
        results,
        totalHosts: results.length,
        successfulPings: results.filter(r => r.success).length,
        timestamp: new Date().toISOString()
    });
});

// Simple echo endpoint for testing
app.post('/api/echo', (req, res) => {
    res.json({
        echo: req.body,
        timestamp: new Date().toISOString(),
        headers: req.headers
    });
});

// 404 handler
app.use((req, res) => {
    res.status(404).json({
        error: 'Not Found',
        message: `Route ${req.method} ${req.path} not found`,
        timestamp: new Date().toISOString()
    });
});

// Error handler
app.use((err, req, res, next) => {
    console.error('Error:', err);
    res.status(500).json({
        error: 'Internal Server Error',
        message: err.message,
        timestamp: new Date().toISOString()
    });
});

// Start server
app.listen(port, '0.0.0.0', () => {
    console.log(`🚀 Server running on port ${port}`);
    console.log(`📅 Started at: ${new Date().toISOString()}`);
    console.log(`🔗 Environment: ${process.env.NODE_ENV || 'development'}`);
    
    // Log Tailscale status on startup
    const tailscaleStatus = getTailscaleStatus();
    if (tailscaleStatus) {
        console.log(`🔒 Tailscale connected as: ${tailscaleStatus.Self?.DNSName || 'unknown'}`);
        const peers = getTailscalePeers();
        console.log(`👥 Connected to ${peers.length} peers`);
        console.log(`🌐 Tailnet: ${tailscaleStatus.CurrentTailnet?.Name || 'unknown'}`);
        console.log(`✅ API ready for testing`);
    } else {
        console.log('⚠️  Tailscale status unavailable');
    }
});

// Graceful shutdown
process.on('SIGTERM', () => {
    console.log('📴 Received SIGTERM, shutting down gracefully');
    process.exit(0);
});

process.on('SIGINT', () => {
    console.log('📴 Received SIGINT, shutting down gracefully');
    process.exit(0);
});
