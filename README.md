# Matjero Core

Shared domain, infrastructure, and data foundation for the Matjero distributed
commerce platform. This repository owns the business logic, the centralized
database migrations, and the public Go packages that every actor repository
depends on.

## Repository boundaries

| Local folder | GitHub repository | Owns |
| --- | --- | --- |
| `core` (this repo) | [`matjeroapps/core`](https://github.com/matjeroapps/core) | Commerce/theme/market/storefront domain logic, shared `packages/*`, centralized `migrations/`, background workers, local infrastructure |
| `admin` | [`matjeroapps/admin`](https://github.com/matjeroapps/admin) | Admin API, admin OpenAPI document, admin dashboard |
| `seller` | [`matjeroapps/seller`](https://github.com/matjeroapps/seller) | Seller API + Storefront API, theme HTTP surface, seller dashboard, storefront web app |
| `supplier` | [`matjeroapps/supplier`](https://github.com/matjeroapps/supplier) | Supplier API, supplier OpenAPI document, supplier dashboard |
| `supplier-hub` | [`matjeroapps/supplier-hub`](https://github.com/matjeroapps/supplier-hub) | Supplier-side external commerce integration/connectors (not implemented yet) |
| `seller-hub` | [`matjeroapps/seller-hub`](https://github.com/matjeroapps/seller-hub) | Seller-side external store integration/connectors (not implemented yet) |

"Hub" denotes a connector/integration boundary, not a dashboard.

All database migrations live here, in `migrations/`. No other repository owns
migrations.

Actor repositories consume Core only through its public packages
(`pkg/actorapi`, `pkg/actorhttp`, `pkg/api`, `pkg/commerce`, `pkg/contracts`,
`pkg/markets`, `pkg/openapi`, `pkg/storefront`, `pkg/themes`, `packages/*`).
`internal/` is private to Core.

## Local Development

All six folders are checked out side by side under a single workspace root:

```
/var/www/personal/matjero/
├── core/
├── admin/
├── seller/
├── supplier/
├── seller-hub/
├── supplier-hub/
├── go.work
└── go.work.sum
```

`go.work` and `go.work.sum` live at the workspace root, one level above every
repository, and are deliberately never committed.

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

