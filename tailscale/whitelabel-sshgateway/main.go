package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

// Config holds the application configuration
type Config struct {
	Hostname    string
	StateDir    string
	WebUsername string // Custom auth username for web UI
	WebPassword string // Custom auth password for web UI
	ListenPort  string
}

// SSHGateway manages the tsnet server and SSH connections
type SSHGateway struct {
	config     *Config
	tsServer   *tsnet.Server
	upgrader   websocket.Upgrader
	loginURL   string
	authed     bool
	authMutex  sync.RWMutex
}

// NewSSHGateway creates a new SSH gateway instance
func NewSSHGateway(config *Config) *SSHGateway {
	return &SSHGateway{
		config: config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // In production, implement proper CORS
			},
		},
	}
}

// Start initializes the tsnet server and HTTP server
func (sg *SSHGateway) Start(ctx context.Context) error {
	// Initialize tsnet server with interactive login
	sg.tsServer = &tsnet.Server{
		Hostname:  sg.config.Hostname,
		Dir:       sg.config.StateDir,
		Ephemeral: false,
		// No AuthKey - this triggers interactive login with URL
	}

	// Start tsnet server
	log.Printf("Starting tsnet server as '%s'...", sg.config.Hostname)
	log.Println("If not already authenticated, a login URL will be displayed below.")
	log.Println("Visit the URL to authenticate with your Tailscale account.")
	if err := sg.tsServer.Start(); err != nil {
		return fmt.Errorf("failed to start tsnet server: %w", err)
	}

	// Wait for the server to be ready
	log.Println("Waiting for tsnet to be ready...")
	localClient, err := sg.tsServer.LocalClient()
	if err != nil {
		return fmt.Errorf("failed to get local client: %w", err)
	}

	// Monitor authentication status in background
	go sg.monitorAuthStatus(ctx, localClient)

	// Get initial status
	status, err := localClient.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}
	log.Printf("Connected to tailnet! Self: %s", status.Self.DNSName)

	// Create HTTP listener on the tailnet
	listener, err := sg.tsServer.Listen("tcp", ":"+sg.config.ListenPort)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/", sg.handleIndex)                     // No auth - users need to see UI to get login URL
	mux.HandleFunc("/api/auth/status", sg.handleAuthStatus) // No auth - users need this to see login URL
	mux.HandleFunc("/api/auth/logout", sg.handleLogout)     // Logout from Tailscale
	mux.HandleFunc("/api/hosts", sg.handleListHosts)        // No auth - users can see available hosts
	mux.HandleFunc("/ws/ssh", sg.handleWebSocket)           // No auth - WebSocket connection

	server := &http.Server{
		Handler: mux,
	}

	// Also start a localhost listener for pre-auth access
	go func() {
		localListener, err := net.Listen("tcp", "localhost:"+sg.config.ListenPort)
		if err != nil {
			log.Printf("Warning: Could not start localhost listener: %v", err)
			return
		}
		log.Printf("Gateway also available on http://localhost:%s (for authentication)", sg.config.ListenPort)
		if err := server.Serve(localListener); err != nil && err != http.ErrServerClosed {
			log.Printf("Localhost server error: %v", err)
		}
	}()

	log.Printf("SSH Gateway running on https://%s:%s", sg.config.Hostname, sg.config.ListenPort)
	log.Printf("Access http://localhost:%s to authenticate if needed", sg.config.ListenPort)
	
	return server.Serve(listener)
}

// basicAuth implements HTTP Basic Authentication middleware
func (sg *SSHGateway) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="SSH Gateway"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Constant time comparison to prevent timing attacks
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(sg.config.WebUsername)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(sg.config.WebPassword)) == 1

		if !usernameMatch || !passwordMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="SSH Gateway"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// handleIndex serves the web terminal HTML page
func (sg *SSHGateway) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := getWebTerminalHTML()
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleAuthStatus returns the current authentication status and login URL
func (sg *SSHGateway) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	sg.authMutex.RLock()
	loginURL := sg.loginURL
	authed := sg.authed
	sg.authMutex.RUnlock()

	response := map[string]interface{}{
		"authenticated": authed,
		"loginURL":      loginURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleLogout logs out from Tailscale
func (sg *SSHGateway) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	localClient, err := sg.tsServer.LocalClient()
	if err != nil {
		http.Error(w, "Failed to get local client", http.StatusInternalServerError)
		return
	}

	// Logout from Tailscale
	if err := localClient.Logout(ctx); err != nil {
		log.Printf("Logout failed: %v", err)
		http.Error(w, fmt.Sprintf("Logout failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("User logged out from Tailscale")

	// Reset auth state
	sg.authMutex.Lock()
	sg.authed = false
	sg.loginURL = ""
	sg.authMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// monitorAuthStatus monitors the authentication status and captures login URL
func (sg *SSHGateway) monitorAuthStatus(ctx context.Context, client *tailscale.LocalClient) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := client.Status(ctx)
			if err != nil {
				continue
			}

			sg.authMutex.Lock()
			if status.BackendState == "Running" {
				sg.authed = true
				sg.loginURL = ""
			} else if status.BackendState == "NeedsLogin" {
				sg.authed = false
				if status.AuthURL != "" {
					sg.loginURL = status.AuthURL
					log.Printf("Authentication required. Login URL available via web interface: %s", status.AuthURL)
				}
			}
			sg.authMutex.Unlock()
		}
	}
}

// handleListHosts returns available hosts in the tailnet
func (sg *SSHGateway) handleListHosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	localClient, err := sg.tsServer.LocalClient()
	if err != nil {
		http.Error(w, "Failed to get local client", http.StatusInternalServerError)
		return
	}

	status, err := localClient.Status(ctx)
	if err != nil {
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	type Host struct {
		Name string `json:"name"`
		IP   string `json:"ip"`
	}

	var hosts []Host
	for _, peer := range status.Peer {
		// Filter out exit nodes (Mullvad, etc) and only include real machines
		if len(peer.TailscaleIPs) > 0 && 
		   !peer.ExitNode && 
		   !peer.ExitNodeOption &&
		   !strings.Contains(strings.ToLower(peer.DNSName), "mullvad") {
			hosts = append(hosts, Host{
				Name: peer.DNSName,
				IP:   peer.TailscaleIPs[0].String(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hosts)
}

// handleWebSocket handles WebSocket connections for SSH sessions using Tailscale SSH
func (sg *SSHGateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("WebSocket handler called from %s", r.RemoteAddr)
	
	// Get target host from query parameters
	targetHost := r.URL.Query().Get("host")
	targetUser := r.URL.Query().Get("user")
	log.Printf("Request params: host=%s, user=%s", targetHost, targetUser)
	
	if targetHost == "" || targetUser == "" {
		log.Printf("Missing parameters: host=%s, user=%s", targetHost, targetUser)
		http.Error(w, "Missing host or user parameter", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	log.Printf("Attempting WebSocket upgrade...")
	ws, err := sg.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer ws.Close()

	log.Printf("WebSocket upgraded successfully, starting SSH proxy...")
	
	// Use tsnet to proxy the SSH connection through the tailnet
	sg.proxySSHSession(ws, targetHost, targetUser)
	
	log.Printf("WebSocket handler complete")
}

// proxySSHSession creates an interactive SSH session through the tailnet
func (sg *SSHGateway) proxySSHSession(ws *websocket.Conn, targetHost, targetUser string) {
	ctx := context.Background()

	log.Printf("Starting SSH session to %s@%s", targetUser, targetHost)

	// Connect to SSH port on target via tailnet
	target := net.JoinHostPort(targetHost, "22")
	log.Printf("Dialing %s...", target)

	conn, err := sg.tsServer.Dial(ctx, "tcp", target)
	if err != nil {
		log.Printf("Failed to dial %s: %v", target, err)
		errMsg := fmt.Sprintf("\r\n\x1b[31mConnection failed: %v\x1b[0m\r\n", err)
		errMsg += "\x1b[33mMake sure the target is online and reachable.\x1b[0m\r\n"
		ws.WriteMessage(websocket.TextMessage, []byte(errMsg))
		return
	}
	defer conn.Close()

	log.Printf("TCP connection established to %s", target)

	// Setup SSH client config - try passwordless first (Tailscale SSH)
	// Then fall back to keyboard-interactive
	sshConfig := &ssh.ClientConfig{
		User: targetUser,
		Auth: []ssh.AuthMethod{
			// Try empty auth first for Tailscale SSH
			ssh.Password(""),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				// For Tailscale SSH, this should succeed without prompts
				log.Printf("Keyboard interactive: user=%s, instruction=%s, questions=%d", user, instruction, len(questions))
				return make([]string, len(questions)), nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	log.Printf("Attempting SSH handshake with %s as %s...", targetHost, targetUser)

	// Establish SSH connection
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, targetHost, sshConfig)
	if err != nil {
		log.Printf("SSH handshake failed: %v (type: %T)", err, err)
		errMsg := fmt.Sprintf("\r\n\x1b[31mSSH connection failed: %v\x1b[0m\r\n", err)
		errMsg += "\x1b[33mMake sure Tailscale SSH is enabled on target: tailscale up --ssh\x1b[0m\r\n"
		errMsg += fmt.Sprintf("\x1b[33mOr check ACLs at https://login.tailscale.com/admin/acls\x1b[0m\r\n")
		ws.WriteMessage(websocket.TextMessage, []byte(errMsg))
		return
	}
	defer sshConn.Close()

	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	log.Printf("SSH client connected to %s", targetHost)

	// Create SSH session
	session, err := client.NewSession()
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mSession error: %v\x1b[0m\r\n", err)))
		return
	}
	defer session.Close()

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 40, 80, modes); err != nil {
		log.Printf("PTY request failed: %v", err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mPTY error: %v\x1b[0m\r\n", err)))
		return
	}

	// Get session I/O
	sessionStdin, _ := session.StdinPipe()
	sessionStdout, _ := session.StdoutPipe()
	sessionStderr, _ := session.StderrPipe()

	// Start shell
	if err := session.Shell(); err != nil {
		log.Printf("Failed to start shell: %v", err)
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mShell error: %v\x1b[0m\r\n", err)))
		return
	}

	log.Printf("Shell started successfully for %s@%s", targetUser, targetHost)

	var wg sync.WaitGroup
	wg.Add(3)

	// WebSocket -> SSH stdin
	go func() {
		defer wg.Done()
		defer sessionStdin.Close()
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			var msg struct {
				Type string `json:"type"`
				Data string `json:"data"`
				Rows int    `json:"rows"`
				Cols int    `json:"cols"`
			}

			if err := json.Unmarshal(message, &msg); err != nil {
				sessionStdin.Write(message)
				continue
			}

			switch msg.Type {
			case "input":
				sessionStdin.Write([]byte(msg.Data))
			case "resize":
				session.WindowChange(msg.Rows, msg.Cols)
			}
		}
	}()

	// SSH stdout -> WebSocket
	go func() {
		defer wg.Done()
		io.Copy(&wsWriter{ws: ws}, sessionStdout)
	}()

	// SSH stderr -> WebSocket
	go func() {
		defer wg.Done()
		io.Copy(&wsWriter{ws: ws}, sessionStderr)
	}()

	// Wait for session to complete
	session.Wait()
	wg.Wait()

	log.Printf("SSH session ended for %s@%s", targetUser, targetHost)
}

// wsWriter adapts WebSocket to io.Writer
type wsWriter struct {
	ws *websocket.Conn
}

func (w *wsWriter) Write(p []byte) (n int, err error) {
	err = w.ws.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func main() {
	config := &Config{
		Hostname:    getEnv("HOSTNAME", "ssh-gateway"),
		StateDir:    getEnv("STATE_DIR", "./tsnet-state"),
		WebUsername: getEnv("WEB_USERNAME", "admin"),
		WebPassword: getEnv("WEB_PASSWORD", "changeme"),
		ListenPort:  getEnv("LISTEN_PORT", "8080"),
	}

	gateway := NewSSHGateway(config)
	
	ctx := context.Background()
	if err := gateway.Start(ctx); err != nil {
		log.Fatalf("Gateway failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
