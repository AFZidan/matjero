# ADR-012: Kafka Introduction Strategy

## Status

Accepted

## Decision

Kafka is not a Phase 0 or early MVP runtime dependency. Event envelopes are Kafka-compatible from the beginning, and Kafka is introduced when replay, high-volume fan-out, analytics, search indexing, data warehouse, or ecosystem requirements justify it.

## Consequences

RabbitMQ and PostgreSQL outbox support the MVP. Future Kafka introduction should not require rewriting event identity, aggregate ordering, or schema versioning conventions.
