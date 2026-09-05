package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "")

	cfg, err := Load("admin-api")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ServiceName != "admin-api" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.ZitadelAudience != "admin-api" {
		t.Fatalf("ZitadelAudience = %q", cfg.ZitadelAudience)
	}
	if cfg.CheckoutSessionLifetime != 30*time.Minute {
		t.Fatalf("CheckoutSessionLifetime = %s", cfg.CheckoutSessionLifetime)
	}
}

func TestLoadRejectsInvalidCheckoutSessionLifetime(t *testing.T) {
	t.Setenv("CHECKOUT_SESSION_LIFETIME", "not-a-duration")

	if _, err := Load("admin-api"); err == nil {
		t.Fatal("expected invalid Checkout Session lifetime error")
	}
}

func TestLoadRejectsInvalidShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "nope")

	if _, err := Load("admin-api"); err == nil {
		t.Fatal("expected invalid timeout error")
	}
}
