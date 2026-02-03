package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

var CLI struct {
	Subnets  []string `arg:"" name:"subnets" help:"Subnets to advertise (e.g., 192.168.1.0/24)" optional:""`
	ExitNode bool     `name:"exit-node" help:"Advertise as an exit node (routes all traffic)" default:"false"`
	Hostname string   `name:"hostname" help:"Tailscale hostname" default:"tsnet-subnet-router"`
	AuthKey  string   `name:"auth-key" help:"Tailscale authentication key (optional)" env:"TS_AUTHKEY"`
}

func main() {
	kong.Parse(&CLI,
		kong.Name("tsnet-subnet-router"),
		kong.Description("A Tailscale subnet router using tsnet"),
		kong.UsageOnError(),
	)

	s := &tsnet.Server{
		Hostname: CLI.Hostname,
		AuthKey:  CLI.AuthKey,
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		log.Fatalf("tsnet start: %v", err)
	}

	lc, _ := s.LocalClient()

	// Parse subnet prefixes
	var routes []netip.Prefix
	for _, subnet := range CLI.Subnets {
		prefix, err := netip.ParsePrefix(subnet)
		if err != nil {
			log.Fatalf("invalid subnet %q: %v", subnet, err)
		}
		routes = append(routes, prefix)
	}

	// Add default routes if exit node is requested
	if CLI.ExitNode {
		log.Println("Advertising as exit node")
		routes = append(routes,
			netip.MustParsePrefix("0.0.0.0/0"),
			netip.MustParsePrefix("::/0"),
		)
	}

	if len(routes) == 0 {
		log.Fatalf("no routes to advertise: specify subnets or use --exit-node")
	}

	mp := &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			AdvertiseRoutes: routes,
			WantRunning:     true,
		},
		AdvertiseRoutesSet: true,
		WantRunningSet:     true,
	}

	if _, err := lc.EditPrefs(context.Background(), mp); err != nil {
		log.Fatalf("edit prefs: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check if we need to log login URL
	if CLI.AuthKey == "" {
		status, err := lc.Status(ctx)
		if err == nil && status.AuthURL != "" {
			fmt.Printf("\nAuthentication required. Please visit:\n%s\n\n", status.AuthURL)
		}
	}

	if _, err := s.Up(ctx); err != nil {
		log.Fatalf("up: %v", err)
	}

	var routeStrs []string
	for _, r := range routes {
		routeStrs = append(routeStrs, r.String())
	}
	log.Printf("subnet router is advertising: %s", strings.Join(routeStrs, ", "))
	select {} // keep the process alive
}
