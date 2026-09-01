# ADR-001: PostgreSQL Source of Truth

## Status

Accepted

## Decision

PostgreSQL is the transactional source of truth for platform business state. Redis, RabbitMQ, caches, search indexes, and external systems are derived or transport layers.

## Consequences

Transactional workflows fail closed when PostgreSQL is unavailable. Phase 0 creates PostgreSQL connection, migration, transaction, outbox, and inbox foundations before commerce features are implemented.
