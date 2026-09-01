# Phase 0 Implementation Report

> **Superseded architecture note.** Kafka was retained as a future possibility at
> the time this report was written, which is why the architecture summary below
> records "RabbitMQ as MVP queue/command transport" and documented Kafka
> compatibility.
> [ADR-018](../plans/adr/ADR-018-rabbitmq-asynchronous-messaging-backbone.md)
> later retired that plan: RabbitMQ is the sole asynchronous messaging backbone,
> not an MVP-only transport, and Kafka is not on the roadmap. Every reference
> below that treats Kafka as deferred work is historical and no longer describes
> the roadmap. The delivered implementation facts below are unchanged.

## Summary

Phase 0 established the engineering and architecture foundation for the distributed commerce platform. The repository now contains architecture documentation, ADRs, backend foundations, frontend foundations, local development infrastructure, CI, migrations, and verification commands.

## Architecture Established

- Monorepo with independently buildable applications.
- Thin actor-facing APIs backed by shared Commerce Core packages.
- PostgreSQL as transactional source of truth.
- Redis as non-authoritative cache/coordination layer.
- RabbitMQ as MVP queue/command transport.
- Kafka compatibility documented without adding Kafka as a runtime dependency.
- Transactional outbox and consumer inbox foundations.
- Supplier and seller integrations modeled as independent deployments.

## Applications Created

- `apps/admin-api`
- `apps/seller-api`
- `apps/supplier-api`
- `apps/storefront-api`
- `apps/workers/general-worker`
- `apps/workers/scheduler`
- `web/admin`
- `web/seller`
- `web/supplier`
- `web/storefront`

## Infrastructure Established

- Docker Compose for PostgreSQL, Redis, RabbitMQ, and ZITADEL.
- Dockerfile for Go application builds.
- Dockerfile for web application builds.
- `.env.example` with placeholder local configuration.
- Makefile targets for backend, frontend, Docker, and migration checks.

## Database Foundation

- PostgreSQL connection package using `pgxpool`.
- Transaction helper for explicit context propagation.
- Migration `000001_phase0_foundation` creates:
  - `outbox_events`
  - `processed_events`

## Messaging Foundation

- Event envelope with event identity, type, schema version, aggregate identity, aggregate version, correlation ID, causation ID, occurred time, and payload.
- Message envelope with message identity and propagation metadata.
- RabbitMQ publisher seam using persistent JSON messages.

## Authentication Foundation

- ZITADEL issuer configuration is part of shared config.
- Auth package provides bearer token extraction and principal context.
- Full token validation and resource authorization are deferred to Phase 1.

## Frontend Foundation

- Admin, Seller, and Supplier dashboards use React + TypeScript + Vite.
- Storefront uses Next.js + TypeScript.
- API client conventions exist for dashboard and storefront contexts.
- All web apps include Arabic and English locale resources.
- RTL/LTR direction helpers are implemented.

## Localization Foundation

- Initial locales: `ar`, `en`.
- Direction mapping: Arabic uses `rtl`, English uses `ltr`.
- Locale validation script checks all web apps.

## Observability Foundation

- Structured JSON logging with service and environment fields.
- HTTP request ID and correlation ID propagation.
- OpenTelemetry tracer provider initialization.
- API health and readiness endpoints.

## CI/CD Foundation

GitHub Actions workflow includes:

- Go format check.
- Go vet.
- Go tests.
- Frontend lint.
- Frontend typecheck.
- Frontend locale tests.
- Docker Compose validation.
- Representative Docker builds.
- npm audit.
- gitleaks secret scan.

## Tests Executed

- `go test ./...`
- `go vet ./...`
- `go build ./...`
- `npm run lint`
- `npm run typecheck`
- `npm run test`
- Explicit typechecks for Admin, Seller, Supplier, and Storefront workspaces.
- `npm audit --audit-level=high`
- `docker compose config --quiet`
- PostgreSQL migration up/down against Docker PostgreSQL.
- Docker builds for Admin API, General Worker, Admin Web, and Storefront Web.

## Test Results

All executed checks passed.

`npm install` and Docker `npm ci` reported blocked install-script warnings for `esbuild`, but local and Docker frontend builds completed successfully and `npm audit` found zero vulnerabilities.

## Security Checks

- No plaintext secrets were added.
- `.env.example` contains local placeholders only.
- `npm audit --audit-level=high` passed.
- CI includes gitleaks secret scanning.
- Auth and resource authorization boundaries are documented and scaffolded without pretending Phase 1 authorization is complete.

## Deviations From Plan

- One root Go module was selected for Phase 0. This keeps package development and CI simple while preserving independently buildable application entrypoints.

## Deferred Decisions

- Full ZITADEL token validation and resource authorization.
- Concrete RabbitMQ exchange/queue topology, retry windows, and DLQs for real jobs.
- Kafka deployment.
- Ledger chart of accounts and posting rules.
- Storefront custom-domain verification.
- Theme rendering and extension contracts.

## Known Issues

- The phase 0 report was written before the repository state was finalized; the repository is initialized and remote-linked.
- Full infrastructure integration tests are not yet a separate automated suite.
- No commerce-domain behavior exists yet by design.

## Readiness for Phase 1

The Phase 0 foundation is ready for Phase 1 identity, localization, and markets work.

READY FOR PHASE 1
