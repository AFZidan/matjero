package config_test

import (
	"testing"
	"time"

	"github.com/matjeroapps/core/packages/config"
)

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
