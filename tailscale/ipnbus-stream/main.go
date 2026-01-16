package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/types/netmap"
)

func main() {
	var socket string
	var pretty bool

	flag.StringVar(&socket, "socket", "", "override tailscaled LocalAPI socket/path (leave empty for platform default)")
	flag.BoolVar(&pretty, "pretty", true, "pretty-print JSON")
	flag.Parse()

	// Context lifetime is the watch lifetime. Cancel it to stop the stream.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl-C / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	lc := &local.Client{
		// If Socket is empty, the client uses a platform-specific default.
		// (unix socket on macOS/Linux, named pipe on Windows, etc.)
		Socket: socket,
	}

	// Pick a mask that gives you useful initial state + ongoing engine updates.
	// You can tune this depending on what you care about.
	mask := ipn.NotifyInitialState |
		ipn.NotifyInitialPrefs |
		ipn.NotifyInitialNetMap |
		ipn.NotifyWatchEngineUpdates

	w, err := lc.WatchIPNBus(ctx, mask)
	if err != nil {
		log.Fatalf("WatchIPNBus: %v", err)
	}
	defer func() {
		_ = w.Close()
	}()

	log.Printf("Connected to Tailscale daemon - monitoring route changes...")

	var lastAdvertisedRoutes []string

	for {
		n, err := w.Next()
		if err != nil {
			// When ctx is cancelled, Next returns that error.
			if ctx.Err() != nil {
				log.Printf("stopping (ctx done): %v", ctx.Err())
				return
			}
			log.Fatalf("Next: %v", err)
		}

		// Check for errors
		if n.ErrMessage != nil {
			log.Printf("❌ Daemon error: %s", *n.ErrMessage)
		}

		// Check for preferences changes (advertised routes)
		if n.Prefs != nil {
			currentRoutes := extractAdvertisedRoutesFromPrefs(n.Prefs)
			if routesChanged(lastAdvertisedRoutes, currentRoutes) {
				log.Printf("🔄 Route configuration changed:")
				if len(lastAdvertisedRoutes) == 0 {
					log.Printf("   Added: %v", currentRoutes)
				} else if len(currentRoutes) == 0 {
					log.Printf("   Removed: %v", lastAdvertisedRoutes)
				} else {
					log.Printf("   From: %v", lastAdvertisedRoutes)
					log.Printf("   To:   %v", currentRoutes)
				}
				lastAdvertisedRoutes = currentRoutes
			}
		}

		// Check for NetMap changes (shows which nodes are advertising routes)
		if n.NetMap != nil {
			nodeRoutes := extractRoutesFromNetMap(n.NetMap)
			if len(nodeRoutes) > 0 {
				log.Printf("🌐 Active routes in network:")
				for nodeName, routes := range nodeRoutes {
					log.Printf("   %s: %v", nodeName, routes)
				}
			}
		}

		// Skip all other notifications (engine updates, etc.)
		// Only show route changes and critical errors
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
