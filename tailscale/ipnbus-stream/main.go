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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/types/netmap"
)

// RouteEventMsg represents a route change event
type RouteEventMsg struct {
	Type       string
	NodeName   string
	OldRoutes  []string
	NewRoutes  []string
	NodeRoutes map[string][]string
	Error      string
	NetMapJSON string // Debug: full netmap as JSON
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
	case RouteEventMsg:
		timestamp := timestampStyle.Render(time.Now().Format("15:04:05"))
		
		switch msg.Type {
		case "connected":
			m.status = "Connected to Tailscale daemon"
			eventStr := fmt.Sprintf("%s %s", 
				timestamp,
				connectedStyle.Render("✅ Connected to Tailscale daemon"))
			m.events = append(m.events, eventStr)
			
			// Show initial state
			if len(msg.NodeRoutes) > 0 {
				eventStr := fmt.Sprintf("%s %s", 
					timestamp,
					connectedStyle.Render("🌐 Initial network state - Active routes:"))
				m.events = append(m.events, eventStr)
				for nodeName, routes := range msg.NodeRoutes {
					detail := fmt.Sprintf("     %s: %s", 
						nodeStyle.Render(nodeName), 
						routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(routes, ", "))))
					m.events = append(m.events, detail)
				}
			} else {
				eventStr := fmt.Sprintf("%s %s", 
					timestamp,
					statusStyle.Render("🌐 No routes currently advertised"))
				m.events = append(m.events, eventStr)
			}
			
		case "preference_change":
			nodeName := msg.NodeName
			if nodeName == "" {
				nodeName = "local"
			}
			
			eventStr := fmt.Sprintf("%s %s %s", 
				timestamp,
				routeChangeStyle.Render("🔄 Local route configuration changed:"),
				nodeStyle.Render(fmt.Sprintf("(%s)", nodeName)))
			m.events = append(m.events, eventStr)
			
			if len(msg.OldRoutes) == 0 {
				detail := fmt.Sprintf("     %s %s", 
					routeChangeStyle.Render("Added:"), 
					routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(msg.NewRoutes, ", "))))
				m.events = append(m.events, detail)
			} else if len(msg.NewRoutes) == 0 {
				detail := fmt.Sprintf("     %s %s", 
					routeChangeStyle.Render("Removed:"), 
					routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(msg.OldRoutes, ", "))))
				m.events = append(m.events, detail)
			} else {
				fromDetail := fmt.Sprintf("     %s %s", 
					routeChangeStyle.Render("From:"), 
					routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(msg.OldRoutes, ", "))))
				toDetail := fmt.Sprintf("     %s %s", 
					routeChangeStyle.Render("To:"), 
					routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(msg.NewRoutes, ", "))))
				m.events = append(m.events, fromDetail, toDetail)
			}
			
		case "netmap_update":
			if len(msg.NodeRoutes) > 0 {
				eventStr := fmt.Sprintf("%s %s", 
					timestamp,
					connectedStyle.Render("🌐 Network routes changed:"))
				m.events = append(m.events, eventStr)
				for nodeName, routes := range msg.NodeRoutes {
					detail := fmt.Sprintf("     %s: %s", 
						nodeStyle.Render(nodeName), 
						routeStyle.Render(fmt.Sprintf("[%s]", strings.Join(routes, ", "))))
					m.events = append(m.events, detail)
				}
			} else {
				eventStr := fmt.Sprintf("%s %s", 
					timestamp,
					statusStyle.Render("🌐 No routes currently advertised"))
				m.events = append(m.events, eventStr)
			}
			
			// Debug: show full netmap
			if m.debug && msg.NetMapJSON != "" {
				m.events = append(m.events, fmt.Sprintf("%s %s", 
					timestamp,
					debugStyle.Render("🔍 DEBUG - Full NetMap:")))
				m.events = append(m.events, debugStyle.Render(msg.NetMapJSON))
			}
			
		case "error":
			eventStr := fmt.Sprintf("%s %s", 
				timestamp,
				errorStyle.Render(fmt.Sprintf("❌ Error: %s", msg.Error)))
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

	// Setup bubbletea
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

	// Handle Ctrl-C / SIGTERM gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start route monitoring in a goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.Send(RouteEventMsg{
					Type:  "error",
					Error: fmt.Sprintf("Monitoring crashed: %v", r),
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
		p.Send(RouteEventMsg{
			Type:  "error",
			Error: fmt.Sprintf("Failed to connect to Tailscale daemon: %v", err),
		})
		return
	}
	defer func() {
		_ = w.Close()
	}()

	var lastAdvertisedRoutes []string
	var lastNetMapRoutes map[string][]string

	for {
		n, err := w.Next()
		if err != nil {
			// When ctx is cancelled, Next returns that error.
			if ctx.Err() != nil {
				return
			}
			p.Send(RouteEventMsg{
				Type:  "error",
				Error: fmt.Sprintf("Error reading from IPN bus: %v", err),
			})
			return
		}

		// Check for daemon errors
		if n.ErrMessage != nil {
			p.Send(RouteEventMsg{
				Type:  "error",
				Error: *n.ErrMessage,
			})
		}

		// Check for preferences changes (advertised routes)
		if n.Prefs != nil {
			currentRoutes := extractAdvertisedRoutesFromPrefs(n.Prefs)
			if routesChanged(lastAdvertisedRoutes, currentRoutes) {
				p.Send(RouteEventMsg{
					Type:      "preference_change",
					NodeName:  "local",
					OldRoutes: lastAdvertisedRoutes,
					NewRoutes: currentRoutes,
				})
				lastAdvertisedRoutes = currentRoutes
			}
		}

		// Check for NetMap changes (shows which nodes are advertising routes)
		if n.NetMap != nil {
			nodeRoutes := extractRoutesFromNetMap(n.NetMap)
			
			var netmapJSON string
			if debug {
				if b, err := json.MarshalIndent(n.NetMap, "", "  "); err == nil {
					netmapJSON = string(b)
				}
			}

			// Check if network-wide routes changed
			if lastNetMapRoutes == nil || netMapRoutesChanged(lastNetMapRoutes, nodeRoutes) {
				eventType := "netmap_update"
				if lastNetMapRoutes == nil {
					eventType = "connected"
				}

				p.Send(RouteEventMsg{
					Type:       eventType,
					NodeRoutes: nodeRoutes,
					NetMapJSON: netmapJSON,
				})
				lastNetMapRoutes = nodeRoutes
			}
		}
	}
}

func printNotify(n ipn.Notify, pretty bool) error {
	// Add a timestamp wrapper so logs are easier to ingest.
	type wrapped struct {
		At     time.Time  `json:"at"`
		Notify ipn.Notify `json:"notify"`
	}
	w := wrapped{At: time.Now(), Notify: n}

	var b []byte
	var err error
	if pretty {
		b, err = json.MarshalIndent(w, "", "  ")
	} else {
		b, err = json.Marshal(w)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// extractAdvertisedRoutesFromPrefs extracts the AdvertiseRoutes from preferences
func extractAdvertisedRoutesFromPrefs(prefs *ipn.PrefsView) []string {
	var routes []string
	if prefs != nil {
		advertiseRoutes := prefs.AdvertiseRoutes()
		for i := 0; i < advertiseRoutes.Len(); i++ {
			route := advertiseRoutes.At(i)
			routes = append(routes, route.String())
		}
	}
	return routes
}

// extractRoutesFromNetMap extracts routes from all nodes in the network map
func extractRoutesFromNetMap(netMap *netmap.NetworkMap) map[string][]string {
	nodeRoutes := make(map[string][]string)

	// Check all peer nodes in the network
	for _, node := range netMap.Peers {
		if !node.Valid() {
			continue
		}

		nodeName := node.ComputedName()
		if nodeName == "" {
			nodeName = node.Name()
		}

		var routes []string

		// Get primary routes (subnet routes this node advertises)
		primaryRoutes := node.PrimaryRoutes()
		for j := 0; j < primaryRoutes.Len(); j++ {
			route := primaryRoutes.At(j)
			routes = append(routes, route.String())
		}

		// Only include nodes that actually advertise routes
		if len(routes) > 0 {
			nodeRoutes[nodeName] = routes
		}
	}

	// Also check our own node (SelfNode)
	if netMap.SelfNode.Valid() {
		selfNode := netMap.SelfNode
		nodeName := selfNode.ComputedName()
		if nodeName == "" {
			nodeName = selfNode.Name()
		}

		var routes []string
		primaryRoutes := selfNode.PrimaryRoutes()
		for i := 0; i < primaryRoutes.Len(); i++ {
			route := primaryRoutes.At(i)
			routes = append(routes, route.String())
		}

		if len(routes) > 0 {
			nodeRoutes[nodeName+" (self)"] = routes
		}
	}

	return nodeRoutes
}

// routesChanged compares two route slices to detect changes
func routesChanged(old, new []string) bool {
	if len(old) != len(new) {
		return true
	}

	// Create maps for easier comparison
	oldMap := make(map[string]bool)
	newMap := make(map[string]bool)

	for _, route := range old {
		oldMap[route] = true
	}

	for _, route := range new {
		newMap[route] = true
	}

	// Check if any routes were added or removed
	for route := range oldMap {
		if !newMap[route] {
			return true
		}
	}

	for route := range newMap {
		if !oldMap[route] {
			return true
		}
	}

	return false
}

// netMapRoutesChanged compares two network maps to detect changes
func netMapRoutesChanged(old, new map[string][]string) bool {
	if len(old) != len(new) {
		return true
	}

	// Check if any nodes were added or removed
	for nodeName := range old {
		if _, exists := new[nodeName]; !exists {
			return true
		}
	}

	for nodeName := range new {
		if _, exists := old[nodeName]; !exists {
			return true
		}
	}

	// Check if routes for existing nodes changed
	for nodeName, oldRoutes := range old {
		newRoutes, exists := new[nodeName]
		if !exists {
			return true
		}
		if routesChanged(oldRoutes, newRoutes) {
			return true
		}
	}

	return false
}
