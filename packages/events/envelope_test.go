package events

import (
	"testing"
	"time"
)

func TestEventEnvelopeValidation(t *testing.T) {
	event := EventEnvelope{
		EventID:       "evt_1",
		EventType:     "TestEvent",
		SchemaVersion: 1,
		OccurredAt:    time.Now(),
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestEventEnvelopeRequiresStableIdentity(t *testing.T) {
	event := EventEnvelope{
		EventType:     "TestEvent",
		SchemaVersion: 1,
		OccurredAt:    time.Now(),
	}

	if err := event.Validate(); err == nil {
		t.Fatal("expected missing event id error")
	}
}
