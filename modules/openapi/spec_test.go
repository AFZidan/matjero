package openapi

import (
	"net/http"
	"testing"
)

// A response header that carries part of the contract must reach the generated
// document, otherwise a client integrator has no way to learn it exists.
func TestResponseHeadersAreDocumented(t *testing.T) {
	doc, err := BuildDocument(DocumentSpec{
		Title:       "Matjero Response Header Test",
		Description: "Minimal document used to exercise response header generation.",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Path: "/v1/thing", OperationID: "getThing",
			Summary: "Get a thing",
			Responses: []ResponseSpec{{
				Status:      http.StatusOK,
				Description: "A thing",
				Body:        struct{}{},
				Headers: []HeaderSpec{{
					Name:        "X-Generation",
					Description: "Opaque generation of the thing",
					Schema:      int64(0),
				}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("build document: %v", err)
	}

	response := doc.Paths.Find("/v1/thing").Get.Responses.Status(http.StatusOK)
	if response == nil || response.Value == nil {
		t.Fatal("expected a 200 response")
	}
	header, ok := response.Value.Headers["X-Generation"]
	if !ok || header.Value == nil {
		t.Fatalf("expected the response header to be documented, got %+v", response.Value.Headers)
	}
	if header.Value.Description != "Opaque generation of the thing" {
		t.Errorf("header description = %q", header.Value.Description)
	}
	if header.Value.Schema == nil || header.Value.Schema.Value.Format != "int64" {
		t.Errorf("header schema = %+v, want an int64", header.Value.Schema)
	}
}

// A response without headers must not gain an empty headers object.
func TestResponseWithoutHeadersStaysBare(t *testing.T) {
	doc, err := BuildDocument(DocumentSpec{
		Title:       "Matjero Bare Response Test",
		Description: "Minimal document used to exercise header-free responses.",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Path: "/v1/thing", OperationID: "getThing",
			Summary:   "Get a thing",
			Responses: []ResponseSpec{OKResponse("A thing", struct{}{})},
		}},
	})
	if err != nil {
		t.Fatalf("build document: %v", err)
	}

	response := doc.Paths.Find("/v1/thing").Get.Responses.Status(http.StatusOK)
	if response.Value.Headers != nil {
		t.Fatalf("expected no headers, got %+v", response.Value.Headers)
	}
}
