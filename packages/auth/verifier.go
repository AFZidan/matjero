package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Verifier interface {
	Verify(ctx context.Context, token string) (Principal, error)
}

type OIDCVerifier struct {
	verifier   *oidc.IDTokenVerifier
	rolesClaim string
	issuer     string
}

func NewOIDCVerifier(ctx context.Context, cfg Config) (*OIDCVerifier, error) {
	issuer := NormalizeIssuer(cfg.IssuerURL)
	if issuer == "" {
		return nil, fmt.Errorf("issuer url is required")
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("audience is required")
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover oidc provider: %w", err)
	}

	return &OIDCVerifier{
		verifier:   provider.Verifier(&oidc.Config{ClientID: cfg.Audience}),
		rolesClaim: firstNonEmpty(cfg.RolesClaim, DefaultRolesClaim()),
		issuer:     issuer,
	}, nil
}

func (v *OIDCVerifier) Verify(ctx context.Context, token string) (Principal, error) {
	if v == nil || v.verifier == nil {
		return Principal{}, fmt.Errorf("verifier is not configured")
	}

	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return Principal{}, WrapVerificationError(err)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Principal{}, fmt.Errorf("decode oidc claims: %w", err)
	}

	principal := Principal{
		Subject:  idToken.Subject,
		Issuer:   idToken.Issuer,
		Audience: append([]string(nil), idToken.Audience...),
		Roles:    extractRoles(claims, v.rolesClaim),
		Claims:   claims,
	}

	if email, ok := claims["email"].(string); ok {
		principal.Email = email
	}
	if username, ok := claims["preferred_username"].(string); ok {
		principal.PreferredUsername = username
	}
	if locale, ok := claims["locale"].(string); ok {
		principal.Locale = locale
	}

	principal.IssuedAt = idToken.IssuedAt
	principal.ExpiresAt = idToken.Expiry

	if principal.Subject == "" {
		return Principal{}, fmt.Errorf("token subject is empty")
	}
	if principal.Issuer != v.issuer {
		return Principal{}, fmt.Errorf("token issuer %q does not match configured issuer %q", principal.Issuer, v.issuer)
	}
	if len(principal.Audience) == 0 {
		return Principal{}, fmt.Errorf("token audience is empty")
	}

	return principal, nil
}

func extractRoles(claims map[string]any, rolesClaim string) []string {
	raw, ok := claims[rolesClaim]
	if !ok {
		return nil
	}

	roles := make([]string, 0, 8)
	switch typed := raw.(type) {
	case map[string]any:
		for role := range typed {
			roles = append(roles, role)
		}
	case map[string]string:
		for role := range typed {
			roles = append(roles, role)
		}
	}

	return uniqueStrings(roles)
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
