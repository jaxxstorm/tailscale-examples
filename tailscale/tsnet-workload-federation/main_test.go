package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExchangeJWTForAccessToken tests the JWT to access token exchange.
func TestExchangeJWTForAccessToken(t *testing.T) {
	const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	const testClientID = "test-client-id"
	const expectedAccessToken = "tskey-api-abc123def456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/oauth/token-exchange" {
			t.Errorf("Expected path /api/v2/oauth/token-exchange, got %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded" {
			t.Errorf("Expected Content-Type application/x-www-form-urlencoded, got %s", contentType)
		}

		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse form: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// oauth2 clientcredentials sends client_id in the form, not the EndpointParams
		clientID := r.Form.Get("client_id")
		if clientID == "" {
			clientID = testClientID // oauth2 might send it differently
		}

		jwtParam := r.Form.Get("jwt")
		if jwtParam != testJWT {
			t.Errorf("Expected jwt %s, got %s", testJWT, jwtParam)
		}

		response := OAuthTokenResponse{
			AccessToken: expectedAccessToken,
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Override the API base for testing
	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	accessToken, err := exchangeJWTForAccessToken(ctx, testClientID, testJWT)
	if err != nil {
		t.Fatalf("exchangeJWTForAccessToken failed: %v", err)
	}

	if accessToken != expectedAccessToken {
		t.Errorf("Expected access token %s, got %s", expectedAccessToken, accessToken)
	}
}

// TestExchangeJWTForAccessToken_InvalidJWT tests error handling for invalid JWT.
func TestExchangeJWTForAccessToken_InvalidJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid JWT token",
		})
	}))
	defer server.Close()

	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	_, err := exchangeJWTForAccessToken(ctx, "test-client-id", "invalid-jwt")
	if err == nil {
		t.Fatal("Expected error for invalid JWT, got nil")
	}

	if !strings.Contains(err.Error(), "token exchange failed") {
		t.Errorf("Expected error to contain 'token exchange failed', got: %v", err)
	}
}

// TestExchangeJWTForAccessToken_NoAccessToken tests when response has no access token.
func TestExchangeJWTForAccessToken_NoAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := OAuthTokenResponse{
			TokenType: "Bearer",
			ExpiresIn: 3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	_, err := exchangeJWTForAccessToken(ctx, "test-client-id", "test-jwt")
	if err == nil {
		t.Fatal("Expected error for missing access token, got nil")
	}

	// oauth2 library has its own error message format
	if !strings.Contains(err.Error(), "access_token") {
		t.Errorf("Expected error to contain 'access_token', got: %v", err)
	}
}

// TestCreateAuthKey tests creating a Tailscale auth key.
func TestCreateAuthKey(t *testing.T) {
	const testAccessToken = "tskey-api-test123"
	const expectedKey = "tskey-auth-xyz789"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/tailnet/-/keys" {
			t.Errorf("Expected path /api/v2/tailnet/-/keys, got %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + testAccessToken
		if authHeader != expectedAuth {
			t.Errorf("Expected Authorization %s, got %s", expectedAuth, authHeader)
		}

		var req AuthKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if !req.Capabilities.Devices.Create.Ephemeral {
			t.Error("Expected ephemeral to be true")
		}

		if !req.Capabilities.Devices.Create.Preauthorized {
			t.Error("Expected preauthorized to be true")
		}

		if len(req.Capabilities.Devices.Create.Tags) != 1 {
			t.Errorf("Expected 1 tag, got %d", len(req.Capabilities.Devices.Create.Tags))
		}

		response := AuthKeyResponse{
			Key: expectedKey,
			ID:  "key-123",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	authKey, err := createAuthKey(ctx, testAccessToken, "tag:test")
	if err != nil {
		t.Fatalf("createAuthKey failed: %v", err)
	}

	if authKey != expectedKey {
		t.Errorf("Expected auth key %s, got %s", expectedKey, authKey)
	}
}

// TestCreateAuthKey_ErrorResponse tests error handling when creating auth key fails.
func TestCreateAuthKey_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "Invalid tag format"}`))
	}))
	defer server.Close()

	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	_, err := createAuthKey(ctx, "test-token", "invalid-tag")
	if err == nil {
		t.Fatal("Expected error for bad request, got nil")
	}

	if !strings.Contains(err.Error(), "create auth key failed") {
		t.Errorf("Expected error to contain 'create auth key failed', got: %v", err)
	}
}

// TestCreateAuthKey_EmptyKey tests when response has empty key.
func TestCreateAuthKey_EmptyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthKeyResponse{
			ID: "key-123",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	origBase := tailscaleAPIBase
	tailscaleAPIBase = server.URL
	defer func() { tailscaleAPIBase = origBase }()

	ctx := context.Background()
	_, err := createAuthKey(ctx, "test-token", "tag:test")
	if err == nil {
		t.Fatal("Expected error for empty key, got nil")
	}

	if !strings.Contains(err.Error(), "empty key") {
		t.Errorf("Expected error to contain 'empty key', got: %v", err)
	}
}

// TestPrefix tests the prefix helper function.
func TestPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short string", "short", "short"},
		{"exact 10 chars", "exactly10c", "exactly10c"},
		{"long string", "this-is-a-very-long-string", "this-is-a-..."},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prefix(tt.input)
			if result != tt.expected {
				t.Errorf("prefix(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
