package main

func getWebTerminalHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SSH Gateway</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.min.css" />
    <script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.min.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #2E3440 0%, #3B4252 100%);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }
        
        .header {
            background: rgba(255, 255, 255, 0.95);
            padding: 1rem 2rem;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .header h1 {
            color: #5E81AC;
            font-size: 1.5rem;
            font-weight: 600;
            margin: 0;
        }
        
        .container {
            flex: 1;
            display: flex;
            flex-direction: column;
            padding: 2rem;
            gap: 1rem;
            max-width: 1400px;
            margin: 0 auto;
            width: 100%;
        }
        
        .connection-panel {
            background: rgba(255, 255, 255, 0.95);
            padding: 1.5rem;
            border-radius: 12px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        
        .form-group {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 1rem;
        }
        
        .input-wrapper {
            display: flex;
            flex-direction: column;
            gap: 0.5rem;
        }
        
        label {
            font-weight: 500;
            color: #4a5568;
            font-size: 0.9rem;
        }
        
        input, select {
            padding: 0.75rem;
            border: 2px solid #e2e8f0;
            border-radius: 8px;
            font-size: 1rem;
            transition: border-color 0.2s;
        }
        
        input:focus, select:focus {
            outline: none;
            border-color: #5E81AC;
        }
        
        button {
            padding: 0.75rem 2rem;
            background: #5E81AC;
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 1rem;
            font-weight: 500;
            cursor: pointer;
            transition: background 0.2s, transform 0.1s;
        }
        
        button:hover {
            background: #4C6A8F;
        }
        
        button:active {
            transform: scale(0.98);
        }
        
        button:disabled {
            background: #cbd5e0;
            cursor: not-allowed;
        }
        
        .terminal-container {
            flex: 1;
            background: #1e1e1e;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 8px 16px rgba(0,0,0,0.2);
            display: flex;
            flex-direction: column;
            min-height: 500px;
        }
        
        .terminal-header {
            background: #2d2d2d;
            padding: 0.75rem 1rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        
        .terminal-dot {
            width: 12px;
            height: 12px;
            border-radius: 50%;
        }
        
        .dot-close { background: #ff5f56; }
        .dot-minimize { background: #ffbd2e; }
        .dot-maximize { background: #27c93f; }
        
        .terminal-wrapper {
            flex: 1;
            padding: 1rem;
        }
        
        #terminal {
            height: 100%;
            width: 100%;
        }
        
        .status {
            padding: 0.5rem 1rem;
            background: #2d2d2d;
            color: #a0aec0;
            font-size: 0.85rem;
            text-align: center;
        }
        
        .status.connected {
            background: #27c93f;
            color: white;
        }
        
        .status.error {
            background: #ff5f56;
            color: white;
        }
        
        .auth-banner {
            background: linear-gradient(135deg, #88C0D0 0%, #5E81AC 100%);
            color: white;
            padding: 1.5rem;
            border-radius: 12px;
            margin-bottom: 1rem;
            display: none;
        }
        
        .auth-banner.show {
            display: block;
        }
        
        .auth-banner h2 {
            margin: 0 0 1rem 0;
            font-size: 1.25rem;
        }
        
        .auth-banner p {
            margin: 0.5rem 0;
        }
        
        .login-button {
            display: inline-block;
            margin-top: 1rem;
            padding: 0.75rem 2rem;
            background: white;
            color: #5E81AC;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 600;
            transition: transform 0.2s;
        }
        
        .login-button:hover {
            transform: scale(1.05);
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 1rem;
            }
            
            .form-group {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🔒 Secure SSH Gateway</h1>
        <button class="logout-btn" id="logoutBtn" onclick="logout()" style="display: none;">Logout from Tailscale</button>
    </div>
    
    <div class="container">
        <div class="auth-banner" id="authBanner">
            <h2>🔐 Tailscale Authentication Required</h2>
            <p>This gateway needs to authenticate with your Tailscale account.</p>
            <p>Click the button below to log in:</p>
            <a href="#" id="loginLink" class="login-button" target="_blank">Login to Tailscale</a>
            <p style="margin-top: 1rem; font-size: 0.9rem;">After logging in, this page will automatically refresh.</p>
        </div>
        
        <div class="connection-panel">
            <div class="form-group">
                <div class="input-wrapper">
                    <label for="host">Target Host</label>
                    <div style="display: flex; gap: 0.5rem;">
                        <select id="host" style="flex: 1;">
                            <option value="">Select a host...</option>
                        </select>
                        <button onclick="loadHosts()" style="padding: 0.75rem 1rem; background: #4C566A;" title="Refresh host list">↻</button>
                    </div>
                </div>
                <div class="input-wrapper">
                    <label for="custom-host">Or Custom Host/IP</label>
                    <input type="text" id="custom-host" placeholder="hostname or IP">
                </div>
                <div class="input-wrapper">
                    <label for="username">Username</label>
                    <input type="text" id="username" value="root" placeholder="SSH username">
                </div>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button id="connectBtn" onclick="connect()">Connect</button>
                <button id="disconnectBtn" onclick="disconnect()" style="display:none; background: #BF616A;">Disconnect</button>
                <button id="clearBtn" onclick="clearTerminal()" style="background: #4C566A;">Clear Terminal</button>
            </div>
        </div>
        
        <div class="terminal-container">
            <div class="terminal-header">
                <div class="terminal-dot dot-close"></div>
                <div class="terminal-dot dot-minimize"></div>
                <div class="terminal-dot dot-maximize"></div>
            </div>
            <div class="terminal-wrapper">
                <div id="terminal"></div>
            </div>
            <div class="status" id="status">Not connected</div>
        </div>
    </div>

    <script>
        // Initialize xterm.js terminal
        const term = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
                background: '#1e1e1e',
                foreground: '#ffffff',
                cursor: '#ffffff',
                black: '#000000',
                red: '#e06c75',
                green: '#98c379',
                yellow: '#d19a66',
                blue: '#61afef',
                magenta: '#c678dd',
                cyan: '#56b6c2',
                white: '#abb2bf',
                brightBlack: '#5c6370',
                brightRed: '#e06c75',
                brightGreen: '#98c379',
                brightYellow: '#d19a66',
                brightBlue: '#61afef',
                brightMagenta: '#c678dd',
                brightCyan: '#56b6c2',
                brightWhite: '#ffffff'
            }
        });

        const fitAddon = new FitAddon.FitAddon();
        term.loadAddon(fitAddon);
        term.open(document.getElementById('terminal'));
        fitAddon.fit();

        // Handle window resize
        window.addEventListener('resize', () => {
            fitAddon.fit();
            if (ws && ws.readyState === WebSocket.OPEN) {
                sendResize();
            }
        });

        let ws = null;

        // Check authentication status
        async function checkAuthStatus() {
            try {
                const response = await fetch('/api/auth/status');
                const data = await response.json();
                
                if (!data.authenticated && data.loginURL) {
                    // Show auth banner with login URL
                    document.getElementById('authBanner').classList.add('show');
                    document.getElementById('loginLink').href = data.loginURL;
                    return false;
                } else if (data.authenticated) {
                    // Hide auth banner
                    document.getElementById('authBanner').classList.remove('show');
                    return true;
                }
            } catch (error) {
                console.error('Failed to check auth status:', error);
            }
            return false;
        }

        // Load available hosts
        async function loadHosts() {
            try {
                console.log('Loading hosts...');
                const response = await fetch('/api/hosts');
                console.log('Hosts response status:', response.status);
                const hosts = await response.json();
                console.log('Loaded hosts:', hosts);
                
                const select = document.getElementById('host');
                const currentValue = select.value; // Preserve current selection
                
                select.innerHTML = '<option value="">Select a host...</option>';
                hosts.forEach(host => {
                    const option = document.createElement('option');
                    option.value = host.ip;
                    option.textContent = host.name || host.ip;
                    select.appendChild(option);
                });
                
                // Restore selection if it still exists
                if (currentValue && Array.from(select.options).some(opt => opt.value === currentValue)) {
                    select.value = currentValue;
                }
                
                console.log('Dropdown populated with', hosts.length, 'hosts');
            } catch (error) {
                console.error('Failed to load hosts:', error);
                term.writeln('\r\n\x1b[31mFailed to load available hosts\x1b[0m');
            }
        }

        function setStatus(message, type = 'info') {
            const statusEl = document.getElementById('status');
            statusEl.textContent = message;
            statusEl.className = 'status ' + type;
        }

        function sendResize() {
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                    type: 'resize',
                    rows: term.rows,
                    cols: term.cols
                }));
            }
        }

        async function connect() {
            const hostSelect = document.getElementById('host').value;
            const customHost = document.getElementById('custom-host').value;
            const host = customHost || hostSelect;
            const username = document.getElementById('username').value;

            if (!host || !username) {
                alert('Please provide both host and username');
                return;
            }

            term.clear();
            setStatus('Connecting...', 'info');
            
            // Create WebSocket connection
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws/ssh?host=' + 
                          encodeURIComponent(host) + '&user=' + encodeURIComponent(username);
            
            ws = new WebSocket(wsUrl);

            ws.onopen = () => {
                setStatus('Connected to ' + host, 'connected');
                document.getElementById('connectBtn').style.display = 'none';
                document.getElementById('disconnectBtn').style.display = 'inline-block';
                term.focus();
                
                // Send initial resize
                setTimeout(() => sendResize(), 100);
            };

            ws.onmessage = (event) => {
                // Handle binary data (SSH output)
                if (event.data instanceof Blob) {
                    event.data.arrayBuffer().then(buffer => {
                        const uint8Array = new Uint8Array(buffer);
                        term.write(uint8Array);
                    });
                } else {
                    // Handle text data
                    term.write(event.data);
                }
            };

            ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                setStatus('Connection error', 'error');
            };

            ws.onclose = () => {
                setStatus('Disconnected', 'error');
                document.getElementById('connectBtn').style.display = 'inline-block';
                document.getElementById('disconnectBtn').style.display = 'none';
                term.writeln('\r\n\x1b[33m[Connection closed]\x1b[0m');
            };

            // Handle terminal input
            term.onData((data) => {
                if (ws && ws.readyState === WebSocket.OPEN) {
                    ws.send(JSON.stringify({
                        type: 'input',
                        data: data
                    }));
                }
            });
        }

        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
        }
        
        function clearTerminal() {
            term.clear();
            term.writeln('\x1b[1;36m╔═══════════════════════════════════════════════════════════╗\x1b[0m');
            term.writeln('\x1b[1;36m║                  Secure SSH Gateway                       ║\x1b[0m');
            term.writeln('\x1b[1;36m║              Powered by Tailscale SSH                     ║\x1b[0m');
            term.writeln('\x1b[1;36m╚═══════════════════════════════════════════════════════════╝\x1b[0m');
            term.writeln('');
            term.writeln('\x1b[32m✓\x1b[0m Authentication via Tailscale - no passwords needed!');
            term.writeln('\x1b[32m✓\x1b[0m Secure end-to-end encrypted connections');
            term.writeln('');
            term.writeln('Select a host from your tailnet and connect.');
            term.writeln('');
        }

        // Load hosts on page load
        let hostsLoaded = false;
        checkAuthStatus();
        loadHosts();
        
        // Poll auth status every 3 seconds
        setInterval(async () => {
            const authed = await checkAuthStatus();
            // Only load hosts once after successful auth
            if (authed && !hostsLoaded) {
                hostsLoaded = true;
                loadHosts();
            }
        }, 3000);

        // Welcome message
        term.writeln('\x1b[1;36m╔═══════════════════════════════════════════════════════════╗\x1b[0m');
        term.writeln('\x1b[1;36m║                  Secure SSH Gateway                       ║\x1b[0m');
        term.writeln('\x1b[1;36m║              Powered by Tailscale SSH                     ║\x1b[0m');
        term.writeln('\x1b[1;36m╚═══════════════════════════════════════════════════════════╝\x1b[0m');
        term.writeln('');
        term.writeln('\x1b[32m✓\x1b[0m Authentication via Tailscale - no passwords needed!');
        term.writeln('\x1b[32m✓\x1b[0m Secure end-to-end encrypted connections');
        term.writeln('');
        term.writeln('Select a host from your tailnet and connect.');
        term.writeln('');
    </script>
</body>
</html>
`
}