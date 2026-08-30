package openapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestBuildDocumentsValidate(t *testing.T) {
	specs := []struct {
		name  string
		build func() (*openapi3.T, error)
	}{
		{name: "admin", build: BuildAdminSpec},
		{name: "seller", build: BuildSellerSpec},
		{name: "supplier", build: BuildSupplierSpec},
		{name: "storefront", build: BuildStorefrontSpec},
	}

	for _, tc := range specs {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := tc.build()
			if err != nil {
				t.Fatalf("build spec: %v", err)
			}
			if err := ValidateDocument(spec); err != nil {
				t.Fatalf("validate spec: %v", err)
			}
		})
	}
}

func TestBuildDocumentsDeterministic(t *testing.T) {
	specs := []struct {
		name  string
		build func() (*openapi3.T, error)
	}{
		{name: "admin", build: BuildAdminSpec},
		{name: "seller", build: BuildSellerSpec},
		{name: "supplier", build: BuildSupplierSpec},
		{name: "storefront", build: BuildStorefrontSpec},
	}

	for _, tc := range specs {
		t.Run(tc.name, func(t *testing.T) {
			first, err := tc.build()
			if err != nil {
				t.Fatalf("build first spec: %v", err)
			}
			firstBytes, err := MarshalDocument(first)
			if err != nil {
				t.Fatalf("marshal first spec: %v", err)
			}

			second, err := tc.build()
			if err != nil {
				t.Fatalf("build second spec: %v", err)
			}
			secondBytes, err := MarshalDocument(second)
			if err != nil {
				t.Fatalf("marshal second spec: %v", err)
			}

			if string(firstBytes) != string(secondBytes) {
				t.Fatalf("spec generation is not deterministic")
			}
		})
	}
}

func TestSecuritySchemes(t *testing.T) {
	authSpecs := []struct {
		name  string
		build func() (*openapi3.T, error)
	}{
		{name: "admin", build: BuildAdminSpec},
		{name: "seller", build: BuildSellerSpec},
		{name: "supplier", build: BuildSupplierSpec},
	}

	for _, tc := range authSpecs {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := tc.build()
			if err != nil {
				t.Fatalf("build spec: %v", err)
			}
			if spec.Components == nil || spec.Components.SecuritySchemes == nil {
				t.Fatalf("missing security schemes")
			}
			if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
				t.Fatalf("bearerAuth scheme missing")
			}
		})
	}

	publicSpec, err := BuildStorefrontSpec()
	if err != nil {
		t.Fatalf("build storefront spec: %v", err)
	}
	if publicSpec.Components != nil && len(publicSpec.Components.SecuritySchemes) != 0 {
		t.Fatalf("storefront spec should not declare bearer auth")
	}
}

func TestImportantRoutes(t *testing.T) {
	adminSpec, err := BuildAdminSpec()
	if err != nil {
		t.Fatalf("build admin spec: %v", err)
	}
	adminPath := adminSpec.Paths.Value("/v1/admin/overview")
	if adminPath == nil || adminPath.Get == nil {
		t.Fatalf("admin overview route missing")
	}
	if !containsTag(adminPath.Get.Tags, "Audit") {
		t.Fatalf("admin overview route missing Audit tag")
	}

	sellerSpec, err := BuildSellerSpec()
	if err != nil {
		t.Fatalf("build seller spec: %v", err)
	}
	sellerPath := sellerSpec.Paths.Value("/v1/seller/catalog/offers")
	if sellerPath == nil || sellerPath.Get == nil {
		t.Fatalf("seller catalog route missing")
	}
	if !containsTag(sellerPath.Get.Tags, "Catalog") {
		t.Fatalf("seller catalog route missing Catalog tag")
	}

	storefrontSpec, err := BuildStorefrontSpec()
	if err != nil {
		t.Fatalf("build storefront spec: %v", err)
	}
	publicPath := storefrontSpec.Paths.Value("/v1/bootstrap")
	if publicPath == nil || publicPath.Get == nil {
		t.Fatalf("storefront bootstrap route missing")
	}
	if publicPath.Get.Security != nil {
		t.Fatalf("storefront bootstrap should be public")
	}
}

func TestDocsRouterEnabledDisabled(t *testing.T) {
	spec, err := BuildAdminSpec()
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

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
