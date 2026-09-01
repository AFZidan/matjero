package coreapi

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/matjeroapps/core/pkg/openapi"
)

// routerRoutes walks the chi tree and returns every registered method+path.
//
// chi reports a mounted sub-router's routes relative to the mount point, so the
// walker carries the prefix down and strips the mount wildcard. Catch-all mount
// handlers ("*", OPTIONS) are not real routes and are skipped.
func routerRoutes(t *testing.T) []string {
	t.Helper()

	var routes []string

	var collect func(prefix string, routeList []chi.Route)
	collect = func(prefix string, routeList []chi.Route) {
		for _, route := range routeList {
			if route.SubRoutes != nil {
				collect(prefix+strings.TrimSuffix(route.Pattern, "/*"), route.SubRoutes.Routes())
				continue
			}
			if route.Pattern == "" || route.Pattern == "/" {
				continue
			}
			for method := range route.Handlers {
				if method == "*" || method == http.MethodOptions {
					continue
				}
				routes = append(routes, method+" "+prefix+route.Pattern)
			}
		}
	}
	collect("", NewRouter(Dependencies{}).Routes())

	sort.Strings(routes)
	return routes
}

func specRoutes(t *testing.T) []string {
	t.Helper()

	var routes []string
	for _, route := range internalRoutes() {
		routes = append(routes, route.Method+" "+route.Path)
	}
	sort.Strings(routes)
	return routes
}

// TestSpecMatchesRouter is the drift guard: a route added to the router without
// a matching declaration (or declared but never registered) fails the build.
// The committed docs/api/internal/openapi.json is generated from the same
// declarations, so this keeps the document honest too.
func TestSpecMatchesRouter(t *testing.T) {
	got := routerRoutes(t)
	want := specRoutes(t)

	if len(got) != len(want) {
		t.Fatalf("route count mismatch: router has %d, spec declares %d\nrouter: %v\nspec:   %v",
			len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("route %d: router has %q, spec declares %q", i, got[i], want[i])
		}
	}
}

func TestInternalSpecBuildsAndValidates(t *testing.T) {
	doc, err := BuildInternalSpec()
	if err != nil {
		t.Fatalf("build internal spec: %v", err)
	}
	if err := openapi.ValidateDocument(doc); err != nil {
		t.Fatalf("validate internal spec: %v", err)
	}
	if doc.Info.Title != InternalSpecTitle {
		t.Errorf("title = %q, want %q", doc.Info.Title, InternalSpecTitle)
	}
	if len(doc.Paths.Map()) == 0 {
		t.Error("spec declares no paths")
	}
}

// The internal API must not be documented as an OIDC-authenticated surface: it
// uses per-caller service tokens, and a reader must not mistake it for a
// customer-facing API.
func TestInternalSpecDescribesServiceAuth(t *testing.T) {
	doc, err := BuildInternalSpec()
	if err != nil {
		t.Fatalf("build internal spec: %v", err)
	}

	scheme, ok := doc.Components.SecuritySchemes["bearerAuth"]
	if !ok || scheme.Value == nil {
		t.Fatal("expected a bearerAuth security scheme")
	}
	if scheme.Value.Description != "Core internal per-caller service token" {
		t.Errorf("scheme description = %q, want the service-token description", scheme.Value.Description)
	}
	if doc.Info.Description == "" {
		t.Error("expected the internal API description to document its audience and security posture")
	}
}

// Every declared route must carry the unauthorized response, because the whole
// namespace sits behind service authentication.
func TestEveryRouteDeclaresUnauthorized(t *testing.T) {
	for _, route := range internalRoutes() {
		found := false
		for _, response := range route.Responses {
			if response.Status == http.StatusUnauthorized {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s %s does not declare a 401 response", route.Method, route.Path)
		}
	}
}
