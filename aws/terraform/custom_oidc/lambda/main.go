package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	jwt "github.com/golang-jwt/jwt/v5"
)

type Config struct {
	SecretARN  string
	IssuerURL  string
	RolePrefix string

	AWSRegion  string
	AWSAccount string

	KeyID    string
	OIDCTags string // Optional comma‑separated list (e.g. "tag:aws,tag:prod") - must include tag // FIXME: validate the input
}

// AWS instance‑identity doc shape
// (see https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html)

type InstanceIdentity struct {
	AccountID        string    `json:"accountId"`
	Architecture     string    `json:"architecture"`
	AvailabilityZone string    `json:"availabilityZone"`
	ImageID          string    `json:"imageId"`
	InstanceID       string    `json:"instanceId"`
	InstanceType     string    `json:"instanceType"`
	PendingTime      time.Time `json:"pendingTime"`
	PrivateIP        string    `json:"privateIp"`
	Region           string    `json:"region"`
	Version          string    `json:"version"`
}

type TokenRequest struct {
	InstanceIdentity string `json:"instance_identity"`
	Signature        string `json:"signature"`
	RoleARN          string `json:"role_arn"`
	Audience         string `json:"audience"`
}

// Claims mirrors what Tailscale expects for Workload IDs.
// RegisteredClaims is embedded so the JWT lib can handle exp/iat/etc.

type Claims struct {
	jwt.RegisteredClaims

	Audience         string            `json:"aud"`
	Tags             []string          `json:"tags,omitempty"`
	AWSAccountID     string            `json:"aws:account_id"`
	AWSRegion        string            `json:"aws:region"`
	AWSInstanceID    string            `json:"aws:instance_id"`
	AWSRoleName      string            `json:"aws:role_name"`
	AWSRoleARN       string            `json:"aws:role_arn"`
	AWSInstanceIdent *InstanceIdentity `json:"aws:instance_identity"`
}

// Minimal OAuth/OIDC wire responses

type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`

	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

var (
	cfg           Config
	secretsClient *secretsmanager.Client
	privateKey    *rsa.PrivateKey
	keyID         string
)

func init() {
	cfg = Config{
		SecretARN:  os.Getenv("SECRET_ARN"), // contains the private key to sign JWTs
		IssuerURL:  os.Getenv("ISSUER_URL"),
		RolePrefix: os.Getenv("ROLE_PREFIX"),

		AWSRegion:  os.Getenv("OIDC_AWS_REGION"),
		AWSAccount: os.Getenv("OIDC_AWS_ACCOUNT"),

		KeyID:    os.Getenv("KEY_ID"),
		OIDCTags: os.Getenv("OIDC_TAGS"),
	}

	if cfg.SecretARN == "" || cfg.IssuerURL == "" || cfg.RolePrefix == "" {
		log.Fatal("SECRET_ARN, ISSUER_URL and ROLE_PREFIX are required")
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("unable to load AWS config: %v", err)
	}
	secretsClient = secretsmanager.NewFromConfig(awsCfg)

	if err := loadSigningKey(); err != nil {
		log.Fatalf("unable to load signing key: %v", err)
	}
}

// SecretData represents the structure of the secret stored in AWS Secrets Manager.
// FIXME: we should store this in ACM?
type SecretData struct {
	PrivateKey string `json:"private_key"`
	KeyID      string `json:"key_id"`
}

func loadSigningKey() error {
	sec, err := secretsClient.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(cfg.SecretARN),
	})
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}

	var s SecretData
	if err := json.Unmarshal([]byte(*sec.SecretString), &s); err != nil {
		return fmt.Errorf("parse secret JSON: %w", err)
	}

	blk, _ := pem.Decode([]byte(s.PrivateKey))
	if blk == nil {
		return fmt.Errorf("decode PEM: no block found")
	}

	key, err := x509.ParsePKCS1PrivateKey(blk.Bytes)
	if err != nil {
		return fmt.Errorf("parse RSA key: %w", err)
	}

	privateKey = key
	keyID = s.KeyID
	return nil
}

func verifyInstanceIdentity(raw, sig string) (*InstanceIdentity, error) {
	var ident InstanceIdentity
	if err := json.Unmarshal([]byte(raw), &ident); err != nil {
		return nil, fmt.Errorf("invalid instance identity JSON: %w", err)
	}

	if ident.AccountID != cfg.AWSAccount {
		return nil, fmt.Errorf("account mismatch: got %s", ident.AccountID)
	}
	if ident.Region != cfg.AWSRegion {
		return nil, fmt.Errorf("region mismatch: got %s", ident.Region)
	}
	// allow an identity document up to 24 h old – an EC2 launch day is usually fine
	if time.Since(ident.PendingTime) > 24*time.Hour {
		return nil, fmt.Errorf("instance identity >24h old")
	}
	if sig == "" {
		return nil, fmt.Errorf("missing signature")
	}
	// FIXME: verify signature with AWS cert chain
	return &ident, nil
}

func createJWT(ident *InstanceIdentity, roleARN, audience string) (string, error) {
	parts := strings.Split(roleARN, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid role ARN")
	}
	roleName := parts[len(parts)-1]

	if !strings.HasPrefix(roleName, cfg.RolePrefix) {
		return "", fmt.Errorf("role %s lacks required prefix %s", roleName, cfg.RolePrefix)
	}

	now := time.Now()

	tags := []string{}
	if cfg.OIDCTags != "" {
		for _, t := range strings.Split(cfg.OIDCTags, ",") {
			if tt := strings.TrimSpace(t); tt != "" {
				tags = append(tags, tt)
			}
		}
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.IssuerURL,
			Subject:   fmt.Sprintf("system:role:%s:%s", ident.AccountID, roleName),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        fmt.Sprintf("%s-%d", ident.InstanceID, now.Unix()),
			// we intentionally omit NotBefore to avoid clock‑skew rejections
		},
		Audience:         audience,
		Tags:             tags,
		AWSAccountID:     ident.AccountID,
		AWSRegion:        ident.Region,
		AWSInstanceID:    ident.InstanceID,
		AWSRoleName:      roleName,
		AWSRoleARN:       roleARN,
		AWSInstanceIdent: ident,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(privateKey)
}

// serves the required endpoints for OIDC discovery and JWKS
func handleOIDCDiscovery() (events.APIGatewayProxyResponse, error) {
	disc := OIDCDiscovery{
		Issuer:                           cfg.IssuerURL,
		AuthorizationEndpoint:            cfg.IssuerURL + "/auth",
		TokenEndpoint:                    cfg.IssuerURL + "/token",
		JWKSURI:                          cfg.IssuerURL + "/.well-known/jwks.json",
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid"},
		ClaimsSupported:                  []string{"sub", "iss", "aud", "exp", "iat", "jti", "tags", "aws:account_id", "aws:region", "aws:instance_id", "aws:role_name"},
	}

	b, _ := json.Marshal(disc)
	return jsonResp(200, b)
}

func handleJWKS() (events.APIGatewayProxyResponse, error) {
	pub := &privateKey.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	jwk := JWK{Kty: "RSA", Use: "sig", Kid: keyID, Alg: "RS256", N: n, E: e}

	b, _ := json.Marshal(JWKS{Keys: []JWK{jwk}})
	return jsonResp(200, b)
}

func handleToken(r events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var tr TokenRequest

	ct := strings.ToLower(r.Headers["content-type"])
	switch {
	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		vals, err := url.ParseQuery(r.Body)
		if err != nil {
			log.Printf("[handleToken] Failed to parse form data: %v", err)
			return errResp(400, "invalid_request", "cannot parse form data")
		}
		tr.InstanceIdentity = vals.Get("instance_identity")
		tr.Signature = vals.Get("signature")
		tr.RoleARN = vals.Get("role_arn")
		tr.Audience = vals.Get("audience")
	default:
		if err := json.Unmarshal([]byte(r.Body), &tr); err != nil {
			log.Printf("[handleToken] Failed to parse JSON body: %v", err)
			log.Printf("[handleToken] Body: %s", r.Body)
			return errResp(400, "invalid_request", "cannot parse JSON body")
		}
	}

	// FIXME: should we set a value here, or make it required?
	if tr.Audience == "" {
		tr.Audience = "tailscale"
	}

	if tr.InstanceIdentity == "" || tr.Signature == "" || tr.RoleARN == "" {
		log.Printf("[handleToken] Missing required fields: instance_identity='%s', signature='%s', role_arn='%s'", tr.InstanceIdentity, tr.Signature, tr.RoleARN)
		return errResp(400, "invalid_request", "missing required fields")
	}

	ident, err := verifyInstanceIdentity(tr.InstanceIdentity, tr.Signature)
	if err != nil {
		log.Printf("[handleToken] verifyInstanceIdentity error: %v", err)
		log.Printf("[handleToken] instance_identity: %s", tr.InstanceIdentity)
		log.Printf("[handleToken] signature: %s", tr.Signature)
		return errResp(403, "invalid_identity", err.Error())
	}

	jwtStr, err := createJWT(ident, tr.RoleARN, tr.Audience)
	if err != nil {
		log.Printf("[handleToken] createJWT error: %v", err)
		log.Printf("[handleToken] ident: %+v", ident)
		log.Printf("[handleToken] role_arn: %s", tr.RoleARN)
		log.Printf("[handleToken] audience: %s", tr.Audience)
		return errResp(500, "server_error", err.Error())
	}

	out, err := json.Marshal(TokenResponse{
		AccessToken: jwtStr,
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	})
	if err != nil {
		log.Printf("[handleToken] Failed to marshal TokenResponse: %v", err)
		return errResp(500, "server_error", "failed to marshal token response")
	}
	return jsonResp(200, out)
}

func jsonResp(code int, body []byte) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: code,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "GET,POST,OPTIONS",
			"Access-Control-Allow-Headers": "Content-Type",
		},
		Body: string(body),
	}, nil
}

func errResp(code int, errCode, desc string) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(ErrorResponse{Error: errCode, ErrorDescription: desc})
	return jsonResp(code, b)
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("%s %s", req.HTTPMethod, req.Path)

	if req.HTTPMethod == "OPTIONS" { // CORS preflight
		return jsonResp(200, []byte(`{"status":"ok"}`))
	}

	switch {
	case req.Path == "/.well-known/openid-configuration" && req.HTTPMethod == "GET":
		return handleOIDCDiscovery()
	case req.Path == "/.well-known/openid_configuration" && req.HTTPMethod == "GET":
		return handleOIDCDiscovery()
	case req.Path == "/.well-known/jwks.json" && req.HTTPMethod == "GET":
		return handleJWKS()
	case req.Path == "/token" && req.HTTPMethod == "POST":
		return handleToken(req)
	default:
		return errResp(404, "not_found", "endpoint not found")
	}
}

func main() { lambda.Start(handler) }
