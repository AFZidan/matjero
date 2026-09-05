# Multi-Repository Extraction Manifest

> **Status:** authoritative pre-move plan. Written **before** any file was created, moved,
> modified, or deleted.
>
> **Source commit:** `8378f32590dba083050ac2b619ba5cbf511a38dd` (`main`, clean tree)
> **Working branch:** `refactor/multi-repo-folder-split`
> **Nature of change:** filesystem move + minimal adaptation. **Not** a rewrite. The
> existing implementation at the source commit is the single source of truth.
>
> **Historical folder names.** This document records the folder names as planned at the
> time of the split. The folders have since been relocated into one workspace root and
> renamed. Current canonical layout:
>
> | Folder name in this document (historical) | Current local folder |
> | --- | --- |
> | `/var/www/personal/drop-shipping/` | `/var/www/personal/matjero/core/` |
> | `/var/www/personal/matjero-admin/` | `/var/www/personal/matjero/admin/` |
> | `/var/www/personal/matjero-seller/` | `/var/www/personal/matjero/seller/` |
> | `/var/www/personal/matjero-supplier/` | `/var/www/personal/matjero/supplier/` |
> | `/var/www/personal/matjero-seller-integrations/` | `/var/www/personal/matjero/seller-hub/` |
> | `/var/www/personal/matjero-supplier-integrations/` | `/var/www/personal/matjero/supplier-hub/` |
>
> The workspace root — and therefore the location of the untracked `go.work` — is now
> `/var/www/personal/matjero/`, not `/var/www/personal/`.

---

## 1. Target Folder Boundaries

Six sibling folders, planned under `/var/www/personal/` (now `/var/www/personal/matjero/`,
see the note above). Each becomes its own GitHub repository later.

| Folder | GitHub repository | Module path | Contains code? |
| --- | --- | --- | --- |
| `drop-shipping/` | [`matjeroapps/core`](https://github.com/matjeroapps/core) (folder intentionally **not** renamed) | `github.com/matjeroapps/core` | yes |
| `matjero-admin/` | [`matjeroapps/admin`](https://github.com/matjeroapps/admin) — Admin Platform | `github.com/matjeroapps/admin` | yes |
| `matjero-seller/` | [`matjeroapps/seller`](https://github.com/matjeroapps/seller) — Seller Platform **+ Native Storefront** | `github.com/matjeroapps/seller` | yes |
| `matjero-supplier/` | [`matjeroapps/supplier`](https://github.com/matjeroapps/supplier) — Supplier Platform | `github.com/matjeroapps/supplier` | yes |
| `matjero-supplier-integrations/` | [`matjeroapps/supplier-hub`](https://github.com/matjeroapps/supplier-hub) — Supplier connectors | `github.com/matjeroapps/supplier-hub` | no — ownership placeholder |
| `matjero-seller-integrations/` | [`matjeroapps/seller-hub`](https://github.com/matjeroapps/seller-hub) — Seller/channel connectors | `github.com/matjeroapps/seller-hub` | no — ownership placeholder |

Deliberately **no** separate repository for:

- **Storefront** → belongs to `matjeroapps/seller` (the storefront is a seller-owned surface).
- **Infrastructure** → stays in Core (`docker-compose.yml`, `migrations/`, base Dockerfiles).
- **Shared contracts** → stays in Core, exposed through a narrow public Go boundary.

---

## 2. Non-Negotiable Constraints Governing This Manifest

1. Migrations stay centralized in Core `migrations/`, including
   `000007_theme_engine_schema.{up,down}.sql`.
2. Theme **domain / repository / validation / persistence** stays in Core.
   Only the theme **HTTP surface**, seller dashboard, and storefront rendering move to Seller.
3. Commerce business logic (Store, SellerListing, Product, Inventory, Order, Payment,
   Theme persistence) stays in Core.
4. Sibling repositories cannot import Core's Go `internal/`. Only capabilities genuinely
   required across repositories are promoted to a public path. A blanket
   `internal/` → `pkg/` rename is **not** performed.
5. Copy-verify-then-delete. Never delete before the copy has been validated.
6. No `git init` / `git remote add` / `gh repo create` in sibling folders.
   No `git clean -fdx` in the parent directory.
7. No phase identifiers (`phase4`, `p4_`, …) in production or runtime filenames.
8. No new API→API HTTP coupling is introduced to solve the split.
9. `seller-api`, `seller-web`, `storefront-api`, `storefront-web` remain four separate
   build/run targets.

---

## 3. Core Public Boundary Design (Prerequisite for Every Actor Repo)

Today **all** cross-actor shared code lives under Go `internal/`, which siblings cannot
import. The following packages are promoted from `internal/` to `pkg/` because they are
genuinely required by at least one sibling repository. Every promotion is justified below;
nothing is promoted "just in case".

| Source | Destination | Consumers outside Core | Justification |
| --- | --- | --- | --- |
| `internal/commerce/` | `pkg/commerce/` | admin, seller, supplier | Commerce domain + repository + service. All three actor HTTP surfaces are thin adapters over it. |
| `internal/markets/` | `pkg/markets/` | admin, seller, supplier, storefront | Market reference data; every actor `main.go` constructs `markets.NewService`. |
| `internal/themes/` | `pkg/themes/` | seller | Theme domain/persistence stays Core; Seller owns only the HTTP surface, which needs `themes.Service`. |
| `internal/storefront/` | `pkg/storefront/` | seller (storefront-api) | Trusted domain→store tenant resolution (ADR-010). Security-sensitive; must not be reimplemented per repo. |
| `internal/actorapi/` | `pkg/actorapi/` | admin, seller, supplier, storefront | Shared actor router: locale middleware, auth middleware, role gate, `/v1/bootstrap`, `/v1/markets`. |
| `internal/api/` | `pkg/api/` | admin, seller, supplier, storefront (indirectly) | `api.Bootstrap` is the bootstrap response contract, consumed by `actorapi` and by every actor's OpenAPI spec. |
| `internal/openapi/spec.go` | `pkg/openapi/spec.go` | admin, seller, supplier | Code-first OpenAPI document builder, validator, reflection-based schema generator. |
| `internal/openapi/http.go` | `pkg/openapi/http.go` | admin, seller, supplier, storefront | `/openapi.json` + `/docs` Swagger router. |

### 3.1 New Core packages created by extraction (not moves)

| New package | Extracted from | Exported surface | Consumers |
| --- | --- | --- | --- |
| `pkg/actorhttp/` | unexported helpers in `internal/platformapi/router.go` | `Page`, `ParsePage`, `SubjectFrom`, `DecodeJSON`, `UpdateStatusHandler`, `WriteCommerceError`, `ResolveSupplierID`, `ResolveSellerID`, `TranslationInput` | admin, seller, supplier |
| `pkg/contracts/` | generic DTOs in `internal/platformapi/contracts.go` | `CollectionResponse[T]`, `StatusResponse`, `CountResponse`, `MarketsResponse`, `StatusUpdateRequest` | admin, seller, supplier |
| `pkg/openapi/routes.go` | generic route/response helpers in `internal/openapi/specs.go` | `ActorRoutes(authenticated bool)`, `ListResponses[T]`, `AuthReadResponses`, `AuthCreatedResponses`, `AuthOKResponses`, `OKResponse`, `CreatedResponse`, `ErrorResponse`, `LimitParam`, `OffsetParam`, `PathStringParam`, `StringParam`, `CommonTags()` | admin, seller, supplier, storefront |

**Rationale for `pkg/actorhttp`:** `parsePage`, `subjectFrom`, `decodeJSON`,
`updateStatusHandler`, `writeCommerceError`, `resolveSupplierID`, `resolveSellerID` and
`translationInput` are today file-private helpers in `internal/platformapi/router.go`,
used by admin, supplier, seller **and** theme handlers. Without promotion, each actor repo
would have to duplicate them — which would violate "move, do not rewrite" and would let
error-code mapping drift between repositories.

### 3.2 Packages that deliberately stay `internal/` in Core

| Package | Why it stays internal |
| --- | --- |
| `internal/testdb/` | Test-only Postgres harness. Consumed by five Core integration tests only (`commerce` ×2, `markets`, `themes`, `storefront`). No sibling test requires it. |
| `internal/platformapi/` | **Deleted from Core** after its contents are split across actor repos (see §5). Nothing Core-side consumes it afterwards. |
| `internal/openapi/specs.go`, `types.go`, `spec_test.go` | Actor-specific; split and moved (see §5). |

### 3.3 Empty untracked placeholder directories

`internal/{catalog,events,finance,fulfillment,inventory,listings,orders,payments,phase3api,returns,sellers,stores,suppliers}`
contain no tracked files. They are not moved. `internal/phase3api` additionally carries a
phase-based name and is removed rather than propagated.

---

## 4. Straight Moves (whole directories / files, no content split)

### 4.1 → `matjero-admin/`

| Source (Core) | Destination |
| --- | --- |
| `apps/admin-api/main.go` | `apps/admin-api/main.go` |
| `web/admin/` (8 tracked files) | `web/admin/` |
| `docs/api/admin/openapi.json` | `docs/api/admin/openapi.json` |
| `scripts/check-locales.mjs` | `scripts/check-locales.mjs` (copy; Core keeps its own) |
| `docker/go-app.Dockerfile` | `docker/go-app.Dockerfile` (copy, `APP_PATH` default retargeted) |
| `docker/web-app.Dockerfile` | `docker/web-app.Dockerfile` (copy, workspace list reduced) |
| `.dockerignore` | `.dockerignore` (copy) |

### 4.2 → `matjero-seller/`

| Source (Core) | Destination |
| --- | --- |
| `apps/seller-api/main.go` | `apps/seller-api/main.go` |
| `apps/storefront-api/main.go` | `apps/storefront-api/main.go` |
| `web/seller/` (8 tracked files) | `web/seller/` |
| `web/storefront/` (9 tracked files) | `web/storefront/` |
| `docs/api/seller/openapi.json` | `docs/api/seller/openapi.json` |
| `docs/api/storefront/openapi.json` | `docs/api/storefront/openapi.json` |
| `scripts/check-locales.mjs` | `scripts/check-locales.mjs` (copy) |
| `docker/*.Dockerfile`, `.dockerignore` | copies, retargeted |

### 4.3 → `matjero-supplier/`

| Source (Core) | Destination |
| --- | --- |
| `apps/supplier-api/main.go` | `apps/supplier-api/main.go` |
| `web/supplier/` (8 tracked files) | `web/supplier/` |
| `docs/api/supplier/openapi.json` | `docs/api/supplier/openapi.json` |
| `scripts/check-locales.mjs` | `scripts/check-locales.mjs` (copy) |
| `docker/*.Dockerfile`, `.dockerignore` | copies, retargeted |

### 4.4 Stays in Core (`drop-shipping/`)

`migrations/000001`–`000007` (up+down) · `docker-compose.yml` ·
`docker/go-app.Dockerfile` · `docker/web-app.Dockerfile` · `.dockerignore` ·
`.env.example` · `scripts/check-locales.mjs` · `apps/workers/general-worker/` ·
`apps/workers/scheduler/` · `packages/**` (12 packages) · `pkg/**` (per §3) ·
`internal/testdb/` · `docs/decisions/` · `docs/implementation/` · `docs/plans/` ·
`README.md` · `.github/workflows/ci.yml` (rewritten, §7) · `Makefile` (reduced, §7).

`packages/**` requires **no** relocation: it already lives outside `internal/` and is
importable by siblings once the module path changes.

---

## 5. Content Splits (mixed files — mechanical extraction, no rewrite)

### 5.1 `internal/platformapi/router.go` (911 lines) → 4 destinations

| Extracted symbols | Destination |
| --- | --- |
| `pageQuery`, `parsePage`, `subjectFrom`, `decodeJSON`, `updateStatusHandler`, `resolveSupplierID`, `resolveSellerID`, `writeCommerceError`, `translationInput` | Core `pkg/actorhttp/actorhttp.go` (exported) |
| `Dependencies`, `RegisterAdminRoutes` (17 routes), `handleAdmin*` (17 handlers incl. `handleAdminOverview`) | `matjero-admin/internal/adminapi/router.go` |
| `Dependencies`, `RegisterSupplierRoutes` (14 routes), `handleSupplier*` (14 handlers) | `matjero-supplier/internal/supplierapi/router.go` |
| `Dependencies`, `RegisterSellerRoutes` (9 routes), `handleSeller*` (9 handlers) | `matjero-seller/internal/sellerapi/router.go` |

`Dependencies{Commerce commerce.Service; Repo commerce.Repository}` is duplicated into each
actor package because it is a per-actor wiring struct, not shared logic. This duplication is
recorded in the duplicate-ownership audit as intentional.

### 5.2 `internal/platformapi/contracts.go` (129 lines) → 4 destinations

| DTOs | Destination |
| --- | --- |
| `CollectionResponse[T]`, `StatusResponse`, `CountResponse`, `MarketsResponse`, `StatusUpdateRequest` | Core `pkg/contracts/contracts.go` |
| `SupplierProfileResponse`, `SupplierProfileUpdateRequest`, `SupplierLocationCreateRequest`, `SupplierProductTranslationRequest`, `SupplierProductCreateRequest`, `SupplierProductCategoriesRequest`, `SupplierOfferCreateRequest`, `ProductCreateResponse`, `InventorySnapshotCreateRequest`, `InventoryAdjustmentRequest`, `InventoryAdjustmentResponse` | `matjero-supplier/internal/supplierapi/contracts.go` |
| `SellerProfileResponse`, `SellerProfileUpdateRequest`, `SellerStoreCreateRequest`, `SellerListingImportRequest`, `SellerListingPriceRequest` | `matjero-seller/internal/sellerapi/contracts.go` |
| (admin uses only the generic DTOs) | — |

### 5.3 `internal/platformapi/themes.go` (278 lines) → Seller

Whole file → `matjero-seller/internal/sellerapi/themes.go`
(`ThemeDependencies`, `themeServer`, `RegisterSellerThemeRoutes` (10 routes), `sellerID`,
`writeThemeError`, 10 handlers, 9 theme DTOs). Its dependencies on `subjectFrom` /
`decodeJSON` are re-pointed at `pkg/actorhttp`.

### 5.4 `internal/openapi/specs.go` (737 lines) → 5 destinations

| Symbols | Destination |
| --- | --- |
| `actorRoutes`, `listResponses`, `authReadResponses`, `authCreatedResponses`, `authOKResponses`, `okResponse`, `createdResponse`, `errorResponse`, `limitParam`, `offsetParam`, `pathStringParam`, `stringParam`, `openAPITags` | Core `pkg/openapi/routes.go` (exported) |
| `BuildAdminSpec`, `adminRoutes()` (18 routes) | `matjero-admin/internal/openapi/spec.go` |
| `BuildSupplierSpec`, `supplierRoutes()` (14 routes) | `matjero-supplier/internal/openapi/spec.go` |
| `BuildSellerSpec`, `sellerRoutes()` (19 routes: 9 commerce + 10 theme) | `matjero-seller/internal/openapi/seller_spec.go` |
| `BuildStorefrontSpec` | `matjero-seller/internal/openapi/storefront_spec.go` |

**Preserved as-is:** `openAPITags()` declares 15 tags and does **not** declare `Themes` /
`Theme Configuration`, even though seller theme routes reference them. Spec validation
currently passes; this behaviour is preserved rather than "fixed", per the no-rewrite rule.

### 5.5 `internal/openapi/types.go` (35 lines) — coupling seam, deleted

35 type aliases from `openapi` → `platformapi`. Each alias is replaced by a direct
reference in the repository that now owns the underlying DTO (Core `pkg/contracts` for the
generic five; the actor's own `contracts.go` otherwise). The `openapi → platformapi`
dependency edge disappears entirely.

### 5.6 `internal/openapi/spec_test.go` (193 lines) → 4 destinations

| Test | Destination |
| --- | --- |
| `TestDocsRouterEnabledDisabled` (generic router behaviour) | Core `pkg/openapi/http_test.go`, using a minimal locally built document instead of the admin spec |
| `TestBuildDocumentsValidate`, `TestBuildDocumentsDeterministic`, `TestSecuritySchemes`, `TestImportantRoutes` (admin slice), `containsTag` | `matjero-admin/internal/openapi/spec_test.go` |
| same tests, supplier slice | `matjero-supplier/internal/openapi/spec_test.go` |
| same tests, seller + storefront slices (incl. storefront "declares no security scheme" and `/v1/bootstrap` `Security == nil`) | `matjero-seller/internal/openapi/spec_test.go` |

### 5.7 `cmd/openapi-gen/main.go` (45 lines) → 3 destinations

The 4-entry `{path, build}` table is split so each actor repo regenerates only the specs it
owns: admin → 1 spec, supplier → 1 spec, seller → 2 specs (seller + storefront). The `fail`
helper is copied into each. Core no longer ships `cmd/openapi-gen`.

### 5.8 `Makefile` → 4 destinations

Core keeps `go-test`, `docker-config`, `migrate-check`, and a `docker-build` reduced to the
worker image only. Each actor repo receives a Makefile with its own `go-test`,
`frontend-*`, and `docker-build` targets. `migrate-check` exists only in Core, since
migrations are Core-owned.

### 5.9 `package.json` (root) → 4 destinations

Core drops the npm workspace root entirely (no frontend remains in Core). Each actor repo
gets a root `package.json` with `private: true`, `typescript ^5.9.2`, and a workspace list
containing only its own web apps:

| Repo | Workspaces |
| --- | --- |
| `matjero-admin` | `web/admin` |
| `matjero-seller` | `web/seller`, `web/storefront` |
| `matjero-supplier` | `web/supplier` |

---

## 6. Required Minimal Adaptations (and why each is unavoidable)

| # | Adaptation | Reason |
| --- | --- | --- |
| 1 | Module path `matjero` → `github.com/matjeroapps/{core,admin,seller,supplier,…}` | Six independent modules cannot share one bare module path. |
| 2 | Import rewrite `matjero/internal/X` → `github.com/matjeroapps/core/pkg/X` in siblings | Siblings cannot import Core `internal/`. |
| 3 | **`pkg/openapi/spec.go`: `schemaRefForType` hardcodes `t.PkgPath() == "matjero/packages/money"`** → must become `github.com/matjeroapps/core/packages/money` | Silent regression risk: an unchanged string makes the `Money` schema special-case fall through to struct reflection, changing every generated spec without any compile error. |
| 4 | Workspace-root `go.work` (now `/var/www/personal/matjero/go.work`) for local multi-module development | Sibling folders are not yet published repos. Kept out of production config; no absolute local paths are baked into any `go.mod`. |
| 5 | Each actor repo gets its own `scripts/check-locales.mjs` and its `web/*` `test` script path corrected | Today all four web `test` scripts invoke `node ../../scripts/check-locales.mjs`, reaching outside the workspace to the monorepo root — a path that does not exist after the split. |
| 6 | `docker/web-app.Dockerfile` per repo: `COPY web/*/package.json` list reduced to that repo's workspaces | The current file copies all four web package.json files; three of them will not exist. |
| 7 | `docker/go-app.Dockerfile` per repo: `APP_PATH` default retargeted | Default `./apps/admin-api` is invalid in seller/supplier/core. |
| 8 | Core `.github/workflows/ci.yml` rewritten to Core-only scope | Current CI runs `find apps`-based gofmt, `go run ./cmd/openapi-gen`, `docs/api` drift checks, actor docker builds, and npm workspace jobs — all of which move to actor repos. |
| 9 | `internal/platformapi` and `internal/openapi/{specs,types,spec_test}.go` deleted from Core | Their contents are fully redistributed; leaving them would create duplicate ownership. |

No other behavioural change is made. Handler bodies, SQL, error codes, route paths, role
gates, and JSON shapes are moved verbatim.

---

## 7. Ownership Classification Summary

| Component | Owner |
| --- | --- |
| `migrations/**` | CORE |
| `docker-compose.yml`, `.env.example` | CORE |
| `packages/**` (auth, config, database, events, httpx, i18n, inbox, logging, messaging, money, observability, outbox) | CORE |
| `pkg/{commerce,markets,themes,storefront,actorapi,api,openapi,actorhttp,contracts}` | CORE |
| `internal/testdb` | CORE (stays internal) |
| `apps/workers/{general-worker,scheduler}` | CORE |
| `docs/{decisions,implementation,plans}` | CORE |
| `apps/admin-api`, `web/admin`, `docs/api/admin`, admin HTTP + admin spec | ADMIN |
| `apps/seller-api`, `apps/storefront-api`, `web/seller`, `web/storefront`, `docs/api/{seller,storefront}`, seller HTTP + theme HTTP + seller/storefront specs | SELLER |
| `apps/supplier-api`, `web/supplier`, `docs/api/supplier`, supplier HTTP + supplier spec | SUPPLIER |
| `apps/integrations/suppliers` (empty) | SUPPLIER_INTEGRATIONS — README-only placeholder |
| `apps/integrations/sellers` (empty) | SELLER_INTEGRATIONS — README-only placeholder |

All previously SHARED / REQUIRES_DECISION items are resolved:

| Item | Decision |
| --- | --- |
| `internal/storefront` (imported by no app today) | CORE capability; consumed later by Seller's storefront-api. |
| `internal/testdb` | CORE, remains `internal/` — all five consumers are Core tests. |
| `docker-compose.yml` | CORE — contains only shared infrastructure (postgres, redis, rabbitmq, zitadel); no app services. |
| `web/storefront` | SELLER. |
| Theme engine | domain/persistence CORE, HTTP surface SELLER. |
| Shared actor HTTP helpers | CORE `pkg/actorhttp`. |
| Generic API DTOs | CORE `pkg/contracts`. |
| Generic OpenAPI helpers | CORE `pkg/openapi`. |
| `apps/integrations/*` | README-only ownership placeholders (both dirs are empty). |

---

## 8. Execution Order

1. This manifest (done).
2. Core internal restructure: `git mv internal/{commerce,markets,themes,storefront,actorapi,api} pkg/`; create `pkg/{actorhttp,contracts}`; split `internal/openapi`.
3. Rewrite Core imports; fix the `packages/money` PkgPath string; `go build ./... && go vet ./... && go test ./...`.
4. Create the five sibling folders with READMEs.
5. Per actor: `rsync -a` copy → `find | wc -l` + `sha256sum` + `diff -qr` verification → write split files, `go.mod`, `package.json`, `Makefile`, Dockerfiles → build/vet/test.
6. Create the workspace-root `go.work` (now at `/var/www/personal/matjero/go.work`).
7. Delete originals from Core only after each destination validates.
8. Duplicate-ownership audit.
9. Rewrite Core `Makefile` + `.github/workflows/ci.yml` to Core-only scope.
10. Final validation of all four code-bearing folders; confirm integration folders are README-only.
11. Write `docs/implementation/multi-repo-folder-split-report.md`.
12. Commit on `refactor/multi-repo-folder-split`, push, open PR to `main`, do not merge.
