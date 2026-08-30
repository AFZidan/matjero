package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"dropshipping/packages/events"
)

type Store struct{}

type PendingEvent struct {
	EventID       string
	EventType     string
	SchemaVersion int
	Payload       []byte
	CorrelationID string
	CausationID   string
	OccurredAt    time.Time
}

func NewStore() Store {
	return Store{}
}

func (Store) Enqueue(ctx context.Context, tx pgx.Tx, event events.EventEnvelope) error {
	if err := event.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (
			event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
			schema_version, payload, correlation_id, causation_id, occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, event.EventID, event.AggregateType, event.AggregateID, event.AggregateVersion, event.EventType,
		event.SchemaVersion, payload, event.CorrelationID, event.CausationID, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	return nil
}
