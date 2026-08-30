package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type Principal struct {
	Subject           string         `json:"subject"`
	Issuer            string         `json:"issuer"`
	Audience          []string       `json:"audience"`
	Email             string         `json:"email,omitempty"`
	PreferredUsername string         `json:"preferred_username,omitempty"`
	Locale            string         `json:"locale,omitempty"`
	Roles             []string       `json:"roles"`
	Claims            map[string]any `json:"claims,omitempty"`
	IssuedAt          time.Time      `json:"issued_at"`
	ExpiresAt         time.Time      `json:"expires_at"`
}

type contextKey string

const principalKey contextKey = "principal"

const (
	RolePlatformAdmin   = "platform_admin"
	RoleSellerOwner     = "seller_owner"
	RoleSellerManager   = "seller_manager"
	RoleSellerStaff     = "seller_staff"
	RoleSupplierOwner   = "supplier_owner"
	RoleSupplierManager = "supplier_manager"
	RoleSupplierStaff   = "supplier_staff"
)

var ErrMissingBearerToken = ErrUnauthorized("missing bearer token")

func BearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", ErrMissingBearerToken
	}

	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || token == "" {
		return "", ErrMissingBearerToken
	}

	return token, nil
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}

func (p Principal) HasRole(role string) bool {
	for _, existing := range p.Roles {
		if existing == role {
			return true
		}
	}
	return false
}

func (p Principal) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}
