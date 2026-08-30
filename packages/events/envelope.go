package events

import (
	"errors"
	"time"
)

type EventEnvelope struct {
	EventID          string         `json:"event_id"`
	EventType        string         `json:"event_type"`
	SchemaVersion    int            `json:"schema_version"`
	AggregateType    string         `json:"aggregate_type,omitempty"`
	AggregateID      string         `json:"aggregate_id,omitempty"`
	AggregateVersion int64          `json:"aggregate_version,omitempty"`
	CorrelationID    string         `json:"correlation_id,omitempty"`
	CausationID      string         `json:"causation_id,omitempty"`
	OccurredAt       time.Time      `json:"occurred_at"`
	Payload          map[string]any `json:"payload"`
}

type MessageEnvelope struct {
	MessageID     string         `json:"message_id"`
	MessageType   string         `json:"message_type"`
	SchemaVersion int            `json:"schema_version"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	CausationID   string         `json:"causation_id,omitempty"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Payload       map[string]any `json:"payload"`
}

func (e EventEnvelope) Validate() error {
	if e.EventID == "" {
		return errors.New("event_id is required")
	}
	if e.EventType == "" {
		return errors.New("event_type is required")
	}
	if e.SchemaVersion <= 0 {
		return errors.New("schema_version must be greater than zero")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	return nil
}

func (m MessageEnvelope) Validate() error {
	if m.MessageID == "" {
		return errors.New("message_id is required")
	}
	if m.MessageType == "" {
		return errors.New("message_type is required")
	}
	if m.SchemaVersion <= 0 {
		return errors.New("schema_version must be greater than zero")
	}
	if m.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	return nil
}
