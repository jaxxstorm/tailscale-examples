package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
)

// IPNEventMsg represents an IPN bus event
type IPNEventMsg struct {
	Type    string
	Message string
}

// peerProperties tracks observable properties of a peer
type peerProperties struct {
	Online         bool
	OffersExitNode bool
	SSHEnabled     bool
	HostinfoJSON   string
}

// Model for bubbletea
type model struct {
	spinner spinner.Model
	status  string
	events  []string
	debug   bool
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case IPNEventMsg:
		timestamp := timestampStyle.Render(time.Now().Format("15:04:05"))

		if msg.Type == "error" {
			eventStr := fmt.Sprintf("%s %s",
				timestamp,
				errorStyle.Render(fmt.Sprintf("❌ %s", msg.Message)))
			m.events = append(m.events, eventStr)
		} else if msg.Type == "connected" {
			m.status = "Watching IPN bus events"
			eventStr := fmt.Sprintf("%s %s",
				timestamp,
				connectedStyle.Render("✅ Connected to Tailscale daemon"))
			m.events = append(m.events, eventStr)
		} else {
			// Generic event message
			eventStr := fmt.Sprintf("%s %s",
				timestamp,
				statusStyle.Render(msg.Message))
			m.events = append(m.events, eventStr)
		}

		// Keep only last 50 events to prevent memory issues
		if len(m.events) > 50 {
			m.events = m.events[len(m.events)-50:]
		}
	}
	return m, nil
}

func (m model) View() string {
	header := headerStyle.Render(fmt.Sprintf("%s %s", m.spinner.View(), m.status))

	var eventLines []string
	for _, event := range m.events {
		eventLines = append(eventLines, event)
	}

	body := strings.Join(eventLines, "\n")
	footer := footerStyle.Render("Press 'q' or 'ctrl+c' to quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
}

var (
	// Styles
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7B68EE")).MarginBottom(1)
	connectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#32CD32"))
	routeChangeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
	nodeStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
	routeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#98FB98"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	statusStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#DDD"))
	timestampStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#666")).MarginTop(1)
	debugStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Faint(true)
)

func main() {
	var socket string
	var pretty bool
	var debug bool

	flag.StringVar(&socket, "socket", "", "override tailscaled LocalAPI socket/path (leave empty for platform default)")
	flag.BoolVar(&pretty, "pretty", true, "pretty-print JSON")
	flag.BoolVar(&debug, "debug", false, "enable debug output including full netmap")
	flag.Parse()

	// Context lifetime is the watch lifetime. Cancel it to stop the stream.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl-C / SIGTERM gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// In debug mode, skip bubbletea and just output JSON
	if debug {
		go func() {
			<-sigCh
			cancel()
		}()
		
		monitorRoutes(ctx, socket, debug, nil)
		return
	}

	// Setup bubbletea for normal mode
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{
		spinner: s,
		status:  "Connecting to Tailscale daemon...",
		debug:   debug,
	}

	// Start the bubbletea program
	p := tea.NewProgram(m)

	// Start route monitoring in a goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.Send(IPNEventMsg{
					Type:    "error",
					Message: fmt.Sprintf("Monitoring crashed: %v", r),
				})
			}
		}()

		monitorRoutes(ctx, socket, debug, p)
	}()

	// Handle signals
	go func() {
		<-sigCh
		cancel()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}

func monitorRoutes(ctx context.Context, socket string, debug bool, p *tea.Program) {
	lc := &local.Client{
		Socket: socket,
	}

	// Pick a mask that gives you useful initial state + ongoing engine updates.
	mask := ipn.NotifyInitialState |
		ipn.NotifyInitialPrefs |
		ipn.NotifyInitialNetMap |
		ipn.NotifyWatchEngineUpdates

	w, err := lc.WatchIPNBus(ctx, mask)
	if err != nil {
		p.Send(IPNEventMsg{
			Type:    "error",
			Message: fmt.Sprintf("Failed to connect to Tailscale daemon: %v", err),
		})
		return
	}
	defer func() {
		_ = w.Close()
	}()

	var connected bool
	var lastState string
	var lastAdvertisedRoutes []string
	var lastExitNode string
	var lastOfferExitNode bool
	var lastRunSSH bool
	var lastShieldsUp bool
	var lastNetMapPeerCount int
	var lastNetMapRoutes map[string][]string
	lastNetMapPeerProps := make(map[string]peerProperties)

	for {
		n, err := w.Next()
		if err != nil {
			// When ctx is cancelled, Next returns that error.
			if ctx.Err() != nil {
				return
			}
			p.Send(IPNEventMsg{
				Type:    "error",
				Message: fmt.Sprintf("Error reading from IPN bus: %v", err),
			})
			return
		}

		// Debug mode: output JSON to stdout
		if debug {
			if b, err := json.MarshalIndent(n, "", "  "); err == nil {
				fmt.Println(string(b))
			}
			continue
		}

		// Send notification for first connection
		if !connected {
			connected = true
			p.Send(IPNEventMsg{
				Type:    "connected",
				Message: "Connected to Tailscale daemon",
			})
		}

		// Handle errors
		if n.ErrMessage != nil {
			p.Send(IPNEventMsg{
				Type:    "error",
				Message: fmt.Sprintf("Error: %s", *n.ErrMessage),
			})
			continue
		}

		// Check for state changes
		if n.State != nil {
			newState := n.State.String()
			if newState != lastState {
				p.Send(IPNEventMsg{
					Type:    "event",
					Message: fmt.Sprintf("📨 State changed: %s", newState),
				})
				lastState = newState
			}
		}

		// Check for preference changes
		if n.Prefs != nil {
			var changes []string

			// Check advertised routes
			var currentRoutes []string
			advertiseRoutes := n.Prefs.AdvertiseRoutes()
			for i := 0; i < advertiseRoutes.Len(); i++ {
				currentRoutes = append(currentRoutes, advertiseRoutes.At(i).String())
			}
			if !stringSliceEqual(currentRoutes, lastAdvertisedRoutes) {
				changes = append(changes, fmt.Sprintf("AdvertisedRoutes: %v -> %v", lastAdvertisedRoutes, currentRoutes))
				lastAdvertisedRoutes = currentRoutes
			}

			// Check exit node selection
			currentExitNode := ""
			if n.Prefs.ExitNodeID().IsZero() == false {
				currentExitNode = fmt.Sprintf("%v", n.Prefs.ExitNodeID())
			}
			if currentExitNode != lastExitNode {
				changes = append(changes, fmt.Sprintf("ExitNode: %s -> %s", lastExitNode, currentExitNode))
				lastExitNode = currentExitNode
			}

			// Check if offering as exit node
			currentOfferExitNode := n.Prefs.AdvertisesExitNode()
			if currentOfferExitNode != lastOfferExitNode {
				changes = append(changes, fmt.Sprintf("OfferExitNode: %v -> %v", lastOfferExitNode, currentOfferExitNode))
				lastOfferExitNode = currentOfferExitNode
			}

			// Check SSH
			currentRunSSH := n.Prefs.RunSSH()
			if currentRunSSH != lastRunSSH {
				changes = append(changes, fmt.Sprintf("RunSSH: %v -> %v", lastRunSSH, currentRunSSH))
				lastRunSSH = currentRunSSH
			}

			// Check shields up
			currentShieldsUp := n.Prefs.ShieldsUp()
			if currentShieldsUp != lastShieldsUp {
				changes = append(changes, fmt.Sprintf("ShieldsUp: %v -> %v", lastShieldsUp, currentShieldsUp))
				lastShieldsUp = currentShieldsUp
			}

			if len(changes) > 0 {
				p.Send(IPNEventMsg{
					Type:    "event",
					Message: fmt.Sprintf("📨 Prefs changed: %s", strings.Join(changes, ", ")),
				})
			}
		}

		// Check for NetMap changes
		if n.NetMap != nil {
			var changes []string

			// Check peer count
			currentPeerCount := len(n.NetMap.Peers)
			if currentPeerCount != lastNetMapPeerCount {
				changes = append(changes, fmt.Sprintf("Peers: %d -> %d", lastNetMapPeerCount, currentPeerCount))
				lastNetMapPeerCount = currentPeerCount
			}

			// Check routes and properties from peers
			currentRoutes := make(map[string][]string)
			currentProps := make(map[string]peerProperties)
			for _, peer := range n.NetMap.Peers {
				if !peer.Valid() {
					continue
				}
				nodeName := peer.ComputedName()

				// Collect routes
				var routes []string
				primaryRoutes := peer.PrimaryRoutes()
				for i := 0; i < primaryRoutes.Len(); i++ {
					routes = append(routes, primaryRoutes.At(i).String())
				}
				if len(routes) > 0 {
					currentRoutes[nodeName] = routes
				}

				// Collect peer properties
				props := peerProperties{
					Online: peer.Online().Valid() && peer.Online().Get(),
				}

				// Check if peer offers exit node (advertises 0.0.0.0/0 or ::/0)
				for _, route := range routes {
					if route == "0.0.0.0/0" || route == "::/0" {
						props.OffersExitNode = true
						break
					}
				}

				// Check SSH by looking at hostinfo
				if peer.Hostinfo().Valid() {
					hi := peer.Hostinfo()
					// SSH is enabled if there are services with TCP port 22
					if hi.Services().Len() > 0 {
						for i := 0; i < hi.Services().Len(); i++ {
							svc := hi.Services().At(i)
							if svc.Proto == "tcp" && svc.Port == 22 {
								props.SSHEnabled = true
								break
							}
						}
					}
					if hiJSON, err := json.Marshal(hi); err == nil {
						props.HostinfoJSON = string(hiJSON)
					}
				}
				currentProps[nodeName] = props
			}

			// Check for route changes
			if !routeMapEqual(currentRoutes, lastNetMapRoutes) {
				for node, routes := range currentRoutes {
					if oldRoutes, exists := lastNetMapRoutes[node]; !exists {
						changes = append(changes, fmt.Sprintf("Node %s added routes: %v", node, routes))
					} else if !stringSliceEqual(routes, oldRoutes) {
						changes = append(changes, fmt.Sprintf("Node %s routes: %v -> %v", node, oldRoutes, routes))
					}
				}
				for node := range lastNetMapRoutes {
					if _, exists := currentRoutes[node]; !exists {
						changes = append(changes, fmt.Sprintf("Node %s removed routes", node))
					}
				}
				lastNetMapRoutes = currentRoutes
			}

			// Check for peer property changes
			for nodeName, props := range currentProps {
				if oldProps, exists := lastNetMapPeerProps[nodeName]; exists {
					if props.Online != oldProps.Online {
						changes = append(changes, fmt.Sprintf("Node %s Online: %v -> %v", nodeName, oldProps.Online, props.Online))
					}
					if props.OffersExitNode != oldProps.OffersExitNode {
						changes = append(changes, fmt.Sprintf("Node %s OffersExitNode: %v -> %v", nodeName, oldProps.OffersExitNode, props.OffersExitNode))
					}
					if props.SSHEnabled != oldProps.SSHEnabled {
						changes = append(changes, fmt.Sprintf("Node %s SSH: %v -> %v", nodeName, oldProps.SSHEnabled, props.SSHEnabled))
					}
					if props.HostinfoJSON != oldProps.HostinfoJSON && props.HostinfoJSON != "" && oldProps.HostinfoJSON != "" {
						changes = append(changes, fmt.Sprintf("Node %s Hostinfo changed", nodeName))
					}
				} else {
					// New peer - report its initial properties if notable
					if props.OffersExitNode {
						changes = append(changes, fmt.Sprintf("Node %s OffersExitNode: %v", nodeName, props.OffersExitNode))
					}
					if props.SSHEnabled {
						changes = append(changes, fmt.Sprintf("Node %s SSH: %v", nodeName, props.SSHEnabled))
					}
				}
			}
			lastNetMapPeerProps = currentProps

			if len(changes) > 0 {
				p.Send(IPNEventMsg{
					Type:    "event",
					Message: fmt.Sprintf("📨 NetMap changed: %s", strings.Join(changes, ", ")),
				})
			}
		}

		// Check for other important events
		if n.BrowseToURL != nil {
			p.Send(IPNEventMsg{
				Type:    "event",
				Message: fmt.Sprintf("📨 BrowseToURL: %s", *n.BrowseToURL),
			})
		}

		if n.LoginFinished != nil {
			p.Send(IPNEventMsg{
				Type:    "event",
				Message: "📨 Login finished",
			})
		}
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func routeMapEqual(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, exists := b[k]; !exists || !stringSliceEqual(v, bv) {
			return false
		}
	}
	return true
}
