# Repository Independence — Dependency Inventory

Stage 0 deliverable of the Repository Independence Refactor.

This document is the authoritative inventory of every compile-time dependency
that an actor repository (admin, seller, supplier) currently has on
`github.com/matjeroapps/core`. No architectural change was made before this
inventory was complete.

> **Status: fully resolved.** Every dependency listed below has been removed. The
> inventory is retained as the historical record of the starting state and is
> written in the present tense of that moment. See
> [repository-independence-report.md](repository-independence-report.md) for the
> completion evidence and the final independence matrix.

## Rule being implemented

> No Matjero repository may import source code, Go packages, workspace modules,
> generated build-time artifacts, vendored code, or other compile-time code from
> another Matjero repository.

Cross-repository collaboration happens only through versioned runtime HTTP/JSON
contracts.

## Baseline

| Repository | Branch | HEAD at inventory time |
| --- | --- | --- |
| core | main | `c63d084863d517599853b48bb3c262685a7b6ec9` |
| admin | main | `aa8a7362b5e17aba8c8c6207366e1efc1284727a` |
| seller | main | `b2441ee169f4ac614d3924c65ac74d314843d072` |
| supplier | main | `77ceb5257841623d58f6f73f7e5322c3caedc7ed` |

Module pins found in actor `go.mod` files:

- seller: `github.com/matjeroapps/core v0.0.0-20260901072638-c63d084863d5`
- admin: `github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736`
- supplier: `github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736`

## Classification legend

| Class | Meaning | Replacement strategy |
| --- | --- | --- |
| **A** | Generic technical code | Localize the minimal required implementation inside the consuming repository |
| **B** | Business / application capability | Core-owned runtime HTTP API + actor-owned local Core client |
| **C** | Test-only dependency | Actor-local fake/stub HTTP Core server or actor-owned fixtures |
| **D** | Dead / unused | Remove |

---

## Dependency matrix

### A. Generic technical code

| Repo | Core package | Files using it | Symbols consumed | Class | Replacement |
| --- | --- | --- | --- | --- | --- |
| seller, admin, supplier | `packages/config` | `apps/*/main.go`, `internal/storefrontapi/router.go` | `config.Config`, `config.Load`, `PlatformDomain`, `TrustedForwardedHost`, `ReservedSubdomains`, `ThemePreviewSecret`, `ZitadelIssuer`, `ZitadelAudience`, `DatabaseURL`, `HTTPAddr`, `OpenAPIDocsEnabled` | A | Localize as `internal/config` per actor |
| seller, admin, supplier | `packages/httpx` | all routers, `apps/*/main.go` | `WriteJSON`, `WriteError`, `ErrorResponse`, `NewRouter`, `Run`, `ConfigFrom`, `CorrelationID`, `RequestID`, header constants | A | Localize as `internal/httpx` per actor |
| seller | `packages/i18n` | `internal/storefrontapi/router.go`, `internal/sellerapi/router.go` | `FromContext`, `Locale`, `SupportedLocales`, `Middleware`, `Default` | A | Localize as `internal/i18n` |
| seller, supplier | `packages/money` | `internal/sellerapi/router.go`, `internal/supplierapi/*` | `money.Money`, `money.New` | A | Localize as `internal/money` |
| seller, admin, supplier | `packages/auth` | `apps/*/main.go` | `NewOIDCVerifier`, `Config`, `DefaultRolesClaim`, `Role*`, `Middleware`, `RequireAnyRole`, `Principal`, `PrincipalFrom`, `WithPrincipal`, `BearerToken` | A | Localize as `internal/auth` |
| seller, admin, supplier | `packages/database` | `apps/*/main.go` | `Connect`, `Pool` | A | Remove where no actor-owned persistence exists; otherwise localize |
| seller, admin, supplier | `packages/logging` | `apps/*/main.go` | `New` | A | Localize as `internal/logging` |
| seller, admin, supplier | `packages/observability` | `apps/*/main.go` | `Init`, `Shutdown` | A | Localize as `internal/observability` |
| seller, admin, supplier | `modules/actorapi` | `apps/*/main.go` | `NewRouter`, `Config`, `MarketService` | A | Localize as `internal/actorapi` (bootstrap + markets routes) |
| seller, admin, supplier | `modules/actorhttp` | `internal/{seller,admin,supplier}api/*` | `ParsePage`, `SubjectFrom`, `DecodeJSON`, `TranslationInput`, `UpdateStatusHandler`, `ResolveSellerID`, `ResolveSupplierID`, `WriteCommerceError` | A | Localize as `internal/actorhttp` |
| seller, admin, supplier | `modules/contracts` | `internal/sellerapi/themes.go`, openapi specs | `CollectionResponse`, `StatusResponse`, `CountResponse`, `MarketsResponse`, `StatusUpdateRequest` | A | Localize as `internal/contracts` |
| seller, admin, supplier | `modules/openapi` | `internal/openapi/aliases.go`, `internal/openapi/specs.go` | `RouteSpec`, `ParameterSpec`, `ResponseSpec`, `DocumentSpec`, `RouterConfig`, `BuildDocument`, `MarshalDocument`, `ValidateDocument`, `NewSpecHandler`, `NewRouter`, `ActorRoutes`, `CommonTags`, response/param helpers | A | Localize as `internal/openapi` |
| seller, admin, supplier | `modules/api` | via `modules/openapi`, `modules/actorapi` | `Bootstrap`, `NewBootstrap` | A | Localize alongside openapi/actorapi |

### B. Business / application capability

| Repo | Core package | Files using it | Symbols consumed | Class | Replacement |
| --- | --- | --- | --- | --- | --- |
| seller | `modules/storefront` | `internal/storefrontapi/router.go`, `contracts.go`, `apps/storefront-api/main.go` | `CatalogRepository`, `CatalogScope`, `NewCatalogScope`, `StoreResolver`, `NewStoreResolver`, `ResolvedStore`, `DomainFromRequest`, `NormalizeHost`, `StoreBootstrap`, `CategoryNode`, `ProductQuery`, `ProductPage`, `ProductListItem`, `ProductDetail`, `ErrStoreNotFound`, `ErrDomainInactive`, `ErrStoreInactive`, `ErrCatalogNotFound`, `ErrInvalidQuery` | B | `GET /internal/v1/storefront/*` |
| seller, admin, supplier | `modules/commerce` | all actor routers, `apps/*/main.go` | `Repository` (≈40 methods), `Service` (14 methods), `Page`, `SupplierCatalogFilter`, `ProductTranslation`, and all domain models (`Seller`, `Supplier`, `Store`, `Product`, `Category`, `SupplierOffer`, `SellerListing`, `InventorySnapshot`, `InventoryMovement`, `FulfillmentLocation`, `SupplierMarket`, `SupplierProduct`, …) | B | `GET/POST/PUT /internal/v1/{sellers,suppliers,stores,products,categories,offers,listings,inventory,locations}/*` |
| seller, admin, supplier | `modules/markets` | `apps/*/main.go`, `modules/actorapi` (transitively) | `NewService`, `NewRepository`, `Market`, `ErrNotFound` | B | `GET /internal/v1/markets`, `GET /internal/v1/markets/{code}` |
| seller | `modules/themes` | `internal/sellerapi/themes.go`, `apps/seller-api/main.go` | `Service`, `NewService`, `Repository`, `Options`, `Theme`, `ThemeVersion`, `ThemeInstallation`, `ErrNotFound`, `ErrConflict`, `ErrSchemaMismatch`, `ErrUnsafeContent`, `ErrInvalidInput`, `ErrPreviewNotConfigured` | B | `GET/POST/PUT /internal/v1/themes/*`, `/internal/v1/stores/{id}/theme/*` |

### C. Test-only dependencies

| Repo | Core dependency | Files | Symbols consumed | Class | Replacement |
| --- | --- | --- | --- | --- | --- |
| seller | Core migrations via `go list -m -f '{{.Dir}}' github.com/matjeroapps/core` | `internal/storefrontapi/testdb_test.go` | `000002`–`000007` `.up.sql` files | C | Delete; replace with actor-local fake Core HTTP server |
| seller | `modules/commerce` (test fixtures) | `internal/storefrontapi/router_test.go`, `privacy_test.go` | `NewRepository`, `CreateSeller`, `CreateStoreWithDomain`, `CreateSupplier`, `CreateSupplierMarket`, `CreateFulfillmentLocation`, `CreateProduct`, `CreateSupplierOffer`, `CreateSellerListing`, … | C | Delete; business correctness stays in Core, transport correctness uses stubs |
| seller | `modules/storefront` (test wiring) | `internal/storefrontapi/router_test.go` | `NewCatalogRepository`, `NewStoreResolver` | C | Delete |
| seller | `modules/actorapi`, `modules/markets` (test wiring) | `internal/storefrontapi/router_test.go` | `NewRouter`, `NewService`, `NewRepository` | C | Localize actorapi; stub markets |

### D. Dead / unused

None found. Every Core import in actor repositories is reachable from a
registered route, a `main.go` wiring path, or a test.

---

## Per-repository file inventory

### seller

| File | Core packages | Class |
| --- | --- | --- |
| `apps/seller-api/main.go` | auth, config, database, httpx, logging, observability, actorapi, commerce, markets, themes | A + B |
| `apps/storefront-api/main.go` | config, database, httpx, logging, observability, actorapi, commerce, markets, storefront | A + B |
| `internal/openapi/aliases.go` | openapi | A |
| `internal/openapi/specs.go` | openapi (via aliases) | A |
| `internal/sellerapi/contracts.go` | commerce | B |
| `internal/sellerapi/router.go` | httpx, i18n, money, actorhttp, commerce | A + B |
| `internal/sellerapi/themes.go` | httpx, actorhttp, commerce, contracts, themes | A + B |
| `internal/storefrontapi/contracts.go` | storefront | B |
| `internal/storefrontapi/router.go` | config, httpx, i18n, storefront | A + B |
| `internal/storefrontapi/router_test.go` | config, money, actorapi, commerce, markets, storefront | C |
| `internal/storefrontapi/testdb_test.go` | Core migrations via `go list -m` | C |
| `internal/storefrontapi/privacy_test.go` | (transitively via router_test) | C |
| `README.md` | documentation reference | A (doc) |

### admin

| File | Core packages | Class |
| --- | --- | --- |
| `apps/admin-api/main.go` | auth, config, database, httpx, logging, observability, actorapi, commerce, markets | A + B |
| `internal/adminapi/router.go` | httpx, actorhttp, commerce | A + B |
| `internal/openapi/aliases.go` | openapi | A |
| `internal/openapi/specs.go` | openapi (via aliases) | A |
| `README.md` | documentation reference | A (doc) |

### supplier

| File | Core packages | Class |
| --- | --- | --- |
| `apps/supplier-api/main.go` | auth, config, database, httpx, logging, observability, actorapi, commerce, markets | A + B |
| `internal/supplierapi/contracts.go` | money, commerce | A + B |
| `internal/supplierapi/router.go` | httpx, money, actorhttp, commerce | A + B |
| `internal/openapi/aliases.go` | openapi | A |
| `internal/openapi/specs.go` | openapi (via aliases) | A |
| `README.md` | documentation reference | A (doc) |

---

## Database ownership findings

All three actor repositories currently open a direct PostgreSQL connection to
the Core-owned commerce database and construct `commerce.NewRepository(db.Pool)`.

Direct SQL access to Core-owned tables exists in:

- `admin/internal/adminapi/router.go` — `handleAdminOverview` runs raw
  `SELECT count(*) FROM suppliers|sellers|stores|products|categories|supplier_offers|seller_listings`
  through `deps.Repo.Pool()`. This is the only raw-SQL path and it must move
  behind a Core capability (`GET /internal/v1/admin/overview`).
- Every other actor data access goes through `commerce.Repository` /
  `commerce.Service` / `storefront.CatalogRepository` / `themes.Service`, all of
  which are Core-owned and must be reached over HTTP.

No actor repository owns any persistence of its own. After this refactor:

- seller: no database dependency (pgx removed)
- admin: no database dependency (pgx removed)
- supplier: no database dependency (pgx removed)

## Required Core runtime capabilities

Derived from the inventory above. These are the capabilities Core must expose
on `/internal/v1` before any actor can be migrated.

1. **Markets** — list, get by code (needed by every actor's `/v1/bootstrap` and
   `/v1/markets` routes).
2. **Storefront** — host-scoped store bootstrap, categories, category detail,
   product browse, product detail, search (Seller P4.3).
3. **Seller identity** — resolve authenticated subject → seller ID.
4. **Seller profile & settings** — read, update.
5. **Stores** — list (admin), list by seller, create for seller, get, status.
6. **Supplier catalog** — browse offers available to a store's market.
7. **Seller listings** — list by store, import, set price, set status, list
   (admin).
8. **Supplier identity** — resolve authenticated subject → supplier ID.
9. **Supplier profile & settings** — read, update.
10. **Supplier markets** — list by supplier.
11. **Fulfillment locations** — list by supplier, create, list (admin), status.
12. **Supplier products** — list by supplier, create, set categories.
13. **Supplier offers** — list by supplier, create, set price, set
    availability, list (admin), status.
14. **Inventory** — list snapshots, create snapshot, adjust, list movements.
15. **Products / categories** — list (admin), status.
16. **Admin overview** — aggregate counts.
17. **Themes** — catalog, versions, installation, draft get/update, publish,
    discard, upgrade, preview token.

## Non-Go coupling found

| Repo | Location | Coupling | Resolution |
| --- | --- | --- | --- |
| seller | `internal/storefrontapi/testdb_test.go` | `go list -m -f '{{.Dir}}' github.com/matjeroapps/core` to read `migrations/*.up.sql` | Remove (class C) |
| seller, admin, supplier | `go.mod` / `go.sum` | `require github.com/matjeroapps/core` | Remove |
| all | `/var/www/personal/matjero/go.work` | developer convenience workspace | Keep uncommitted; never required by any repo |

No Docker `COPY ../core`, no `go replace` to a sibling path, no vendored Core
copy, no submodule, and no build-time download of Core OpenAPI was found.
