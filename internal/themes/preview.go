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

// IssuePreviewToken creates a signed, short-lived preview token for the given
// store installation draft. The signing secret is server-side configured and is
// never embedded in the token.
func (s Service) IssuePreviewToken(storeID, installationID string, draftRevision int) (string, error) {
	if len(s.previewSecret) == 0 {
		return "", errors.New("theme preview signing secret is not configured")
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
	if len(s.previewSecret) == 0 {
		return claims, errors.New("theme preview signing secret is not configured")
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
