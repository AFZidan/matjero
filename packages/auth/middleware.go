package auth

import (
	"encoding/json"
	"net/http"
)

func Middleware(verifier Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := BearerToken(r)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, ErrorCodeUnauthorized, err.Error())
				return
			}

			principal, err := verifier.Verify(r.Context(), token)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, ErrorCodeUnauthorized, err.Error())
				return
			}

			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}

func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, ErrorCodeUnauthorized, "missing authenticated principal")
				return
			}
			if !principal.HasAnyRole(roles...) {
				writeAuthError(w, http.StatusForbidden, ErrorCodeForbidden, "insufficient role")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequireAll(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, ErrorCodeUnauthorized, "missing authenticated principal")
				return
			}
			for _, role := range roles {
				if !principal.HasRole(role) {
					writeAuthError(w, http.StatusForbidden, ErrorCodeForbidden, "insufficient role")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func PrincipalOrNil(r *http.Request) *Principal {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		return nil
	}
	return &principal
}

func writeAuthError(w http.ResponseWriter, status int, code ErrorCode, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="matjero"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    string(code),
			"message": message,
		},
	})
}
