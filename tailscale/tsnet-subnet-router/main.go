package main

import (
	"context"
	"log"
	"net/netip"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

func main() {
	s := &tsnet.Server{Hostname: "tsnet-subnet-example"}
	defer s.Close()

	if err := s.Start(); err != nil {
		log.Fatalf("tsnet start: %v", err)
	}

	lc, _ := s.LocalClient()

	subnet := netip.MustParsePrefix("192.168.42.0/24")
	mp := &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			AdvertiseRoutes: []netip.Prefix{subnet},
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
	if _, err := s.Up(ctx); err != nil {
		log.Fatalf("up: %v", err)
	}

	log.Println("subnet router is advertising 192.168.42.0/24")
	select {} // keep the process alive
}
