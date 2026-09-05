ALTER TABLE outbox_events
    ADD COLUMN publish_claim_id UUID NULL,
    ADD COLUMN publish_claimed_at TIMESTAMPTZ NULL,
    ADD COLUMN publish_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX outbox_events_unpublished_claim_idx
    ON outbox_events (next_attempt_at, created_at, event_id)
    WHERE published_at IS NULL;
