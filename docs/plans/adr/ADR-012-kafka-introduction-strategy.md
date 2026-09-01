# ADR-012: Kafka Introduction Strategy

## Status

Superseded by [ADR-018: RabbitMQ as the Asynchronous Messaging Backbone](ADR-018-rabbitmq-asynchronous-messaging-backbone.md).

> **Superseding note.** The original decision below preserved Kafka as a future
> event backbone and asked event contracts to stay "Kafka-compatible" from the
> beginning. That future-planning assumption was retired by ADR-018. RabbitMQ is
> now the sole planned asynchronous messaging backbone. Kafka is not part of the
> roadmap and will only be reconsidered through a new ADR driven by measured
> future requirements. The original decision is preserved below as the historical
> record of why the architecture once carried a two-broker plan.

## Decision (historical)

Kafka is not a Phase 0 or early MVP runtime dependency. Event envelopes are Kafka-compatible from the beginning, and Kafka is introduced when replay, high-volume fan-out, analytics, search indexing, data warehouse, or ecosystem requirements justify it.

## Consequences (historical)

RabbitMQ and PostgreSQL outbox support the MVP. Future Kafka introduction should not require rewriting event identity, aggregate ordering, or schema versioning conventions.

## What remains true

Kafka never became a runtime dependency: no Kafka Go module, Docker service,
environment variable, or client code was ever added. The envelope conventions
this ADR motivated — stable event identity, aggregate ordering metadata, and
schema versioning — remain in force under ADR-018, now justified as
transport-stable versioned contracts rather than as broker compatibility.
