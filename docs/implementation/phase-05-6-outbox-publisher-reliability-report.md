# Matjero Phase 5.6 Implementation Report — Outbox Multi-Publisher Claim & Delivery Reliability

## Summary & Verification Status

Phase 5.6 implements a production-grade, multi-publisher reliable Transactional Outbox delivery worker in `matjeroapps/core`.

- **Base SHA:** `65a53f47b393775ff27e733c1ae3289fe79627aa`
- **Branch:** `feature/p5-6-outbox-publisher-reliability`
- **Head SHA:** `3ef0c208a71e8877a2812ae4b99e37b12ba6f955`
- **PR:** `https://github.com/matjeroapps/core/pull/32`
- **Migration F:** `000013_outbox_publish_claims` (up/down)

---

## Technical Specifications

### Outbox Schema Extension (Migration F)

Migration `000013_outbox_publish_claims` extends `outbox_events` with claim metadata and a partial index for efficient due-event discovery:

```sql
ALTER TABLE outbox_events
    ADD COLUMN publish_claim_id UUID NULL,
    ADD COLUMN publish_claimed_at TIMESTAMPTZ NULL,
    ADD COLUMN publish_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX outbox_events_unpublished_claim_idx
    ON outbox_events (next_attempt_at, created_at, event_id)
    WHERE published_at IS NULL;
```

### Configuration Parameters & Validation

Added to `packages/config/config.go`:

| Parameter | Env Variable | Default | Validation Constraint |
|---|---|---|---|
| Lease Duration | `OUTBOX_CLAIM_LEASE_DURATION` | `30s` | `> 0` |
| Renewal Margin | `OUTBOX_CLAIM_RENEWAL_MARGIN` | `10s` | `0 < margin < lease_duration` |
| Confirm Timeout | `RABBITMQ_PUBLISH_CONFIRM_TIMEOUT` | `5s` | `0 < confirm_timeout < lease_duration` |
| Batch Size | `OUTBOX_BATCH_SIZE` | `50` | `> 0` |
| Poll Interval | `OUTBOX_POLL_INTERVAL` | `500ms` | `> 0` |

### Outbox Claim & Per-Event Ownership Revalidation SQL

1. **Bounded Batch Claim (`FOR UPDATE SKIP LOCKED`):**
   Uses DB clock (`clock_timestamp()`) to claim due unpublished events without blocking concurrent workers:
   ```sql
   WITH eligible AS (
       SELECT event_id
       FROM outbox_events
       WHERE published_at IS NULL
         AND next_attempt_at <= clock_timestamp()
         AND (
           publish_claim_id IS NULL
           OR publish_claimed_at < (clock_timestamp() - $2::interval)
         )
       ORDER BY next_attempt_at ASC, created_at ASC, event_id ASC
       LIMIT $3
       FOR UPDATE SKIP LOCKED
   )
   UPDATE outbox_events
   SET publish_claim_id = $1::uuid,
       publish_claimed_at = clock_timestamp()
   FROM eligible
   WHERE outbox_events.event_id = eligible.event_id
   RETURNING outbox_events.event_id::text;
   ```

2. **DB-Authoritative Near-Expiry Renewal:**
   ```sql
   UPDATE outbox_events
   SET publish_claimed_at = clock_timestamp()
   WHERE event_id = ANY($1::uuid[])
     AND publish_claim_id = $2::uuid
     AND published_at IS NULL
     AND publish_claimed_at >= (clock_timestamp() - $3::interval)
     AND (publish_claimed_at + $3::interval - clock_timestamp()) <= $4::interval;
   ```

3. **Per-Event Ownership Revalidation Before Publish:**
   Immediately prior to every network publish call, revalidates ownership and returns the complete `EventEnvelope`:
   ```sql
   UPDATE outbox_events
   SET publish_claimed_at = clock_timestamp()
   WHERE event_id = $1::uuid
     AND publish_claim_id = $2::uuid
     AND published_at IS NULL
     AND publish_claimed_at >= (clock_timestamp() - $3::interval)
   RETURNING aggregate_type, aggregate_id, aggregate_version, event_type, schema_version, payload, correlation_id, causation_id, occurred_at;
   ```
   If zero rows are returned, the event claim was lost/expired and publication is skipped.

4. **Guarded Mark Published:**
   ```sql
   UPDATE outbox_events
   SET published_at = clock_timestamp(),
       publish_claim_id = NULL,
       publish_claimed_at = NULL
   WHERE event_id = $1::uuid
     AND publish_claim_id = $2::uuid
     AND published_at IS NULL;
   ```

5. **Exponential Failure Backoff:**
   ```sql
   UPDATE outbox_events
   SET publish_attempts = publish_attempts + 1,
       publish_claim_id = NULL,
       publish_claimed_at = NULL,
       next_attempt_at = clock_timestamp() + (interval '1 second' * power(2, LEAST(publish_attempts, 6)))
   WHERE event_id = $1::uuid
     AND publish_claim_id = $2::uuid
     AND published_at IS NULL;
   ```

---

## Messaging & Topology

- **RabbitMQ Exchange:** `commerce.events` (Topic, Durable: `true`, Auto-Delete: `false`)
- **Routing Rules:**
  - `commerce.order.created.v1` -> `order.created`
  - `commerce.order.status_changed.v1` -> `order.status_changed`
- **Publisher Confirmations:** Enabled on channel startup (`channel.Confirm(false)`). Publishes use `PublishWithDeferredConfirmWithContext` and wait for confirmation via `WaitContext(ctx)`. Bounded by `RABBITMQ_PUBLISH_CONFIRM_TIMEOUT`.

---

## Matrix Case Test Mapping

| Matrix Case | Requirement Description | Named Test | Result |
|---|---|---|---|
| **49** | DB-Authoritative Near-Expiry Renewal & Long-Batch Pacing | `TestOutboxNearExpiryDBAuthoritativeRenewal`, `TestOutboxProcessorLongBatchRenewsNearExpiry` | PASS |
| **50** | Multi-publisher claim isolation & `SKIP LOCKED` | `TestOutboxTwoPublishersNeverClaimSameEvent`, `TestOutboxClaimBatchUsesSkipLocked` | PASS |
| **51** | Stale lease recovery | `TestOutboxStaleLeaseCanBeReclaimed` | PASS |
| **52** | Stale ACK rejection | `TestOutboxStaleAckCannotMarkPublished` | PASS |
| **53** | Broker failure backoff & formula | `TestOutboxPublishFailureSchedulesBackoff`, `TestOutboxBackoffIsBounded`, `TestOutboxBackoffFormulaExactDBTime` | PASS |
| **54** | Confirm-then-crash duplicate safety | `TestOutboxConfirmThenCrashRepublishesSameEventID` | PASS |
| **55** | Consumer Inbox duplicate suppression | `TestInboxDuplicateSameConsumerProcessedOnce` | PASS |
| **56** | Multi-consumer Inbox independence | `TestInboxSameEventDifferentConsumersEachProcessOnce` | PASS |
| **78** | Per-publish claim renewal | `TestOutboxRenewClaimAndLoadReturnsCompleteEnvelope` | PASS |
| **79** | Lost claim skips network publish | `TestOutboxLostClaimSkipsNetworkPublish` | PASS |
| **80** | Confirm timeout < claim lease & runtime timeout | `TestConfigValidatesConfirmTimeoutLessThanLease`, `TestRabbitPublisherConfirmTimeoutFails`, `TestOutboxConfirmTimeoutBackoff` | PASS |
| **81** | Ambiguous broker delivery safe | `TestOutboxConfirmThenCrashRepublishesSameEventID` | PASS |
| **96** | Complete `OrderCreated` envelope matches Outbox | `TestOrderCreatedPublishedEnvelopeMatchesOutbox` | PASS |
| **97** | Retrying `OrderStatusChanged` uses stable `event_id` | `TestOutboxRetryPreservesEventID` | PASS |
| **98** | HTTP correlation propagation preserved | `TestOrderCreatedPublishedCorrelationPreserved` | PASS |

### Additional Reliability Verification

- **Worker Reconnect Loop & Pacing:** `TestWorkerInitialRabbitSetupFailureRetries`, `TestWorkerRuntimeTransportFailureReconnects`, `TestWorkerGracefulShutdownOnContextCancel` (PASS)
- **NACK & Confirm Timeout Handling:** `TestRabbitPublisherNACKHandling`, `TestOutboxNACKSchedulesBackoff`, `TestOutboxConfirmTimeoutBackoff` (PASS)
- **Large `int64` Precision Preservation:** `TestOutboxPayloadLargeInt64Precision` (PASS)
- **Existing Row Migration Compatibility:** `TestMigration000013ExistingRowCompatibility` (PASS)
- **Malformed Persisted Event Guarded Backoff:** `TestOutboxMalformedPayloadTriggersBackoff`, `TestOutboxInvalidEnvelopeFieldsTriggersBackoff`, `TestOutboxStaleMalformedReleaseProtection` (PASS)
- **Secure UUID Entropy Failure Handling:** `TestOutboxClaimIDGenerationFailure` (PASS)

---

## Real Broker Integration Evidence

Real RabbitMQ Integration test (`TestRabbitMQRealBrokerIntegration`) passed against running RabbitMQ broker instance (`amqp://commerce:commerce@localhost:5672/`):
- Connection established & `commerce.events` topic exchange declared.
- Message published with publisher confirms enabled.
- RabbitMQ broker ACK received.
- Queue subscription verified received payload body, `MessageId`, `Type`, and `CorrelationId`.

---

## Phase Status Summary

- **P5.7 started:** NO
- **P5.8 started:** NO
