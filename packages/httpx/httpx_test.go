package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthzIncludesRequestAndCorrelationHeaders(t *testing.T) {
	router := NewRouter(App{
		Config: Config{
			ServiceName:     "admin-api",
			Environment:     "test",
			Addr:            ":0",
			ShutdownTimeout: time.Second,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(HeaderCorrelationID, "corr-123")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if resp.Header().Get(HeaderRequestID) == "" {
		t.Fatal("missing request id header")
	}
	if got := resp.Header().Get(HeaderCorrelationID); got != "corr-123" {
		t.Fatalf("correlation id = %q", got)
	}
}

func TestReadyzUsesReadinessCheck(t *testing.T) {
	router := NewRouter(App{
		Config: Config{ServiceName: "admin-api"},
		Ready: func(context.Context) error {
			return context.Canceled
		},
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.Code)
	}
}
