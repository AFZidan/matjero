# Phase 0: Engineering and Architecture Foundation

> **Superseded architecture note.** Kafka was retained here as a future
> possibility at the time this specification was written, and P0.6 described
> envelopes as "Kafka-compatible".
> [ADR-018](../plans/adr/ADR-018-rabbitmq-asynchronous-messaging-backbone.md)
> later retired that plan and standardized RabbitMQ as the sole asynchronous
> messaging backbone. Every reference below that treats Kafka as deferred or
> future work is historical and no longer describes the roadmap: Kafka is not
> planned. The envelope conventions delivered by P0.6 are unchanged and are now
> described as transport-stable versioned contracts. The Phase 0 implementation
> facts below are preserved as delivered.

## Objective

Establish the repository, architecture documentation, backend foundations, frontend foundations, local development infrastructure, CI, and verification strategy needed before commerce implementation begins.

## Scope

- Architecture plan and ADRs.
- Monorepo structure.
- Go app bootstrap for Admin API, Seller API, Supplier API, Storefront API, General Worker, and Scheduler.
- Shared packages for configuration, logging, HTTP lifecycle, database, messaging, outbox, inbox, auth boundary, events, and observability.
- PostgreSQL migrations for outbox and consumer inbox foundations.
- Docker Compose local infrastructure for PostgreSQL, Redis, RabbitMQ, and ZITADEL.
- Frontend bootstrap for Admin, Seller, Supplier, and Storefront web apps.
- Arabic/English and RTL/LTR i18n foundation.
- CI workflow for backend, frontend, migrations, Docker, dependency, and secret checks.

## Out of Scope

- Commerce business features.
- Product, listing, inventory, order, payment, shipping, finance, settlement, marketplace, or integration workflows.
- Kafka runtime deployment.
- Kubernetes, service mesh, CQRS, event sourcing, sharding, database-per-service, and cross-border sourcing.

## Dependencies

- Go 1.26 or newer.
- Node.js 24 or newer.
- npm 12 or newer.
- Docker and Docker Compose.
- PostgreSQL, Redis, RabbitMQ, and ZITADEL for local development.

## Deliverables

- Documentation: architecture plan, ADRs, Phase 0 implementation spec, and implementation report.
- Code: independently buildable Go application entrypoints and shared packages.
- Infrastructure: Docker Compose, migrations, environment example, Makefile.
- Frontend: TypeScript app foundations and i18n resources.
- CI: GitHub Actions workflow.
- Tests: package tests for config, HTTP middleware, events, outbox/inbox SQL builders where applicable.

## Repository Structure

The repository structure is defined in `docs/plans/architecture-plan.md` and implemented under `apps/`, `internal/`, `packages/`, `web/`, `migrations/`, and `docs/`.

## Backend Foundation

Phase 0 implements:

- Go application bootstrap.
- Configuration from environment variables.
- Graceful shutdown.
- Structured JSON logging.
- Request IDs and correlation IDs.
- Structured HTTP errors.
- Validation package dependency.
- HTTP routing conventions.
- `/healthz` and `/readyz` endpoints.
- PostgreSQL connection pooling and transaction helper.
- OpenTelemetry-compatible initialization and shutdown hooks.

## Database Foundation

- Use PostgreSQL through `pgxpool`.
- Use `golang-migrate` SQL migrations.
- Create transactional outbox and consumer inbox tables.
- Provide a transaction helper that keeps context propagation explicit.
- Test database strategy: package unit tests by default; integration tests gated by environment variables when database services are available.

## Messaging Foundation

- Use RabbitMQ via `amqp091-go`.
- Define message and event envelopes with event ID, type, schema version, aggregate identity, correlation ID, causation ID, and timestamps.
- Provide publisher and consumer seams.
- Document retry and DLQ conventions.
- Preserve future Kafka compatibility without requiring Kafka.

## Outbox Foundation

Phase 0 creates the outbox table and package types needed to enqueue and publish events later. Commerce events are intentionally not implemented yet.

## Consumer Inbox Foundation

Phase 0 creates the processed-event table and package foundation for idempotent consumers. Consumer-specific behavior is added with real consumers in later phases.

## Authentication Foundation

Phase 0 creates the ZITADEL configuration and principal/auth boundary. Full resource authorization is implemented in Phase 1 and later domain phases.

## Frontend Foundation

- `web/admin`, `web/seller`, and `web/supplier`: React + TypeScript.
- `web/storefront`: Next.js + TypeScript.
- Shared conventions for API clients and auth config.
- Arabic and English locale resources.
- RTL/LTR direction helpers.
- No commerce screens.

## Observability

- Structured logs include service, environment, request ID, and correlation ID.
- OpenTelemetry hooks exist for future traces and metrics.
- Health/readiness endpoints are available in every API.
- Trace/correlation propagation is part of HTTP and event envelopes.

## Development Environment

Docker Compose provides:

- PostgreSQL
- Redis
- RabbitMQ with management UI
- ZITADEL

Kafka is not required.

## CI

CI runs:

- Go formatting.
- Go vet.
- Go tests.
- Frontend lint.
- Frontend type checking.
- Frontend tests where present.
- Docker Compose configuration validation.
- Migration validation.
- Dependency audit.
- Secret scan.

## Testing Strategy

- Unit tests for pure package behavior.
- HTTP tests for request and correlation ID behavior.
- Migration validation by applying migrations against a PostgreSQL service in CI.
- Frontend type/lint checks for i18n and build foundations.
- Integration tests are opt-in locally until Docker services are running.

## Security Requirements

- Secrets are read from environment variables and never committed.
- `.env.example` contains placeholders only.
- Integration secrets must not be stored plaintext in future phases.
- Auth boundary exists but does not implement domain authorization yet.
- CI includes dependency and secret checks.

## Acceptance Criteria

- All documented ADRs exist and are non-empty.
- All API apps build and expose health/readiness endpoints.
- Worker and scheduler apps build and shut down gracefully.
- Migrations create outbox and processed-event tables.
- Docker Compose config validates.
- Frontend apps type-check and lint.
- Go tests pass.
- Phase 0 implementation report states readiness for Phase 1.

## Task Breakdown

### P0.1 Architecture Documentation

Goal: Capture Phase 0 architecture decisions.
Dependencies: Master Plan.
Implementation: Create architecture plan and ADRs.
Tests: Documentation self-review.
Acceptance Criteria: Docs contain no placeholders and match the Master Plan.

### P0.2 Repository Bootstrap

Goal: Establish monorepo structure.
Dependencies: P0.1.
Implementation: Create app, package, internal, web, migration, and CI directories.
Tests: Directory scan and build commands.
Acceptance Criteria: Directory structure matches architecture plan.

### P0.3 Backend Shared Packages

Goal: Provide reusable backend foundations.
Dependencies: P0.2.
Implementation: Add config, logging, HTTP, database, messaging, events, outbox, inbox, auth, and observability packages.
Tests: Go unit tests.
Acceptance Criteria: Packages build and tests pass.

### P0.4 Backend Applications

Goal: Create buildable app entrypoints.
Dependencies: P0.3.
Implementation: Add API, worker, and scheduler `main.go` files.
Tests: `go test ./...` and `go build` through tests.
Acceptance Criteria: All app commands compile.

### P0.5 Database Migrations

Goal: Install foundational persistence.
Dependencies: P0.3.
Implementation: Add SQL migrations for outbox and inbox.
Tests: Migration validation locally and in CI.
Acceptance Criteria: Migrations apply cleanly to PostgreSQL.

### P0.6 Messaging Foundation

Goal: Prepare RabbitMQ and future Kafka-compatible envelopes.
Dependencies: P0.3.
Implementation: Add message envelopes and RabbitMQ publisher seam.
Tests: Unit tests for envelope validation.
Acceptance Criteria: Messages carry identity, schema version, correlation, and causation metadata.

### P0.7 Frontend Foundations

Goal: Bootstrap all web apps.
Dependencies: P0.2.
Implementation: Add package manifests, TypeScript config, i18n resources, direction helpers, and basic app shells.
Tests: npm lint, typecheck, and tests.
Acceptance Criteria: Apps build their foundations without commerce screens.

### P0.8 Local Development Infrastructure

Goal: Make local services reproducible.
Dependencies: P0.2.
Implementation: Add Docker Compose, `.env.example`, and Makefile.
Tests: Docker Compose config validation.
Acceptance Criteria: PostgreSQL, Redis, RabbitMQ, and ZITADEL are defined; Kafka is absent.

### P0.9 CI

Goal: Automate verification.
Dependencies: P0.3 through P0.8.
Implementation: Add GitHub Actions workflow.
Tests: Local command parity where possible.
Acceptance Criteria: CI expresses backend, frontend, migration, Docker, dependency, and secret checks.

### P0.10 Implementation Report

Goal: Summarize delivered foundation and readiness.
Dependencies: P0.1 through P0.9.
Implementation: Write implementation report after verification.
Tests: Documentation self-review.
Acceptance Criteria: Report includes executed checks and explicit Phase 1 recommendation.

## Implementation Status

Implemented:

- Architecture plan and ADRs.
- Monorepo directory structure.
- Go root module and app entrypoints for Admin API, Seller API, Supplier API, Storefront API, General Worker, and Scheduler.
- Shared Go packages for configuration, logging, HTTP lifecycle, database transactions, message/event envelopes, RabbitMQ publishing, outbox, inbox, auth boundary, and OpenTelemetry initialization.
- PostgreSQL migration for `outbox_events` and `processed_events`.
- Docker Compose for PostgreSQL, Redis, RabbitMQ, and ZITADEL.
- Docker build surfaces for Go apps and web apps.
- React/Vite foundations for Admin, Seller, and Supplier web apps.
- Next.js foundation for Storefront web.
- Arabic/English and RTL/LTR locale foundations.
- CI workflow for backend, frontend, infrastructure, and security checks.

Deferred:

- Commerce business features.
- Full ZITADEL token verification and resource authorization, which belong to Phase 1.
- Real RabbitMQ consumers, retry policies, and DLQ topology, which require concrete jobs/events.
- Kafka deployment.
- Ledger account design and posting rules.
- Storefront domain verification and theme runtime details.

Changed from original plan:

- The repo uses one root Go module for Phase 0 to keep shared package development simple while preserving independently buildable application entrypoints.

Known limitations:

- Frontend app shells are intentionally minimal and contain no commerce screens.
- Integration tests that require all infrastructure services are not yet a separate suite; the foundational migration was validated against PostgreSQL.
- npm reports blocked install scripts for `esbuild`, but local and Docker frontend builds passed.

Test results:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go build ./...`: passed.
- `npm run lint`: passed.
- `npm run typecheck`: passed.
- `npm run test`: passed.
- Explicit workspace typechecks for Admin, Seller, Supplier, and Storefront: passed.
- `npm audit --audit-level=high`: passed with zero vulnerabilities after moving Storefront to Next.js 16.3.3.
- `docker compose config --quiet`: passed.
- PostgreSQL migration up/down against Docker PostgreSQL: passed.
- Docker builds: Admin API, General Worker, Admin Web, and Storefront Web passed.

Architecture deviations:

- None.

## Definition of Done

Phase 0 is complete when all acceptance criteria pass, implementation status is updated, and `docs/implementation/phase-00-implementation-report.md` recommends readiness or clearly states blockers.
