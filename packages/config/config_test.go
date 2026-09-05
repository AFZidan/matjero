package config_test

import (
	"testing"
	"time"

	"github.com/matjeroapps/core/packages/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("expected clean config load, got: %v", err)
	}

	if cfg.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got %s", cfg.ServiceName)
	}
	if cfg.CheckoutSessionLifetime != 30*time.Minute {
		t.Errorf("expected CheckoutSessionLifetime 30m, got %v", cfg.CheckoutSessionLifetime)
	}
	if cfg.OrderConfirmationDuration != 15*time.Minute {
		t.Errorf("expected OrderConfirmationDuration 15m, got %v", cfg.OrderConfirmationDuration)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("expected ShutdownTimeout 10s, got %v", cfg.ShutdownTimeout)
	}
}

func TestLoadCustomOrderConfirmationDuration(t *testing.T) {
	t.Setenv("ORDER_CONFIRMATION_DURATION", "45m")

	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("expected clean config load, got: %v", err)
	}

	if cfg.OrderConfirmationDuration != 45*time.Minute {
		t.Errorf("expected OrderConfirmationDuration 45m, got %v", cfg.OrderConfirmationDuration)
	}
}

func TestLoadRejectsInvalidOrderConfirmationDuration(t *testing.T) {
	invalidValues := []string{"not-a-duration", "0", "-5m", "0s"}
	for _, val := range invalidValues {
		t.Setenv("ORDER_CONFIRMATION_DURATION", val)
		_, err := config.Load("test-service")
		if err == nil {
			t.Fatalf("expected error for ORDER_CONFIRMATION_DURATION=%q, got nil", val)
		}
	}
}

func TestLoadRejectsInvalidCheckoutSessionLifetime(t *testing.T) {
	invalidValues := []string{"invalid", "0", "-10m", "0s"}
	for _, val := range invalidValues {
		t.Setenv("CHECKOUT_SESSION_LIFETIME", val)
		_, err := config.Load("test-service")
		if err == nil {
			t.Fatalf("expected error for CHECKOUT_SESSION_LIFETIME=%q, got nil", val)
		}
	}
}

func TestLoadRejectsInvalidShutdownTimeout(t *testing.T) {
	invalidValues := []string{"invalid", "0", "-5"}
	for _, val := range invalidValues {
		t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", val)
		_, err := config.Load("test-service")
		if err == nil {
			t.Fatalf("expected error for SHUTDOWN_TIMEOUT_SECONDS=%q, got nil", val)
		}
	}
}

func TestConfigOutboxDefaults(t *testing.T) {
	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("expected clean config load, got: %v", err)
	}

	if cfg.OutboxClaimLeaseDuration != 30*time.Second {
		t.Errorf("expected lease duration 30s, got %v", cfg.OutboxClaimLeaseDuration)
	}
	if cfg.OutboxClaimRenewalMargin != 10*time.Second {
		t.Errorf("expected renewal margin 10s, got %v", cfg.OutboxClaimRenewalMargin)
	}
	if cfg.RabbitMQPublishConfirmTimeout != 5*time.Second {
		t.Errorf("expected confirm timeout 5s, got %v", cfg.RabbitMQPublishConfirmTimeout)
	}
	if cfg.OutboxBatchSize != 50 {
		t.Errorf("expected batch size 50, got %d", cfg.OutboxBatchSize)
	}
	if cfg.OutboxPollInterval != 500*time.Millisecond {
		t.Errorf("expected poll interval 500ms, got %v", cfg.OutboxPollInterval)
	}
}

func TestConfigValidatesRenewalMarginLessThanLease(t *testing.T) {
	t.Setenv("OUTBOX_CLAIM_LEASE_DURATION", "10s")
	t.Setenv("OUTBOX_CLAIM_RENEWAL_MARGIN", "10s")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error when renewal margin equals lease duration, got nil")
	}
}

func TestConfigValidatesRenewalMarginNonPositive(t *testing.T) {
	for _, val := range []string{"0", "0s", "-1s"} {
		t.Setenv("OUTBOX_CLAIM_RENEWAL_MARGIN", val)
		_, err := config.Load("test-service")
		if err == nil {
			t.Fatalf("expected error for OUTBOX_CLAIM_RENEWAL_MARGIN=%q, got nil", val)
		}
	}
}

func TestConfigValidatesConfirmTimeoutLessThanLease(t *testing.T) {
	t.Setenv("OUTBOX_CLAIM_LEASE_DURATION", "5s")
	t.Setenv("RABBITMQ_PUBLISH_CONFIRM_TIMEOUT", "10s")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error when confirm timeout exceeds lease duration, got nil")
	}
}

func TestConfigValidatesConfirmTimeoutNonPositive(t *testing.T) {
	for _, val := range []string{"0", "0s", "-1s"} {
		t.Setenv("RABBITMQ_PUBLISH_CONFIRM_TIMEOUT", val)
		_, err := config.Load("test-service")
		if err == nil {
			t.Fatalf("expected error for RABBITMQ_PUBLISH_CONFIRM_TIMEOUT=%q, got nil", val)
		}
	}
}

func TestConfigValidatesLeaseDurationPositive(t *testing.T) {
	t.Setenv("OUTBOX_CLAIM_LEASE_DURATION", "-5s")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for non-positive lease duration, got nil")
	}
}

func TestConfigValidatesBatchSizePositive(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "0")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for non-positive batch size, got nil")
	}
}

func TestConfigValidatesPollIntervalPositive(t *testing.T) {
	t.Setenv("OUTBOX_POLL_INTERVAL", "0s")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for non-positive poll interval, got nil")
	}
}
