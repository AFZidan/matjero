# ADR-007: Consumer Inbox

## Status

Accepted

## Decision

Important event consumers record processed event identity per consumer so processing is idempotent.

## Consequences

Consumers must treat duplicate deliveries as expected. Phase 0 creates a `processed_events` schema and inbox package foundation.
