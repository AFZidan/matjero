# ADR-006: Transactional Outbox

## Status

Accepted

## Decision

Important business state changes that require outgoing events persist the business state and outbox event in the same PostgreSQL transaction.

## Consequences

RabbitMQ publishing is recoverable from PostgreSQL. Phase 0 creates the outbox schema and package foundation without introducing commerce events.
