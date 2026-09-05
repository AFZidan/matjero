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
	if cfg.OrderConfirmationDuration != 15*time.Minute {
		t.Fatalf("OrderConfirmationDuration = %s, want 15m", cfg.OrderConfirmationDuration)
	}
}

func TestLoadCustomOrderConfirmationDuration(t *testing.T) {
	t.Setenv("ORDER_CONFIRMATION_DURATION", "45m")

	cfg, err := Load("admin-api")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.OrderConfirmationDuration != 45*time.Minute {
		t.Fatalf("OrderConfirmationDuration = %s, want 45m", cfg.OrderConfirmationDuration)
	}
}

func TestLoadRejectsInvalidOrderConfirmationDuration(t *testing.T) {
	invalidValues := []string{"not-a-duration", "0", "-5m", "0s"}
	for _, val := range invalidValues {
		t.Run(val, func(t *testing.T) {
			t.Setenv("ORDER_CONFIRMATION_DURATION", val)
			if _, err := Load("admin-api"); err == nil {
				t.Fatalf("expected error for ORDER_CONFIRMATION_DURATION=%q, got nil", val)
			}
		})
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
