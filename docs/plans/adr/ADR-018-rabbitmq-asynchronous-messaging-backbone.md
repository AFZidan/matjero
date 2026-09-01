# ADR-018: RabbitMQ as the Asynchronous Messaging Backbone

## Status

Accepted

Supersedes [ADR-012: Kafka Introduction Strategy](ADR-012-kafka-introduction-strategy.md).
ADR-012 remains as the historical record of the original decision to keep Kafka
as a future event backbone; that future-planning assumption is retired here.

## Context

ADR-012 accepted a two-broker future: RabbitMQ and the PostgreSQL outbox would
serve the MVP, and Kafka would arrive later as the domain-event backbone once
replay, high-volume fan-out, analytics, search indexing, data-warehouse, or
ecosystem requirements justified it. To keep that door open, the platform was
asked to define "Kafka-compatible" event contracts from the beginning and to
plan Kafka partition keys for order, inventory, and seller events.

Carrying that assumption has a real cost:

1. **RabbitMQ already satisfies every asynchronous requirement Matjero has.**
   Background jobs, asynchronous commands, integration synchronisation, webhook
   processing, notifications, and domain-event delivery are all within its
   normal operating envelope.
2. **Two brokers is more than twice the operational surface.** Each one needs
   provisioning, upgrades, capacity planning, monitoring, alerting, backup and
   restore procedures, and on-call familiarity.
3. **Ownership becomes ambiguous.** Every new event forces a question with no
   principled answer: "does this belong on RabbitMQ or Kafka?" That question
   would be asked, and answered differently, on every feature.
4. **Dual publishing introduces a distributed failure mode with no clean
   resolution.** If the RabbitMQ publish succeeds and the Kafka publish fails
   (or the reverse), the platform has to decide whether the event happened.
5. **Kafka brings a second conceptual model** — topics, partitions, consumer
   groups, offsets, retention, rebalancing, broker storage — that the current
   workload does not need and that would have to be understood by everyone
   touching messaging.
6. **The specific drivers listed in ADR-012 do not actually require Kafka at
   Matjero's architecture scale.** Search indexing, notifications, integrations,
   webhooks, jobs, domain events, and fan-out to several independent consumers
   are all ordinary RabbitMQ exchange-and-queue work.
7. **YAGNI.** Designing for a migration that has no scheduled trigger is
   speculative work that constrains present decisions.

None of this is a judgement about Kafka as technology. Kafka is a good fit for a
retained, replayable event log consumed by a large independent ecosystem.
Matjero does not have that requirement, and should not pay for it in advance.

## Decision

### The messaging rule

| Concern | Mechanism |
| --- | --- |
| Synchronous inter-service capability calls needing an immediate answer | HTTP/JSON |
| All asynchronous messaging | RabbitMQ |
| Transactional source of truth | PostgreSQL |
| Reliable coupling of a business commit to event publication | PostgreSQL Transactional Outbox |
| Non-authoritative caching and ephemeral coordination | Redis |
| Kafka | Not part of the architecture or the roadmap |

RabbitMQ is Matjero's **sole planned asynchronous messaging broker**. It is not
a temporary MVP measure and not a placeholder for a later broker.

### RabbitMQ carries all asynchronous messaging

RabbitMQ is the correct transport for asynchronous commands, background jobs,
domain events, integration events, fan-out, notifications, webhook processing,
search indexing, long-running workflows, and asynchronous external integrations.

Illustrative categories, not a commitment to build any of them here:

- **Asynchronous commands** — imports, external platform synchronisation,
  long-running processing.
- **Jobs** — background tasks, report generation, notification dispatch,
  webhook processing.
- **Domain and integration events** — for example `ProductUpdated`,
  `ListingPublished`, `InventoryChanged`, `OrderCreated`, `OrderPaid`,
  `OrderCancelled`, `ThemePublished`, `StoreUpdated`. These names are
  illustrative; no event is introduced by this ADR.

### Fan-out uses exchanges and independently bound queues

Several consumers needing the same event is not a reason to introduce Kafka. A
RabbitMQ exchange with independently bound queues gives each consumer its own
backlog, its own failure isolation, and its own retry and dead-letter policy.

```text
                  domain event
                       │
                       ▼
                    Exchange
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
    search queue   notification   integration
                      queue          queue
```

### The Transactional Outbox stays mandatory

[ADR-006](ADR-006-transactional-outbox.md) is unchanged. Important business
state changes that require a reliable outgoing event persist the business state
and the outbox event in the same PostgreSQL transaction, and a publisher moves
committed outbox rows to RabbitMQ afterwards.

```text
business state + outbox event   (one PostgreSQL transaction)
              ↓
      Outbox Publisher
              ↓
          RabbitMQ
```

This is what prevents the failure where the database commit succeeds, the broker
publish fails, and the event is silently lost.

### Consumers assume at-least-once delivery

RabbitMQ does not provide exactly-once delivery, and neither does any other
broker in a way that survives consumer crashes. The design assumption is
**at-least-once**, so any consumer with side effects must be idempotent. The
`processed_events` consumer inbox from [ADR-007](ADR-007-consumer-inbox.md) is
the existing foundation for this.

```text
RabbitMQ delivery
   ↓
message / event identity
   ↓
already processed?
   ├── yes → ACK safely
   └── no
        ↓
       process
        ↓
       record processing result
        ↓
       ACK
```

### Publisher and consumer reliability expectations

Production messaging paths are expected to use durable exchanges and queues
where the message matters, publisher confirms, explicit consumer
acknowledgement, bounded retry, dead-letter handling, and observable failures
(queue depth, retry counts, dead-letter volume, consumer lag).

This ADR sets the expectation; it does not build a generic framework to satisfy
it.

### Retries are bounded

A poison message must never loop forever. Every retrying consumer applies a
bounded retry/backoff policy and then routes the message to a dead-letter or
failed-message path for inspection. Exact retry counts, backoff windows, and
dead-letter topology belong to the owning workflow and are deliberately not
fixed platform-wide here.

### Message contracts are transport-stable and versioned, not broker-shaped

Event and message contracts must be stable and versioned independently of
RabbitMQ implementation details. They are no longer described as
"Kafka-compatible".

The existing envelope in `packages/events` already carries what a
transport-stable contract needs: message/event identity, type, schema version,
aggregate identity where applicable, correlation and causation identifiers,
occurrence time, and payload. No field is added to an envelope in anticipation
of a different broker.

### Ordering is a business requirement, not a partitioning rule

Where processing order matters, it is stated as a business requirement — for
example, "messages affecting the same aggregate may require ordered processing"
— and satisfied with aggregate identity plus aggregate version, per the existing
event-ordering approach. Ordering requirements are not expressed as partition
keys, and no partitioning strategy is defined for a broker that is not in the
architecture.

### The synchronous HTTP boundary is unchanged

[ADR-017](ADR-017-repository-independence-and-runtime-service-boundaries.md)
stands. Actor services reach Core business capabilities over versioned HTTP/JSON:

```text
seller-api   ──HTTP──►  core-api
admin-api    ──HTTP──►  core-api
supplier-api ──HTTP──►  core-api
```

RabbitMQ must not replace these calls for the sake of decoupling.

### RabbitMQ RPC is explicitly not the default

RabbitMQ is not the mechanism for synchronous queries or immediate
request/response business capabilities. The pattern

```text
request → RabbitMQ → Core consumer → reply queue → correlation ID → synchronous wait
```

must not be used for ordinary reads such as fetching a product, a catalog page,
a theme, or a store. Those use HTTP/JSON. RabbitMQ RPC may be considered only
for a specific demonstrated requirement, never as a platform default.

### RabbitMQ is never a database access path

An actor service must not use RabbitMQ to run queries against Core's database.

Forbidden:

```text
Seller ──RabbitMQ("run this query / return this table")──► Core database
```

Correct:

```text
Seller ──HTTP──► Core capability API ──► Core business logic ──► PostgreSQL
```

Messages describe business intent and business facts. They never describe
database operations.

### Redis boundary

Redis remains a cache, a rate-limiting and ephemeral coordination layer, and a
store for short-lived derived data. Redis is not a message broker substitute,
not a durable event log, and never a business source of truth.

### Search indexing uses the outbox and RabbitMQ

A future dedicated search engine is fed through the same asynchronous path as
everything else, consistent with
[ADR-016](ADR-016-search-readiness-architecture.md):

```text
Core transaction
    ↓
Transactional Outbox
    ↓
Outbox Publisher
    ↓
RabbitMQ
    ↓
Search Indexer
    ↓
Dedicated Search Engine
```

The choice of search engine remains undecided, and no indexer is implemented by
this ADR.

### No broker-portability abstraction

No `Broker` interface, `KafkaAdapter`, `RabbitMQAdapter`, or generic event-bus
abstraction may be introduced for the purpose of making a future broker change
easier. The existing `packages/messaging` publisher seam stays because it is
useful for testing and ownership, not because it anticipates a migration.

## Future reconsideration

Kafka may be reconsidered only through a **new ADR**, and only when measured
production evidence shows a gap RabbitMQ cannot reasonably close. Candidate
evidence:

- sustained event throughput materially beyond what the RabbitMQ design can
  practically serve;
- long-term append-only event retention becoming a product or data requirement;
- a requirement for arbitrary historical replay;
- many independent consumer groups replaying the same retained event stream;
- serious stream-processing requirements;
- a dedicated event or data platform where the retained event log itself becomes
  a first-class data asset.

Deliberately absent: throughput, retention, and message-count thresholds. Such
numbers would be fabricated today, and a fabricated threshold is worse than no
threshold because it invites a decision without evidence.

Even when one of these requirements appears, Kafka is not the automatic answer.
It is evaluated against the alternatives available at that time, against the
requirement as actually measured.

This ADR is not a commitment to introduce Kafka later.

## Consequences

### Positive

- One asynchronous transport to operate, monitor, document, and staff.
- No "which broker?" question on new asynchronous work.
- No dual-publish partial-failure semantics to design around.
- No architectural work performed solely to preserve a hypothetical migration.
- Reliability requirements are unchanged: outbox for publication, idempotent
  consumers for delivery, bounded retry and dead-lettering for failure.

### Negative

- Workloads that genuinely need a retained, replayable log would require a new
  broker decision rather than an already-planned one. Accepted: that decision
  should be made against a real requirement, not pre-committed.
- RabbitMQ becomes a single asynchronous dependency, so its availability and
  capacity planning matter more. Mitigated by the outbox: work that must not be
  lost is durable in PostgreSQL before it reaches the broker.

### Neutral

- No runtime change. Kafka was never a runtime dependency: there is no Kafka Go
  module, no Kafka Docker service, no Kafka environment variable, and no Kafka
  client code in any Matjero repository. This ADR retires a planning assumption
  recorded in documentation.

## Related decisions

- [ADR-001: PostgreSQL Source of Truth](ADR-001-postgresql-source-of-truth.md)
- [ADR-006: Transactional Outbox](ADR-006-transactional-outbox.md)
- [ADR-007: Consumer Inbox](ADR-007-consumer-inbox.md)
- [ADR-012: Kafka Introduction Strategy](ADR-012-kafka-introduction-strategy.md) — superseded by this ADR
- [ADR-016: Search Readiness Architecture](ADR-016-search-readiness-architecture.md)
- [ADR-017: Repository Independence and Runtime Service Boundaries](ADR-017-repository-independence-and-runtime-service-boundaries.md)
