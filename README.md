# Matjero Core

Shared domain, infrastructure, and data foundation for the Matjero distributed
commerce platform. This repository owns the business logic, the centralized
database migrations, and the public Go packages that every actor repository
depends on.

## Repository boundaries

| Repository | Owns |
| --- | --- |
| `matjero-core` (this repo) | Commerce/theme/market/storefront domain logic, shared `packages/*`, centralized `migrations/`, background workers, local infrastructure |
| `matjero-admin` | Admin API, admin OpenAPI document, admin dashboard |
| `matjero-seller` | Seller API + Storefront API, theme HTTP surface, seller dashboard, storefront web app |
| `matjero-supplier` | Supplier API, supplier OpenAPI document, supplier dashboard |
| `matjero-supplier-integrations` | Supplier-side provider adapters (not implemented yet) |
| `matjero-seller-integrations` | Seller-side channel adapters (not implemented yet) |

All database migrations live here, in `migrations/`. No other repository owns
migrations.

Actor repositories consume Core only through its public packages
(`pkg/actorapi`, `pkg/actorhttp`, `pkg/api`, `pkg/commerce`, `pkg/contracts`,
`pkg/markets`, `pkg/openapi`, `pkg/storefront`, `pkg/themes`, `packages/*`).
`internal/` is private to Core.

## Local Development

```sh
cp .env.example .env
docker compose up -d postgres redis rabbitmq zitadel
go build ./...
go vet ./...
go test ./...
docker compose config --quiet
```

Core has no frontend workspace; the web applications live in the actor
repositories. Kafka is intentionally not part of the runtime.

