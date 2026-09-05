DROP INDEX IF EXISTS outbox_events_unpublished_claim_idx;

ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS publish_claim_id,
    DROP COLUMN IF EXISTS publish_claimed_at,
    DROP COLUMN IF EXISTS publish_attempts,
    DROP COLUMN IF EXISTS next_attempt_at;
