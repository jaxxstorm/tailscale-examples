package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"golang.org/x/oauth2/clientcredentials"
	"tailscale.com/tsnet"
)

const (
	defaultTailscaleAPIBase = "https://api.tailscale.com"
)

var (
	tailscaleAPIBase = defaultTailscaleAPIBase
)

func main() {
	var cfg Config
	kong.Parse(&cfg)

	ctx := context.Background()

	// Exchange JWT for Tailscale access token
	fmt.Println("Exchanging JWT for Tailscale access token...")
	accessToken, err := exchangeJWTForAccessToken(ctx, cfg.ClientID, cfg.JWT)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exchanging JWT: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Got access token (prefix):", prefix(accessToken))

	// Create auth key using the access token
	fmt.Println("Creating auth key...")
	authKey, err := createAuthKey(ctx, accessToken, cfg.Tag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating auth key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Got auth key (prefix):", prefix(authKey))

	// Start a tsnet server with that auth key
	if err := startTsnetServer(ctx, cfg.Hostname, authKey, cfg.Port); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting tsnet server: %v\n", err)
		os.Exit(1)
	}
}

// exchangeJWTForAccessToken exchanges a JWT for a Tailscale access token using oauth2.
func exchangeJWTForAccessToken(ctx context.Context, clientID, jwt string) (string, error) {
	config := clientcredentials.Config{
		ClientID: clientID,
		TokenURL: tailscaleAPIBase + "/api/v2/oauth/token-exchange",
		EndpointParams: url.Values{
			"jwt": {jwt},
		},
	}

	token, err := config.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}

	if token.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	return token.AccessToken, nil
}

// createAuthKey creates a Tailscale auth key using an access token.
func createAuthKey(ctx context.Context, accessToken, tag string) (string, error) {
	reqBody := AuthKeyRequest{
		Capabilities: AuthKeyCapabilities{
			Devices: DeviceCapabilities{
				Create: DeviceCreateCapabilities{
					Reusable:      false,
					Ephemeral:     true,
					Preauthorized: true,
					Tags:          []string{tag},
				},
			},
		},
		ExpirySeconds: int((24 * time.Hour).Seconds()),
		Description:   "tsnet WIF demo",
	}

	buf, err := json.Marshal(&reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tailscaleAPIBase+"/api/v2/tailnet/-/keys",
		strings.NewReader(string(buf)),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create auth key failed: %s: %s", resp.Status, string(body))
	}

	var out AuthKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Key == "" {
		return "", fmt.Errorf("empty key in create auth key response")
	}
	return out.Key, nil
}

// startTsnetServer starts a tsnet server with the given auth key.
func startTsnetServer(ctx context.Context, hostname, authKey, port string) error {
	var s tsnet.Server
	s.Hostname = hostname
	s.AuthKey = authKey
	defer s.Close()

	ln, err := s.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("tsnet listen: %w", err)
	}
	defer ln.Close()

	fmt.Printf("tsnet server listening on tailnet port %s\n", port)

	return http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from tsnet via WIF!\nremote addr: %s\n", r.RemoteAddr)
	}))
}

// prefix returns the first 10 characters of a string for display purposes.
func prefix(k string) string {
	if len(k) > 10 {
		return k[:10] + "..."
	}
	return k
}
