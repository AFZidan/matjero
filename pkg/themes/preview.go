package themes

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PreviewClaims are the signed payload of a store-scoped theme preview token.
// The token grants short-lived, read-only access to a store's DRAFT theme
// configuration for preview rendering. It contains no secrets and must not be
// used as a general authentication mechanism.
type PreviewClaims struct {
	StoreID        string `json:"store_id"`
	InstallationID string `json:"installation_id"`
	DraftRevision  int    `json:"draft_revision"`
	Exp            int64  `json:"exp"`
}

// StorefrontPreviewTheme is the draft presentation contract a valid preview
// token may expose for the host-resolved store.
type StorefrontPreviewTheme struct {
	Key           string
	Version       string
	Configuration map[string]any
	DraftRevision int
}

// IssuePreviewToken creates a signed, short-lived preview token for the given
// store installation draft. The signing secret is server-side configured and is
// never embedded in the token.
func (s Service) IssuePreviewToken(storeID, installationID string, draftRevision int) (string, error) {
	// Fail closed: without a configured secret there is no way to sign a token
	// that cannot be forged, so no token is issued at all.
	if len(s.previewSecret) == 0 {
		return "", ErrPreviewNotConfigured
	}
	claims := PreviewClaims{
		StoreID:        storeID,
		InstallationID: installationID,
		DraftRevision:  draftRevision,
		Exp:            s.clock().Add(s.previewTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal preview claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := signPreview(s.previewSecret, encodedPayload)
	return encodedPayload + "." + sig, nil
}

// VerifyPreviewToken validates a preview token's signature and expiry, returning
// the claims. Callers must additionally compare StoreID/InstallationID against the
// requested context to ensure the token is not used for a different store.
func (s Service) VerifyPreviewToken(token string) (PreviewClaims, error) {
	var claims PreviewClaims
	// Fail closed: an unconfigured secret must never be treated as a valid
	// signing key, otherwise every token would verify against an empty HMAC.
	if len(s.previewSecret) == 0 {
		return claims, ErrPreviewNotConfigured
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, errors.New("malformed preview token")
	}
	expectedSig := signPreview(s.previewSecret, parts[0])
	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return claims, errors.New("invalid preview token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, fmt.Errorf("decode preview payload: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, fmt.Errorf("unmarshal preview claims: %w", err)
	}
	if s.clock().Unix() > claims.Exp {
		return claims, errors.New("preview token expired")
	}
	return claims, nil
}

func signPreview(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
