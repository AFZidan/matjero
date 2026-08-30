package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOIDCVerifierAcceptsZitadelStyleToken(t *testing.T) {
	issuer := newOIDCIssuer(t)

	verifier, err := NewOIDCVerifier(context.Background(), Config{
		IssuerURL: issuer.URL,
		Audience:  "admin-api",
	})
	if err != nil {
		t.Fatalf("NewOIDCVerifier returned error: %v", err)
	}

	token := signJWT(t, issuer.privateKey, issuer.URL, "admin-api", map[string]any{
		"email":              "admin@example.test",
		"preferred_username": "platform-admin",
		"locale":             "ar",
		"urn:zitadel:iam:org:project:roles": map[string]any{
			"platform_admin": map[string]any{"project-1": "Matjero"},
		},
	})

	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}

	if principal.Subject != "user-123" {
		t.Fatalf("Subject = %q", principal.Subject)
	}
	if !principal.HasRole(RolePlatformAdmin) {
		t.Fatal("expected platform_admin role")
	}
	if principal.Locale != "ar" {
		t.Fatalf("Locale = %q", principal.Locale)
	}
}

func TestBearerTokenRequiresAuthorizationHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if _, err := BearerToken(req); err == nil {
		t.Fatal("expected missing bearer token error")
	}
}

type oidcIssuer struct {
	*httptest.Server
	privateKey *rsa.PrivateKey
	keyID      string
}

func newOIDCIssuer(t *testing.T) oidcIssuer {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}

	issuer := oidcIssuer{
		privateKey: privateKey,
		keyID:      "test-key",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   issuer.URL,
			"jwks_uri": issuer.URL + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{jwkForRSA(&privateKey.PublicKey, issuer.keyID)},
		})
	})

	server := httptest.NewServer(mux)
	issuer.Server = server
	t.Cleanup(server.Close)

	return issuer
}

func jwkForRSA(publicKey *rsa.PublicKey, keyID string) map[string]any {
	return map[string]any{
		"kty": "RSA",
		"kid": keyID,
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
	}
}

func signJWT(t *testing.T, privateKey *rsa.PrivateKey, issuer, audience string, claims map[string]any) string {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)
	header := map[string]any{
		"alg": "RS256",
		"kid": "test-key",
		"typ": "JWT",
	}
	payload := map[string]any{
		"iss": issuer,
		"sub": "user-123",
		"aud": []string{audience},
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
	}
	for key, value := range claims {
		payload[key] = value
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}
