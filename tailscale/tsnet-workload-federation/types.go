package main

// OAuthTokenResponse represents the response from the WIF token-exchange endpoint.
type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// AuthKeyRequest represents the payload to create a Tailscale auth key.
type AuthKeyRequest struct {
	Capabilities  AuthKeyCapabilities `json:"capabilities"`
	ExpirySeconds int                 `json:"expirySeconds,omitempty"`
	Description   string              `json:"description,omitempty"`
}

// AuthKeyCapabilities defines the capabilities for the auth key.
type AuthKeyCapabilities struct {
	Devices DeviceCapabilities `json:"devices"`
}

// DeviceCapabilities defines device-specific capabilities.
type DeviceCapabilities struct {
	Create DeviceCreateCapabilities `json:"create"`
}

// DeviceCreateCapabilities defines capabilities for creating devices.
type DeviceCreateCapabilities struct {
	Reusable      bool     `json:"reusable"`
	Ephemeral     bool     `json:"ephemeral"`
	Preauthorized bool     `json:"preauthorized"`
	Tags          []string `json:"tags,omitempty"`
}

// AuthKeyResponse represents the response from creating an auth key.
type AuthKeyResponse struct {
	Key string `json:"key"`
	ID  string `json:"id,omitempty"`
}

// Config holds the application configuration.
type Config struct {
	ClientID string `kong:"required,env='TS_WIF_CLIENT_ID',help='Tailscale WIF client ID'"`
	JWT      string `kong:"required,env='TS_WIF_JWT',help='JWT token for workload identity federation'"`
	Tag      string `kong:"default='tag:tsnet-wif-demo',env='TS_TAG',help='Tailscale tag for the device'"`
	Hostname string `kong:"default='tsnet-wif-demo',env='TS_HOSTNAME',help='Hostname for the tsnet server'"`
	Port     string `kong:"default=':8080',env='TS_PORT',help='Port to listen on'"`
}
