package inbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Store struct{}

func NewStore() Store {
	return Store{}
}

func (Store) RecordProcessed(ctx context.Context, tx pgx.Tx, consumerName, eventID string) (bool, error) {
	if consumerName == "" {
		return false, fmt.Errorf("consumer name is required")
	}
	if eventID == "" {
		return false, fmt.Errorf("event id is required")
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO processed_events (consumer_name, event_id)
		VALUES ($1, $2)
		ON CONFLICT (consumer_name, event_id) DO NOTHING
	`, consumerName, eventID)
	if err != nil {
		return false, fmt.Errorf("record processed event: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}
