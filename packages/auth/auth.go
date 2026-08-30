package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type Principal struct {
	Subject string
	Roles   []string
}

type contextKey string

const principalKey contextKey = "principal"

var ErrMissingBearerToken = errors.New("missing bearer token")

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
