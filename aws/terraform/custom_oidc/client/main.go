package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	imds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaxxstorm/tailscale-examples/aws/terraform/oidc/client/pkg/logging"
)

const (
	version   = "3.0.0"
	userAgent = "tailscale-ec2-client/" + version
)

type Config struct {
	OIDCProviderURL   string
	TailscaleClientID string
	TailscaleAudience string
	Debug             bool
	Tags              []string
	Timeout           time.Duration
	Ephemeral         bool
	Preauthorized     bool
	Reusable          bool
	ExpirySeconds     int
	Export            bool
}

type IMDSClient struct {
	client *imds.Client
	logger *logging.Logger
}

func NewIMDSClient(logger *logging.Logger) *IMDSClient {
	// Load AWS config with IMDS endpoint only
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithEC2IMDSRegion(),
	)
	if err != nil {
		logger.Error("Failed to load AWS config for IMDS", zap.Error(err))
		return nil
	}
	client := imds.NewFromConfig(cfg)
	return &IMDSClient{
		client: client,
		logger: logger.WithComponent("imds"),
	}
}

func (i *IMDSClient) getMetadata(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch path {
	case "dynamic/instance-identity/document":
		// Get RAW instance identity document, not the parsed version
		out, err := i.client.GetDynamicData(ctx, &imds.GetDynamicDataInput{Path: "instance-identity/document"})
		if err != nil {
			return nil, fmt.Errorf("failed to get instance identity document: %w", err)
		}
		defer out.Content.Close()
		data, err := io.ReadAll(out.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to read instance identity document content: %w", err)
		}
		return data, nil
	case "dynamic/instance-identity/signature":
		out, err := i.client.GetDynamicData(ctx, &imds.GetDynamicDataInput{Path: "instance-identity/signature"})
		if err != nil {
			return nil, fmt.Errorf("failed to get instance signature: %w", err)
		}
		defer out.Content.Close()
		data, err := io.ReadAll(out.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to read instance signature content: %w", err)
		}
		return data, nil
	case "iam/info":
		out, err := i.client.GetIAMInfo(ctx, &imds.GetIAMInfoInput{})
		if err != nil {
			return nil, fmt.Errorf("failed to get iam/info: %w", err)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal iam/info: %w", err)
		}
		return b, nil
	case "iam/security-credentials/":
		out, err := i.client.GetMetadata(ctx, &imds.GetMetadataInput{Path: "iam/security-credentials/"})
		if err != nil {
			return nil, fmt.Errorf("failed to get iam/security-credentials/: %w", err)
		}
		defer out.Content.Close()
		data, err := io.ReadAll(out.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to read iam/security-credentials/ content: %w", err)
		}
		return data, nil
	default:
		out, err := i.client.GetMetadata(ctx, &imds.GetMetadataInput{Path: path})
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata %s: %w", path, err)
		}
		defer out.Content.Close()
		data, err := io.ReadAll(out.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata %s content: %w", path, err)
		}
		return data, nil
	}
}

type InstanceMetadata struct {
	InstanceIdentity string
	Signature        string
	RoleARN          string
}

func (i *IMDSClient) GetInstanceMetadata() (*InstanceMetadata, error) {
	i.logger.Info("Gathering instance metadata...")

	// Get instance identity document
	identityData, err := i.getMetadata("dynamic/instance-identity/document")
	if err != nil {
		return nil, fmt.Errorf("failed to get instance identity: %w", err)
	}

	// Get signature
	signatureData, err := i.getMetadata("dynamic/instance-identity/signature")
	if err != nil {
		return nil, fmt.Errorf("failed to get instance signature: %w", err)
	}

	// Parse identity for account ID
	var identity map[string]interface{}
	if err := json.Unmarshal(identityData, &identity); err != nil {
		return nil, fmt.Errorf("failed to parse instance identity: %w", err)
	}

	// Try to get role ARN using multiple methods
	roleARN, err := i.getRoleARN(identity)
	if err != nil {
		return nil, fmt.Errorf("failed to get role ARN: %w", err)
	}

	i.logger.Debug("Instance metadata gathered",
		zap.String("role_arn", roleARN),
		zap.String("instance_id", identity["instanceId"].(string)))

	return &InstanceMetadata{
		InstanceIdentity: string(identityData),
		Signature:        string(signatureData),
		RoleARN:          roleARN,
	}, nil
}

func (i *IMDSClient) getRoleARN(identity map[string]interface{}) (string, error) {
	// Method 1: Get role name from security-credentials (most reliable)
	// This gives us the actual role name, not the instance profile name
	if roleNameData, err := i.getMetadata("iam/security-credentials/"); err == nil {
		roleName := strings.TrimSpace(string(roleNameData))
		lines := strings.Split(roleName, "\n")
		if len(lines) > 0 && lines[0] != "" {
			accountID, ok := identity["accountId"].(string)
			if !ok {
				return "", fmt.Errorf("no accountId in instance identity")
			}
			roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, lines[0])
			i.logger.Debug("Got role ARN from security-credentials", zap.String("role_arn", roleARN))
			return roleARN, nil
		}
	}

	// Method 2: Try iam/info as fallback
	if iamInfo, err := i.getMetadata("iam/info"); err == nil {
		var info map[string]interface{}
		if err := json.Unmarshal(iamInfo, &info); err == nil {
			if profileArn, ok := info["InstanceProfileArn"].(string); ok {
				// Convert instance profile ARN to role ARN
				roleARN := strings.Replace(profileArn, "instance-profile", "role", 1)
				i.logger.Debug("Got role ARN from iam/info (converted from instance profile)", zap.String("role_arn", roleARN))
				return roleARN, nil
			}
		}
	}

	return "", fmt.Errorf("could not determine IAM role ARN")
}

type OIDCClient struct {
	providerURL string
	client      *http.Client
	logger      *logging.Logger
}

func NewOIDCClient(providerURL string, logger *logging.Logger) *OIDCClient {
	return &OIDCClient{
		providerURL: providerURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.WithComponent("oidc"),
	}
}

type OIDCTokenRequest struct {
	InstanceIdentity string `json:"instance_identity"`
	Signature        string `json:"signature"`
	RoleARN          string `json:"role_arn"`
	Audience         string `json:"audience"`
}

type OIDCTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (o *OIDCClient) GetToken(metadata *InstanceMetadata, audience string) (string, error) {
	o.logger.Info("Requesting JWT from custom OIDC provider...")

	request := OIDCTokenRequest{
		InstanceIdentity: metadata.InstanceIdentity,
		Signature:        metadata.Signature,
		RoleARN:          metadata.RoleARN,
		Audience:         audience,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OIDC request: %w", err)
	}

	o.logger.Debug("OIDC request payload", zap.String("json", string(jsonData)))

	req, err := http.NewRequest("POST", o.providerURL+"/token", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	o.logger.Debug("Making request to OIDC provider", zap.String("url", o.providerURL+"/token"))

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send OIDC request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OIDC response: %w", err)
	}

	o.logger.Debug("OIDC response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC provider returned error %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse OIDC response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no JWT received from OIDC provider")
	}

	// Debug: Show JWT claims like the bash script does
	o.logger.Info("✅ JWT obtained from OIDC provider")

	// Parse and log JWT parts for debugging
	parts := strings.Split(tokenResp.AccessToken, ".")
	if len(parts) == 3 {
		// Decode header
		if headerData, err := base64Decode(parts[0]); err == nil {
			o.logger.Debug("JWT Header", zap.String("header", string(headerData)))
		}

		// Decode payload
		if payloadData, err := base64Decode(parts[1]); err == nil {
			o.logger.Debug("JWT Payload", zap.String("payload", string(payloadData)))

			// Parse payload to check issuer
			var payload map[string]interface{}
			if err := json.Unmarshal(payloadData, &payload); err == nil {
				if iss, ok := payload["iss"].(string); ok {
					o.logger.Info("JWT Issuer", zap.String("issuer", iss))
					if !strings.Contains(iss, "sf404j1v9b.execute-api.us-west-2.amazonaws.com") {
						o.logger.Error("⚠️  JWT has wrong issuer!",
							zap.String("expected_contains", "sf404j1v9b.execute-api.us-west-2.amazonaws.com"),
							zap.String("actual", iss))
					}
				}
			}
		}
	}

	return tokenResp.AccessToken, nil
}

// Helper function for base64 decoding JWT parts
func base64Decode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

type TailscaleClient struct {
	client *http.Client
	logger *logging.Logger
}

func NewTailscaleClient(logger *logging.Logger) *TailscaleClient {
	return &TailscaleClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.WithComponent("tailscale"),
	}
}

type TailscaleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (t *TailscaleClient) ExchangeToken(clientID, jwt string) (string, error) {
	t.logger.Info("Exchanging JWT for Tailscale access token...")

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("jwt", jwt)

	req, err := http.NewRequest("POST", "https://api.tailscale.com/api/v2/oauth/token-exchange", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token exchange request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send token exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token exchange response: %w", err)
	}

	t.logger.Debug("Tailscale token exchange response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tailscale token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TailscaleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token exchange response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token received from Tailscale")
	}

	t.logger.Info("✅ Tailscale token exchange successful!")
	return tokenResp.AccessToken, nil
}

type AuthKeyRequest struct {
	Capabilities  map[string]interface{} `json:"capabilities"`
	ExpirySeconds int                    `json:"expirySeconds"`
	Description   string                 `json:"description,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
}

type AuthKeyResponse struct {
	Key string `json:"key"`
}

func (t *TailscaleClient) CreateAuthKey(accessToken string, config *Config) (string, error) {
	t.logger.Info("Creating Tailscale auth key...")

	capabilities := map[string]interface{}{
		"devices": map[string]interface{}{
			"create": map[string]interface{}{
				"reusable":      config.Reusable,
				"ephemeral":     config.Ephemeral,
				"preauthorized": config.Preauthorized,
				"tags":          config.Tags,
			},
		},
	}

	userAgentSanitized := strings.NewReplacer(" ", "_", "/", "-", ":", "-", ".", "_").Replace(userAgent)

	authKeyReq := AuthKeyRequest{
		Capabilities:  capabilities,
		ExpirySeconds: config.ExpirySeconds,
		Description:   fmt.Sprintf("OIDC auth key created by %s", userAgentSanitized),
		Tags:          config.Tags,
	}

	jsonData, err := json.Marshal(authKeyReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth key request: %w", err)
	}

	t.logger.Debug("Auth key request", zap.String("json", string(jsonData)))

	req, err := http.NewRequest("POST", "https://api.tailscale.com/api/v2/tailnet/-/keys", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create auth key request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send auth key request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read auth key response: %w", err)
	}

	t.logger.Debug("Auth key response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create auth key, status %d: %s", resp.StatusCode, string(body))
	}

	var authKeyResp AuthKeyResponse
	if err := json.Unmarshal(body, &authKeyResp); err != nil {
		return "", fmt.Errorf("failed to parse auth key response: %w", err)
	}

	if authKeyResp.Key == "" {
		return "", fmt.Errorf("no auth key received from Tailscale")
	}

	t.logger.Info("✅ Auth key created successfully!")
	return authKeyResp.Key, nil
}

type LocalAPIClient struct {
	client *http.Client
	logger *logging.Logger
}

func NewLocalAPIClient(logger *logging.Logger) *LocalAPIClient {
	// Use Unix socket for Tailscale local API
	socketPath := "/run/tailscale/tailscaled.sock"
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &LocalAPIClient{
		client: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
		logger: logger.WithComponent("localapi"),
	}
}

func (l *LocalAPIClient) AuthenticateWithKey(authKey string, tags []string) error {
	l.logger.Info("Authenticating with Tailscale using auth key...")

	// First, check if we're already running
	if status, err := l.getStatus(); err == nil {
		if status["BackendState"] == "Running" {
			l.logger.Info("✅ Tailscale is already running and connected!")
			return nil
		}
		l.logger.Info("Current Tailscale state", zap.Any("state", status["BackendState"]))
	}

	// Let's trace exactly what the CLI does - looking at the CLI source:
	// 1. It calls localClient.Start() with ipn.Options{AuthKey: authKey, UpdatePrefs: prefs}
	// 2. Then if forceReauth OR !st.HaveNodeKey, it calls StartLoginInteractive

	// Let's check if we have a node key first
	status, err := l.getStatus()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	haveNodeKey, _ := status["HaveNodeKey"].(bool)
	l.logger.Info("Node key status", zap.Bool("have_node_key", haveNodeKey))

	// Step 1: Call start with auth key (exactly like CLI)
	if err := l.callStartWithAuthKey(authKey, tags); err != nil {
		l.logger.Warn("LocalAPI start failed, falling back to CLI", zap.Error(err))
		return l.authenticateViaCLI(authKey, tags)
	}

	// Step 2: If we don't have a node key, call login-interactive (like CLI does)
	if !haveNodeKey {
		l.logger.Info("No node key found, calling login-interactive...")
		if err := l.callLoginInteractive(); err != nil {
			l.logger.Warn("Login interactive failed", zap.Error(err))
		}
	}

	// Step 3: Check status once after auth, exit promptly if successful
	status, err = l.getStatus()
	if err != nil {
		l.logger.Warn("Failed to get status after auth, trying CLI as backup", zap.Error(err))
		return l.authenticateViaCLI(authKey, tags)
	}
	if state, ok := status["BackendState"]; ok && state == "Running" {
		l.logger.Info("✅ Tailscale is connected and running!")
		return nil
	}
	if state, ok := status["BackendState"]; ok && state == "NeedsMachineAuth" {
		l.logger.Info("🔑 Machine needs authorization from admin")
		return nil
	}
	l.logger.Warn("Tailscale not connected after auth, trying CLI as backup", zap.Any("state", status["BackendState"]))
	return l.authenticateViaCLI(authKey, tags)
}

func (l *LocalAPIClient) callStartWithAuthKey(authKey string, tags []string) error {
	l.logger.Info("Calling start with auth key...")

	// Create minimal but complete preferences
	prefs := map[string]interface{}{
		"WantRunning": true,
		"RouteAll":    true, // accept-routes
		"CorpDNS":     true, // accept-dns
	}

	if len(tags) > 0 {
		prefs["AdvertiseTags"] = tags
	}

	// This is the exact structure the CLI uses: ipn.Options{AuthKey: authKey, UpdatePrefs: prefs}
	options := map[string]interface{}{
		"AuthKey":     authKey,
		"UpdatePrefs": prefs,
	}

	endpoint := "http://local-tailscaled.sock/localapi/v0/start"

	jsonData, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("failed to marshal start options: %w", err)
	}

	l.logger.Debug("Start request", zap.String("json", string(jsonData)))

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create start request: %w", err)
	}

	req.Host = "local-tailscaled.sock"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read start response: %w", err)
	}

	l.logger.Debug("Start response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)))

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("start failed with status %d: %s", resp.StatusCode, string(body))
	}

	l.logger.Info("✅ Start call completed successfully")
	return nil
}

func (l *LocalAPIClient) callLoginInteractive() error {
	l.logger.Info("Calling login-interactive...")

	endpoint := "http://local-tailscaled.sock/localapi/v0/login-interactive"

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create login-interactive request: %w", err)
	}

	req.Host = "local-tailscaled.sock"
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call login-interactive: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login-interactive response: %w", err)
	}

	l.logger.Debug("Login-interactive response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)))

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login-interactive failed with status %d: %s", resp.StatusCode, string(body))
	}

	l.logger.Info("✅ Login-interactive call completed")
	return nil
}

func (l *LocalAPIClient) waitForConnection() error {
	l.logger.Info("Waiting for Tailscale to connect...")
	// Instead of polling or watching the IPN bus, just check status once and exit if Running or NeedsMachineAuth
	status, err := l.getStatus()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}
	state, _ := status["BackendState"].(string)
	l.logger.Info("Tailscale backend state", zap.String("state", state))
	switch state {
	case "Running":
		l.logger.Info("✅ Tailscale is connected and running!")
		return nil
	case "NeedsMachineAuth":
		l.logger.Info("🔑 Machine needs authorization from admin")
		return nil
	default:
		return fmt.Errorf("tailscale backend state is %q, not authenticated", state)
	}
}

func (l *LocalAPIClient) watchIPNBus() error {
	l.logger.Info("Watching IPN bus for state changes...")

	endpoint := "http://local-tailscaled.sock/localapi/v0/watch-ipn-bus?mask=1" // NotifyInitialState

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create watch request: %w", err)
	}

	req.Host = "local-tailscaled.sock"
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to start watching IPN bus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("watch IPN bus failed with code: %d", resp.StatusCode)
	}

	l.logger.Info("Connected to IPN bus, watching for state changes...")

	// Set a timeout for the overall operation
	timeout := time.Now().Add(60 * time.Second)

	decoder := json.NewDecoder(resp.Body)
	for time.Now().Before(timeout) {
		var notification map[string]interface{}
		if err := decoder.Decode(&notification); err != nil {
			l.logger.Debug("Error reading from IPN bus", zap.Error(err))
			break
		}

		l.logger.Debug("IPN notification", zap.Any("notification", notification))

		// Check for state changes
		if state, ok := notification["State"]; ok {
			stateStr := fmt.Sprintf("%v", state)
			l.logger.Info("State change detected", zap.String("state", stateStr))

			switch stateStr {
			case "Running":
				l.logger.Info("✅ Tailscale is connected and running!")
				return nil
			case "NeedsMachineAuth":
				l.logger.Info("🔑 Machine needs authorization from admin")
				return nil
			case "NoState", "Starting":
				l.logger.Info("⏳ Tailscale is starting...")
				continue
			}
		}

		// Check for auth URLs (shouldn't happen with auth keys, but just in case)
		if authURL, ok := notification["BrowseToURL"]; ok && authURL != nil {
			l.logger.Warn("Unexpected auth URL received", zap.Any("auth_url", authURL))
		}

		// Check for errors
		if errMsg, ok := notification["ErrMessage"]; ok && errMsg != nil {
			return fmt.Errorf("backend error: %v", errMsg)
		}
	}

	return fmt.Errorf("timeout waiting for Tailscale to connect via IPN bus")
}

func (l *LocalAPIClient) getStatus() (map[string]interface{}, error) {
	endpoint := "http://local-tailscaled.sock/localapi/v0/status"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	req.Host = "local-tailscaled.sock"
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status check failed with code: %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status: %w", err)
	}

	return status, nil
}

// Use the Tailscale CLI for authentication - this is the standard approach
func (l *LocalAPIClient) authenticateViaCLI(authKey string, tags []string) error {
	l.logger.Info("Starting Tailscale authentication via CLI...")

	args := []string{"up", "--auth-key", authKey}

	// Add tags if provided
	if len(tags) > 0 {
		args = append(args, "--advertise-tags", strings.Join(tags, ","))
	}

	// Add other common options for headless operation
	args = append(args, "--accept-routes", "--accept-dns")

	l.logger.Debug("Running tailscale command", zap.Strings("args", args))

	cmd := exec.Command("tailscale", args...)
	output, err := cmd.CombinedOutput()

	l.logger.Debug("Tailscale CLI output", zap.String("output", string(output)))

	if err != nil {
		return fmt.Errorf("tailscale CLI failed: %w, output: %s", err, string(output))
	}

	l.logger.Info("✅ Tailscale CLI authentication completed")

	// Wait a moment for the connection to establish
	time.Sleep(3 * time.Second)

	// Verify the connection worked by checking status
	return l.waitForConnection()
}

func main() {
	var config Config

	var rootCmd = &cobra.Command{
		Use:   "tailscale-oidc-auth",
		Short: "AWS EC2 to Tailscale authentication via custom OIDC provider",
		Long: `A tool to authenticate EC2 instances with Tailscale using a custom OIDC provider.
This tool exchanges EC2 instance identity for a Tailscale access token, creates an auth key,
and authenticates the local Tailscale daemon.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(&config)
		},
	}

	// Define flags
	rootCmd.PersistentFlags().StringVarP(&config.OIDCProviderURL, "oidc-provider-url", "u", "", "OIDC provider URL (required)")
	rootCmd.PersistentFlags().StringVarP(&config.TailscaleClientID, "client-id", "c", "", "Tailscale OIDC client ID (required)")
	rootCmd.PersistentFlags().StringVarP(&config.TailscaleAudience, "audience", "a", "tailscale", "OIDC audience claim")
	rootCmd.PersistentFlags().BoolVarP(&config.Debug, "debug", "d", false, "Enable debug output")
	rootCmd.PersistentFlags().StringSliceVarP(&config.Tags, "tags", "t", []string{}, "Tailscale tags to advertise")
	rootCmd.PersistentFlags().DurationVar(&config.Timeout, "timeout", 30*time.Second, "Request timeout")
	rootCmd.PersistentFlags().BoolVar(&config.Ephemeral, "ephemeral", true, "Create ephemeral auth key")
	rootCmd.PersistentFlags().BoolVar(&config.Preauthorized, "preauthorized", true, "Create preauthorized auth key")
	rootCmd.PersistentFlags().BoolVar(&config.Reusable, "reusable", false, "Create reusable auth key")
	rootCmd.PersistentFlags().IntVar(&config.ExpirySeconds, "expiry", 3600, "Auth key expiry in seconds")
	rootCmd.PersistentFlags().BoolVar(&config.Export, "export", false, "Print the Tailscale API access token and auth key, then exit")

	// Mark required flags
	rootCmd.MarkPersistentFlagRequired("client-id")
	rootCmd.MarkPersistentFlagRequired("oidc-provider-url")

	// Initialize viper for configuration
	viper.SetEnvPrefix("TAILSCALE")
	viper.AutomaticEnv()
	viper.BindPFlag("oidc_provider_url", rootCmd.PersistentFlags().Lookup("oidc-provider-url"))
	viper.BindPFlag("client_id", rootCmd.PersistentFlags().Lookup("client-id"))
	viper.BindPFlag("audience", rootCmd.PersistentFlags().Lookup("audience"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))

	// Set values from environment variables if flags not provided
	if clientID := viper.GetString("client_id"); clientID != "" && config.TailscaleClientID == "" {
		config.TailscaleClientID = clientID
	}
	if audience := viper.GetString("audience"); audience != "" && config.TailscaleAudience == "tailscale" {
		config.TailscaleAudience = audience
	}
	if providerURL := viper.GetString("oidc_provider_url"); providerURL != "" && config.OIDCProviderURL == "" {
		config.OIDCProviderURL = providerURL
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(config *Config) error {
	fmt.Printf("Initializing logger with debug=%v\n", config.Debug)

	var logConfig *logging.Config
	if config.Debug {
		logConfig = &logging.Config{
			Level:            "debug",
			Development:      true,
			Encoding:         "console",
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		logConfig = &logging.Config{
			Level:            "info",
			Development:      false,
			Encoding:         "console",
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	}

	logger, err := logging.NewLogger(logConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		return err
	}
	defer logger.SafeSync()

	logger.Info("Logger initialized successfully")
	logger.Debug("Debug logging enabled", zap.Bool("debug", config.Debug))

	logger.Info("Starting Tailscale OIDC authentication via custom provider...",
		zap.String("version", version),
		zap.String("provider_url", config.OIDCProviderURL),
		zap.String("audience", config.TailscaleAudience))

	if config.OIDCProviderURL == "" {
		logger.Error("OIDC provider URL is required")
		return fmt.Errorf("OIDC provider URL must be specified via --oidc-provider-url flag or TAILSCALE_OIDC_PROVIDER_URL environment variable")
	}
	if config.TailscaleClientID == "" {
		logger.Error("Tailscale client ID is required")
		return fmt.Errorf("Tailscale client ID must be specified via --client-id flag or TAILSCALE_CLIENT_ID environment variable")
	}

	imdsClient := NewIMDSClient(logger)
	if imdsClient == nil {
		return fmt.Errorf("failed to create IMDS client")
	}
	metadata, err := imdsClient.GetInstanceMetadata()
	if err != nil {
		logger.Error("Failed to get instance metadata", zap.Error(err))
		return err
	}

	oidcClient := NewOIDCClient(config.OIDCProviderURL, logger)
	jwt, err := oidcClient.GetToken(metadata, config.TailscaleAudience)
	if err != nil {
		logger.Error("Failed to get JWT from OIDC provider", zap.Error(err))
		return err
	}

	if config.Debug {
		jwtPreview := jwt
		if len(jwt) > 50 {
			jwtPreview = jwt[:50] + "..."
		}
		logger.Debug("JWT received from OIDC provider", zap.String("jwt_preview", jwtPreview))
	}

	tailscaleClient := NewTailscaleClient(logger)
	accessToken, err := tailscaleClient.ExchangeToken(config.TailscaleClientID, jwt)
	if err != nil {
		logger.Error("Failed to exchange JWT for Tailscale access token", zap.Error(err))
		return err
	}

	authKey, err := tailscaleClient.CreateAuthKey(accessToken, config)
	if err != nil {
		logger.Error("Failed to create auth key", zap.Error(err))
		return err
	}

	if config.Export {
		fmt.Printf("TAILSCALE_API_TOKEN=%s\nTAILSCALE_AUTH_KEY=%s\n", accessToken, authKey)
		logger.Info("Exported Tailscale API token and auth key, exiting as requested by --export flag")
		return nil
	}

	localAPIClient := NewLocalAPIClient(logger)
	if err := localAPIClient.AuthenticateWithKey(authKey, config.Tags); err != nil {
		logger.Error("Failed to authenticate with local Tailscale daemon", zap.Error(err))
		return err
	}

	logger.Info("🎉 Successfully authenticated with Tailscale!",
		zap.Strings("tags", config.Tags),
		zap.Bool("ephemeral", config.Ephemeral))

	return nil
}
