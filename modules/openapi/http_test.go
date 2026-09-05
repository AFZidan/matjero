package openapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Adapted from the monorepo's internal/openapi/spec_test.go
// TestDocsRouterEnabledDisabled. The original built the admin specification to
// obtain spec bytes; the admin specification now lives in the admin repository,
// so this test builds an equivalent minimal document instead. The router
// behaviour under test is unchanged.
func TestDocsRouterEnabledDisabled(t *testing.T) {
	spec, err := BuildDocument(DocumentSpec{
		Title:         "Matjero Core Docs Router Test",
		Description:   "Minimal document used to exercise the docs router.",
		Authenticated: true,
		Tags:          CommonTags(),
		Routes:        ActorRoutes(true),
	})
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	specBytes, err := MarshalDocument(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	enabled := NewRouter(RouterConfig{Enabled: true, SpecBytes: specBytes})
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	resp := httptest.NewRecorder()
	enabled.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from openapi.json, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "\"openapi\"") {
		t.Fatalf("openapi.json response did not look like a spec")
	}

	req = httptest.NewRequest(http.MethodGet, "/docs", nil)
	resp = httptest.NewRecorder()
	enabled.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from docs, got %d", resp.Code)
	}
	if resp.Body.Len() == 0 {
		t.Fatalf("expected docs body")
	}

	disabled := NewRouter(RouterConfig{Enabled: false, SpecBytes: specBytes})
	req = httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	resp = httptest.NewRecorder()
	disabled.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when docs are disabled, got %d", resp.Code)
	}
}

func TestActorRoutesAuthToggle(t *testing.T) {
	authenticated := ActorRoutes(true)
	public := ActorRoutes(false)
	if len(authenticated) != len(public) {
		t.Fatalf("actor route count differs between auth modes")
	}
	for i := range authenticated {
		if !authenticated[i].Auth {
			t.Fatalf("route %s should require auth", authenticated[i].Path)
		}
		if public[i].Auth {
			t.Fatalf("route %s should be public", public[i].Path)
		}
	}
}
