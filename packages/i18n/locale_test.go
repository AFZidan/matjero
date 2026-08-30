package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiatePrefersArabicWhenRequested(t *testing.T) {
	if got := Negotiate("ar-EG, en;q=0.8"); got != LocaleArabic {
		t.Fatalf("Negotiate returned %q", got)
	}
}

func TestMiddlewareSetsLocaleHeaders(t *testing.T) {
	handler := Middleware(Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != LocaleArabic {
			t.Fatalf("FromContext returned %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/?locale=ar", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if got := resp.Header().Get("Content-Language"); got != string(LocaleArabic) {
		t.Fatalf("Content-Language = %q", got)
	}
	if got := resp.Header().Get("X-Direction"); got != "rtl" {
		t.Fatalf("X-Direction = %q", got)
	}
}
