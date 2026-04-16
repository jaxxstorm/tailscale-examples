package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tailscale.com/hostinfo"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

const (
	appLibrary = "an-example-tsnet-app"
	appVersion = "v0.1.0"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// tsnet reports Package=tsnet on Start; App is the custom identifier for
	// the embedding application, so set it before the server is started.
	hostinfo.SetApp(fmt.Sprintf("%s/%s", appLibrary, appVersion))
	hostinfo.SetDeviceModel("my-model")

	srv := &tsnet.Server{
		Hostname: "example",
	}
	defer srv.Close()

	status, err := upWithLoginPrompt(ctx, srv)
	if err != nil {
		log.Fatalf("starting tsnet server: %v", err)
	}

	log.Printf("tailnet node is up: %s", status.Self.DNSName)
	<-ctx.Done()
}

func upWithLoginPrompt(ctx context.Context, srv *tsnet.Server) (*ipnstate.Status, error) {
	if err := srv.Start(); err != nil {
		return nil, err
	}

	lc, err := srv.LocalClient()
	if err != nil {
		return nil, err
	}

	loginStarted := false
	printedAuthURL := ""
	machineAuthShown := false

	for {
		st, err := lc.StatusWithoutPeers(ctx)
		if err != nil {
			return nil, err
		}

		switch st.BackendState {
		case "Running":
			return st, nil
		case "NeedsLogin":
			if !loginStarted {
				if err := lc.StartLoginInteractive(ctx); err != nil {
					return nil, err
				}
				loginStarted = true
			}
			if st.AuthURL != "" && st.AuthURL != printedAuthURL {
				log.Printf("log in at: %s", st.AuthURL)
				printedAuthURL = st.AuthURL
			}
		case "NeedsMachineAuth":
			if !machineAuthShown {
				log.Printf("machine approval required in the Tailscale admin console")
				machineAuthShown = true
			}
		}

		time.Sleep(time.Second)
	}
}
