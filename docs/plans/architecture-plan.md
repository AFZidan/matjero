# Distributed Commerce Platform Architecture Plan

This plan translates `docs/plans/master-plan.md` into the concrete Phase 0 architecture. The Master Plan remains the roadmap and source of business scope.

## Grilling Outcome

Phase 0 has no unresolved architecture blocker. The foundation will be a monorepo with independently buildable applications, a shared Commerce Core, infrastructure seams for PostgreSQL, Redis, RabbitMQ, ZITADEL, and OpenTelemetry, and no Kafka runtime dependency.

Required now:

- Root Go module with independently buildable app entrypoints.
- Thin actor-facing APIs that call shared Commerce Core modules.
- PostgreSQL migrations, connection pooling, and transaction helper.
- RabbitMQ-compatible message/event envelopes and publisher/consumer seams.
- Transactional outbox and consumer inbox schema.
- React/Next.js frontend foundations with Arabic/English and RTL/LTR.
- Local Docker Compose for PostgreSQL, Redis, RabbitMQ, and ZITADEL.
- CI checks for Go, frontend, migrations, Docker, dependency, and secret hygiene.

Prepared now, implemented later:

- Kafka-compatible event contracts.
- Double-entry ledger invariant.
- Storefront tenant resolution contract.
- Theme safety model.
- Strong inventory reservation strategy.

Postponed intentionally:

- Kafka deployment.
- Kubernetes, service mesh, sharding, distributed SQL, CQRS, and event sourcing.
- Commerce features such as products, orders, checkout, payments, shipping, settlements, and integrations.

## System Context

```mermaid
flowchart TB
    AdminWeb[Admin Web]
    SellerWeb[Seller Web]
    SupplierWeb[Supplier Web]
    StorefrontWeb[Storefront Web]

    AdminAPI[Admin API]
    SellerAPI[Seller API]
    SupplierAPI[Supplier API]
    StorefrontAPI[Storefront API]

    Core[Commerce Core]
    Worker[General Worker]
    Scheduler[Scheduler]

    SupplierIntegrations[Supplier Integrations]
    SellerIntegrations[Seller Integrations]

    PG[(PostgreSQL)]
    Redis[(Redis)]
    Rabbit[(RabbitMQ)]
    Outbox[(Transactional Outbox)]
    Kafka[(Future Kafka)]
    Zitadel[ZITADEL]
    ObjectStorage[(Object Storage)]
    CDN[CDN]

    AdminWeb --> AdminAPI
    SellerWeb --> SellerAPI
    SupplierWeb --> SupplierAPI
    StorefrontWeb --> StorefrontAPI

    AdminWeb --> Zitadel
    SellerWeb --> Zitadel
    SupplierWeb --> Zitadel

    AdminAPI --> Core
    SellerAPI --> Core
    SupplierAPI --> Core
    StorefrontAPI --> Core
    Worker --> Core
    Scheduler --> Core

    Core --> PG
    Core --> Redis
    Core --> Outbox
    Outbox --> Rabbit
    Outbox -. future .-> Kafka

    SupplierIntegrations --> Rabbit
    SellerIntegrations --> Rabbit
    SupplierIntegrations --> Core
    SellerIntegrations --> Core

    AdminWeb --> ObjectStorage
    SellerWeb --> ObjectStorage
    SupplierWeb --> ObjectStorage
    ObjectStorage --> CDN
    CDN --> StorefrontWeb
```

## Application Boundaries

`admin-api`, `seller-api`, `supplier-api`, and `storefront-api` are actor-facing API applications. They own transport, authentication entrypoints, request validation, response shaping, and actor-specific policy checks. They must not duplicate Commerce Core business rules, and they must not call each other to execute commerce workflows.

Commerce Core owns commercial truth and business invariants for markets, sellers, suppliers, stores, catalog, listings, inventory, orders, payments, fulfillment, returns, finance, and events. Actor APIs invoke Commerce Core application/domain modules directly.

`general-worker` executes asynchronous jobs, outbox publication, notifications, cache invalidation, webhook delivery, and other background commands. `scheduler` triggers time-based work such as reconciliation, reservation expiry, settlement cycles, cleanup, and periodic sync.

Supplier and seller integrations are independent applications. A provider can have both `salla-supplier-integration` and `salla-seller-integration`, but they are separately deployable, scalable, versioned, testable, observable, and releasable. There is no mandatory central Salla, Shopify, or WooCommerce runtime service.

## Service Architecture

```mermaid
flowchart LR
    subgraph Apps
        A[admin-api]
        B[seller-api]
        C[supplier-api]
        D[storefront-api]
        W[general-worker]
        S[scheduler]
    end

    subgraph Shared
        HTTP[HTTP bootstrap]
        Auth[Auth boundary]
        DB[Database module]
        Msg[Messaging module]
        Obs[Observability module]
        Events[Event contracts]
    end

    subgraph Core[Commerce Core]
        Markets[Markets]
        Stores[Stores]
        Inventory[Inventory]
        Orders[Orders]
        Finance[Finance]
    end

    A --> HTTP
    B --> HTTP
    C --> HTTP
    D --> HTTP
    W --> Msg
    S --> DB

    HTTP --> Auth
    HTTP --> Obs
    Auth --> Core
    Core --> DB
    Core --> Events
    Events --> Msg
```

## Request Flow

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Auth
    participant Core as Commerce Core
    participant DB as PostgreSQL

    Client->>API: HTTP request
    API->>API: assign request_id and correlation_id
    API->>Auth: validate token and coarse role
    Auth-->>API: principal
    API->>Core: execute use case with principal and context
    Core->>DB: transactional read/write
    DB-->>Core: result
    Core-->>API: domain result
    API-->>Client: structured response
```

## Order and Outbox Async Flow

This is the required later pattern; Phase 0 only installs the foundation.

```mermaid
sequenceDiagram
    participant API
    participant Core as Commerce Core
    participant PG as PostgreSQL
    participant Outbox as Outbox Publisher
    participant Rabbit as RabbitMQ
    participant Worker
    participant Inbox as Consumer Inbox

    API->>Core: command
    Core->>PG: business state + outbox event in one tx
    PG-->>Core: committed
    Outbox->>PG: claim unpublished events
    Outbox->>Rabbit: publish event envelope
    Rabbit->>Worker: deliver message
    Worker->>Inbox: record event identity
    Inbox-->>Worker: process once
```

## Integration Flow

```mermaid
flowchart TB
    Provider[External Provider]
    SupplierApp[Provider Supplier Integration]
    SellerApp[Provider Seller Integration]
    Rabbit[RabbitMQ]
    Core[Commerce Core]
    PG[(PostgreSQL)]

    Provider --> SupplierApp
    Provider --> SellerApp
    SupplierApp --> Rabbit
    SellerApp --> Rabbit
    Rabbit --> SupplierApp
    Rabbit --> SellerApp
    SupplierApp --> Core
    SellerApp --> Core
    Core --> PG
```

Each integration owns integration state only: connections, mappings, cursors, webhook inboxes, sync jobs, reconciliation state, and provider metadata. Commerce Core remains authoritative for products, listings, inventory, orders, payments, balances, and settlements.

## Storefront Request Flow

```mermaid
sequenceDiagram
    participant Customer
    participant CDN
    participant Next as Next.js Storefront
    participant API as Storefront API
    participant Resolver as Store Resolver
    participant Core as Commerce Core
    participant Redis
    participant PG as PostgreSQL

    Customer->>CDN: request domain/path
    CDN->>Next: cache miss or dynamic request
    Next->>API: request with host context
    API->>Resolver: resolve host to store_id and market_id
    Resolver->>Redis: lookup cache
    Resolver->>PG: fallback mapping lookup
    API->>Core: query scoped to resolved store
    Core-->>API: store-scoped data
    API-->>Next: response
    Next-->>Customer: localized RTL/LTR page
```

## Critical Invariants

- PostgreSQL is the transactional source of truth.
- Redis is never authoritative for orders, inventory, payments, balances, or settlements.
- RabbitMQ handles commands, jobs, retries, and work queues.
- Kafka is a future domain-event backbone and is introduced only when replay, fan-out, analytics, or ecosystem requirements justify it.
- Important state changes and outgoing events are persisted transactionally.
- Important event consumers support idempotent processing by event identity.
- Inventory reservation is strongly consistent.
- Inventory belongs conceptually to SKU plus Fulfillment Location.
- Fulfillment Location is a first-class domain concept.
- Store Market equals Seller Listing Market equals Supplier Offer Market and must be enforced in domain logic and database constraints.
- Money never uses floating point. Monetary values use deterministic minor-unit or explicit decimal representation.
- Order items preserve immutable commercial snapshots.
- Financial truth eventually uses an immutable double-entry ledger.
- External integration state is eventually consistent and reconciled.
- Webhooks follow verify, persist, deduplicate, respond quickly, process asynchronously, reconcile.
- Arabic and English are first-class languages with RTL/LTR support.
- Each Seller Store belongs to exactly one Market.
- Cross-border supplier sourcing is unsupported initially.
- Seller stores support multiple platform-controlled themes. Arbitrary seller-provided executable JavaScript is not supported initially.

## Repository Structure

```text
apps/
  admin-api/
  seller-api/
  supplier-api/
  storefront-api/
  workers/
    general-worker/
    scheduler/
  integrations/
    suppliers/
    sellers/
internal/
  markets/
  sellers/
  suppliers/
  stores/
  catalog/
  listings/
  inventory/
  orders/
  payments/
  fulfillment/
  returns/
  finance/
  events/
packages/
  auth/
  config/
  contracts/
  database/
  events/
  httpx/
  inbox/
  logging/
  messaging/
  observability/
  outbox/
web/
  admin/
  seller/
  supplier/
  storefront/
migrations/
docs/
```

## Phase 0 Technology Defaults

- Go: one root module, independently buildable commands.
- HTTP: `net/http` with `chi`.
- Database: PostgreSQL via `pgxpool`.
- Migrations: `golang-migrate`.
- Logging: standard `log/slog` JSON logs.
- Validation: `go-playground/validator`.
- Messaging: RabbitMQ via `amqp091-go` behind package-level seams.
- Observability: OpenTelemetry-compatible trace and metric initialization.
- Frontend: Vite React for management apps, Next.js for storefront, TypeScript everywhere.
- Local infra: Docker Compose for PostgreSQL, Redis, RabbitMQ, and ZITADEL.
