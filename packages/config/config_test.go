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
	t.Setenv("ORDER_CONFIRMATION_DURATION", "invalid")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for invalid ORDER_CONFIRMATION_DURATION, got nil")
	}
}

func TestLoadRejectsInvalidCheckoutSessionLifetime(t *testing.T) {
	t.Setenv("CHECKOUT_SESSION_LIFETIME", "invalid")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for invalid CHECKOUT_SESSION_LIFETIME, got nil")
	}
}

func TestLoadRejectsInvalidShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "invalid")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error for invalid SHUTDOWN_TIMEOUT_SECONDS, got nil")
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

func TestConfigValidatesConfirmTimeoutLessThanLease(t *testing.T) {
	t.Setenv("OUTBOX_CLAIM_LEASE_DURATION", "5s")
	t.Setenv("RABBITMQ_PUBLISH_CONFIRM_TIMEOUT", "10s")

	_, err := config.Load("test-service")
	if err == nil {
		t.Fatal("expected error when confirm timeout exceeds lease duration, got nil")
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
