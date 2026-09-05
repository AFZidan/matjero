# Multi-Repository Folder Split — Implementation Report

> **Superseded architecture notice.** This report accurately records what was
> built at the time: actor repositories extracted into siblings that still
> compiled against Core's Go packages. That compile-time coupling model is
> superseded by
> [ADR-017](../plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md)
> and no longer describes the target architecture. The report has intentionally
> not been rewritten. See
> [repository-independence-report.md](repository-independence-report.md) for the
> current model.

## 1. Summary

The monorepo was split into six sibling folders, each ready to become an
independent GitHub repository. The work was a **filesystem move plus minimal
adaptation**, not a rewrite: every extracted Go file, web application file, and
OpenAPI document originates from the monorepo at source SHA
`8378f32590dba083050ac2b619ba5cbf511a38dd`.

The table below records the folder names **as they were at the time of the
split**. The folders have since been relocated into a single workspace root and
renamed; see §1.1 for the current layout.

| Folder at split time | GitHub repository | Go module | Status |
| --- | --- | --- | --- |
| `drop-shipping/` | [`matjeroapps/core`](https://github.com/matjeroapps/core) | `github.com/matjeroapps/core` | Code-bearing |
| `matjero-admin/` | [`matjeroapps/admin`](https://github.com/matjeroapps/admin) | `github.com/matjeroapps/admin` | Code-bearing |
| `matjero-seller/` | [`matjeroapps/seller`](https://github.com/matjeroapps/seller) | `github.com/matjeroapps/seller` | Code-bearing |
| `matjero-supplier/` | [`matjeroapps/supplier`](https://github.com/matjeroapps/supplier) | `github.com/matjeroapps/supplier` | Code-bearing |
| `matjero-supplier-integrations/` | [`matjeroapps/supplier-hub`](https://github.com/matjeroapps/supplier-hub) | — | README-only placeholder |
| `matjero-seller-integrations/` | [`matjeroapps/seller-hub`](https://github.com/matjeroapps/seller-hub) | — | README-only placeholder |

At split time the `drop-shipping/` folder name was left unchanged on disk; it is
the historical repository, now published as `matjeroapps/core`, and its Go module
path was renamed accordingly.

### 1.1 Current canonical local layout

All six folders now live under one workspace root:

```
/var/www/personal/matjero/
├── core/            → matjeroapps/core          (was drop-shipping/)
├── admin/           → matjeroapps/admin         (was matjero-admin/)
├── seller/          → matjeroapps/seller        (was matjero-seller/)
├── supplier/        → matjeroapps/supplier      (was matjero-supplier/)
├── seller-hub/      → matjeroapps/seller-hub    (was matjero-seller-integrations/)
├── supplier-hub/    → matjeroapps/supplier-hub  (was matjero-supplier-integrations/)
├── go.work
└── go.work.sum
```

`supplier-hub` owns supplier-side external commerce integration/connectors and
`seller-hub` owns seller-side external store integration/connectors. Neither is a
dashboard.

**Storefront** is owned by `matjeroapps/seller` (both the Storefront API and the
Next.js storefront web app). **Infrastructure** and **shared contracts** stay in
Core. No separate storefront, infrastructure, or contracts repository was
created.

## 2. Source of truth

- Source SHA: `8378f32590dba083050ac2b619ba5cbf511a38dd`
- Branch: `refactor/multi-repo-folder-split`
- Remote: `git@github.com:matjeroapps/core.git`, base branch `main`
- No `git init`, `git remote add`, or `gh repo create` was run in any sibling
  folder. The five sibling folders are plain directories with no git metadata.

## 3. Core: `internal/` → `pkg/` moves

Six packages were promoted from `internal/` to `pkg/` because sibling
repositories legitimately need them. They were moved with `git mv`, so git
records them as renames (similarity 86–100%).

| From | To | Reason |
| --- | --- | --- |
| `internal/actorapi/` | `modules/actorapi/` | Shared router construction used by every actor API |
| `internal/api/` | `modules/api/` | Shared bootstrap used by every actor `main.go` |
| `internal/commerce/` | `modules/commerce/` | Commerce domain, repository, service — stays in Core, consumed by all actors |
| `internal/markets/` | `modules/markets/` | Market reference data domain |
| `internal/storefront/` | `modules/storefront/` | Store/host resolution used by the Storefront API |
| `internal/themes/` | `modules/themes/` | Theme domain, repository, validation, persistence — stays in Core |

`internal/testdb/` deliberately **stayed internal**: it is a Core-only
integration-test helper and no sibling needs it.

Not every `internal/` directory was promoted. This was a deliberate, narrow set.

## 4. Core: new public boundaries

Three new packages were created in Core to expose the minimum surface the actor
repositories need. Each exists because the corresponding code was previously
package-private inside `internal/platformapi/` and is now consumed across a
repository boundary.

### `modules/actorhttp/` — shared actor HTTP helpers

Previously unexported helpers in `internal/platformapi`. Exported verbatim with
identical behavior:

| Export | Behavior preserved from monorepo |
| --- | --- |
| `Page{Limit,Offset}`, `ParsePage(r)` | limit defaults to 25 when `<=0` or `>100`; offset clamped to `>=0` |
| `SubjectFrom(r)` | errors `"missing principal"` / `"missing principal subject"` |
| `DecodeJSON(w,r,dst) bool` | 400 with code `invalid_json` |
| `TranslationInput{Locale,Name,Description}` | unchanged shape |
| `UpdateStatusHandler(w,r,fn)` | unchanged |
| `ResolveSupplierID`, `ResolveSellerID` | unchanged |
| `WriteCommerceError(w,err)` | `ErrInvalidInput`→400 `validation_error`; `ErrNotFound`→404 `not_found`; `ErrConflict`→409 `conflict`; `ErrMarketMismatch`→409 `market_mismatch`; `ErrInsufficientInventory`→409 `insufficient_inventory`; default→500 `internal_error` |

### `modules/contracts/` — generic response envelopes

Five DTOs shared by more than one actor, extracted so they are defined exactly
once: `CollectionResponse[T]` (`items`), `StatusResponse` (`status`),
`CountResponse` (`counts`), `MarketsResponse` (`markets`),
`StatusUpdateRequest` (`status`).

### `modules/openapi/` — code-first spec generation

Moved from `internal/openapi/` and split so that the generic machinery is public
and the per-actor documents live in each sibling.

| File | Content | Origin |
| --- | --- | --- |
| `modules/openapi/spec.go` | `RouteSpec`, `ParameterSpec`, `ResponseSpec`, `DocumentSpec`, `BuildDocument`, `MarshalDocument`, `ValidateDocument`, `NewSpecHandler` | `git mv` from `internal/openapi/spec.go` (98% similarity) |
| `modules/openapi/http.go` | `RouterConfig`, `NewRouter` (defaults `/openapi.json`, `/docs`) | `git mv` from `internal/openapi/http.go` (100%) |
| `modules/openapi/routes.go` | `ActorRoutes`, `CommonTags`, `ListResponses[T]`, `AuthReadResponses`, `AuthCreatedResponses`, `AuthOKResponses`, `OKResponse`, `CreatedResponse`, `ErrorResponse`, `LimitParam`, `OffsetParam`, `PathStringParam`, `StringParam` | Extracted from the shared portion of the monorepo `internal/openapi` package |
| `modules/openapi/http_test.go` | `TestDocsRouterEnabledDisabled`, `TestActorRoutesAuthToggle` | Extracted from the monorepo spec tests |

One value change was required inside `modules/openapi/spec.go`: the money type's
reflected `PkgPath` is now
`github.com/matjeroapps/core/packages/money`. This does not alter generated
output, which is proven byte-identical in section 8.

## 5. Core: module path change

`go.mod` module line changed from bare `matjero` to
`github.com/matjeroapps/core`. The import path was rewritten across **42 Go
files**. No other semantic change accompanied the rewrite.

## 6. Extracted paths

### `matjeroapps/admin`

| Destination | Origin in monorepo |
| --- | --- |
| `apps/admin-api/main.go` | `apps/admin-api/main.go` |
| `internal/adminapi/router.go` | admin portion of `internal/platformapi/` (18 funcs, `RegisterAdminRoutes` with 17 routes) |
| `internal/openapi/specs.go` | admin portion of `internal/openapi/` (`BuildAdminSpec`, `adminRoutes`) |
| `internal/openapi/aliases.go` | **new**, minimal adaptation — see section 7 |
| `internal/openapi/spec_test.go` | admin portion of `internal/openapi/spec_test.go` |
| `cmd/openapi-gen/main.go` | `cmd/openapi-gen/main.go`, retargeted to `docs/api/admin/openapi.json` |
| `docs/api/admin/openapi.json` | `docs/api/admin/openapi.json` |
| `web/admin/` (8 files) | `web/admin/` |
| `scripts/check-locales.mjs` | `scripts/check-locales.mjs` |
| `docker/go-app.Dockerfile`, `docker/web-app.Dockerfile` | `docker/` |

### `matjeroapps/seller`

| Destination | Origin in monorepo |
| --- | --- |
| `apps/seller-api/main.go` | `apps/seller-api/main.go` |
| `apps/storefront-api/main.go` | `apps/storefront-api/main.go` |
| `internal/sellerapi/router.go` | seller portion of `internal/platformapi/` (10 funcs, `RegisterSellerRoutes` with 9 routes) |
| `internal/sellerapi/themes.go` | theme HTTP surface from `internal/platformapi/` (verbatim copy + import adaptation) |
| `internal/sellerapi/contracts.go` | seller-only DTOs from `internal/platformapi/` |
| `internal/openapi/specs.go` | seller + storefront portions of `internal/openapi/` (`BuildSellerSpec`, `BuildStorefrontSpec`, `sellerRoutes`) |
| `internal/openapi/spec_test.go` | seller + storefront portions of `internal/openapi/spec_test.go` |
| `cmd/openapi-gen/main.go` | `cmd/openapi-gen/main.go`, retargeted to `docs/api/seller/` and `docs/api/storefront/` |
| `docs/api/seller/openapi.json`, `docs/api/storefront/openapi.json` | same paths |
| `web/seller/` (8 files), `web/storefront/` (9 files) | same paths |
| `scripts/check-locales.mjs`, `docker/` | same paths |

### `matjeroapps/supplier`

| Destination | Origin in monorepo |
| --- | --- |
| `apps/supplier-api/main.go` | `apps/supplier-api/main.go` |
| `internal/supplierapi/router.go` | supplier portion of `internal/platformapi/` (400 lines, 15 funcs) |
| `internal/supplierapi/contracts.go` | supplier-only DTOs from `internal/platformapi/` (11 DTOs) |
| `internal/openapi/specs.go` | supplier portion of `internal/openapi/` (`BuildSupplierSpec`, `supplierRoutes`) |
| `internal/openapi/spec_test.go` | supplier portion of `internal/openapi/spec_test.go` |
| `cmd/openapi-gen/main.go` | `cmd/openapi-gen/main.go`, retargeted to `docs/api/supplier/openapi.json` |
| `docs/api/supplier/openapi.json` | same path |
| `web/supplier/` (8 files) | same path |
| `scripts/check-locales.mjs`, `docker/` | same paths |

### Removed from Core

52 tracked files were deleted from Core, all of which now live in a sibling:

`internal/platformapi/`, `internal/openapi/`, `internal/phase3api/`,
`cmd/openapi-gen/`, `apps/admin-api/`, `apps/seller-api/`,
`apps/supplier-api/`, `apps/storefront-api/`, `web/admin/`, `web/seller/`,
`web/supplier/`, `web/storefront/`, `docs/api/`, `scripts/check-locales.mjs`,
plus the root `package.json`, `package-lock.json` and
`docker/web-app.Dockerfile`, which had no remaining purpose once Core stopped
shipping a frontend.

Two empty untracked placeholder directories (`apps/integrations/suppliers`,
`apps/integrations/sellers`) were removed because their scope now belongs to the
two integration repositories. The twelve empty `internal/*` domain placeholder
directories are untracked and were left as-is; they predate this task and git
does not record them.

## 7. Minimal adaptations (deliberate, documented)

Everything below is an adaptation forced by the repository boundary, not a
redesign.

1. **Sibling `internal/openapi/aliases.go`.** Each sibling's extracted
   `specs.go` referenced package-private identifiers (`actorRoutes`,
   `authReadResponses`, `limitParam`, …) that now live in Core's public
   `modules/openapi`. Rather than rewriting hundreds of call sites, each sibling has
   a small `aliases.go` that re-binds the Core exports to their original
   lowercase names via type aliases and function values. This keeps the
   extracted route definitions byte-for-byte closer to the monorepo original.

2. **Actor DTO qualification (option a).** Each sibling's `internal/openapi`
   imports its own `internal/{admin,seller,supplier}api` package and route
   bodies were qualified as `supplierapi.X{}` / `sellerapi.X{}`. The alternative
   — duplicating DTO aliases inside `internal/openapi` — was rejected because it
   would create two definitions of the same type. The five genuinely shared
   envelopes are qualified as `contracts.X{}` against Core.

3. **Frontend shared API client.** `web/seller/src/lib/api.ts` and
   `web/supplier/src/lib/api.ts` re-exported from
   `../../../admin/src/lib/api` — a cross-app relative import that cannot
   survive the split. The 33-line client was inlined into each app. It is
   identical source in all three apps; no behavior change.

4. **`docker/go-app.Dockerfile` `ARG APP_PATH` defaults** were retargeted per
   repository: `./apps/admin-api`, `./apps/seller-api`,
   `./apps/supplier-api`, and in Core `./apps/workers/general-worker`.

5. **Per-repository `package.json` workspaces** were narrowed to the web apps
   each repository actually owns: admin `["web/admin"]`, supplier
   `["web/supplier"]`, seller `["web/seller","web/storefront"]`.

No API-to-API HTTP coupling was introduced to solve the split. Every
cross-repository dependency is a compile-time Go module dependency on Core.

> **Superseded.** The compile-time model described above was later replaced by
> the Repository Independence Rule in
> [ADR-017](../plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md).
> Actor repositories no longer import Core Go packages; they call the Core
> internal HTTP API (`apps/core-api`, `/internal/v1`). The rest of this report
> remains an accurate record of the folder split as it was performed.

## 8. OpenAPI drift proof

Each sibling's spec generator was run and its output compared against the
committed monorepo document at the source SHA. All four are **byte-identical**.

| Document | Bytes | MD5 (regenerated) | MD5 (monorepo `HEAD`) | Result |
| --- | --- | --- | --- | --- |
| admin | 140412 | `dc697e80c68ce26fe6015f5c192f7821` | `dc697e80c68ce26fe6015f5c192f7821` | IDENTICAL |
| seller | 160841 | `1130b63ddddf2fe9a39ddd94c465e001` | `1130b63ddddf2fe9a39ddd94c465e001` | IDENTICAL |
| supplier | 134286 | `c14df0b4cd2adc3357370a5bf4d57137` | `c14df0b4cd2adc3357370a5bf4d57137` | IDENTICAL |
| storefront | 25913 | `b9cfa15d51a96f16d5efeeb2771b3cf0` | `b9cfa15d51a96f16d5efeeb2771b3cf0` | IDENTICAL |

This is the strongest available evidence that the split preserved behavior: the
generated contract for every actor is unchanged down to the byte.

## 9. Cross-repository module resolution

Each sibling declares `require github.com/matjeroapps/core v0.0.0` and
contains **no `replace` directive**, so nothing filesystem-specific leaks into a
published `go.mod`.

Local development uses an untracked `go.work` at the workspace root,
`/var/www/personal/matjero/go.work`, which lives outside every repository:

```
go 1.26

use (
	./core
	./admin
	./seller
	./supplier
)
```

`seller-hub` and `supplier-hub` are intentionally absent: they contain no
`go.mod` yet.

Two resolution details worth recording:

- While Core was still unpublished, the `go.work` replace **had to be
  version-qualified**. `replace github.com/matjeroapps/core => ./core`
  fails with `workspace module … is replaced at all versions in the go.work
  file`; omitting the replace entirely failed with `Repository not found`. Now
  that Core is published to `matjeroapps/core` the replace is dropped and the
  `use` directives alone are sufficient.
- `go mod tidy` ignores `go.work` replaces for network resolution, so the
  indirect requires were populated with a temporary in-module replace that is
  then dropped:

  ```sh
  go mod edit -replace github.com/matjeroapps/core=../core
  GOWORK=off GOFLAGS=-mod=mod go mod tidy
  go mod edit -dropreplace github.com/matjeroapps/core
  ```

  Each sibling ended with 35 `// indirect` requires and no `replace`.

Once Core is published, each sibling pins a real Core version (tag or
pseudo-version) and the `go.work` file becomes optional.

## 10. Validation results

### Go — all four modules

| Module | `go build ./...` | `go vet ./...` | `gofmt -l .` | `go test ./...` |
| --- | --- | --- | --- | --- |
| `core` (Core) | pass | pass | clean | pass |
| `matjeroapps/admin` | pass | pass | clean | pass |
| `matjeroapps/seller` | pass | pass | clean | pass |
| `matjeroapps/supplier` | pass | pass | clean | pass |

Core test packages green: `modules/actorapi`, `modules/commerce`, `modules/markets`,
`modules/openapi`, `modules/storefront`, `modules/themes`,
`packages/{auth,config,events,httpx,i18n,money}`.

### Frontend — three actor repositories

| Repository | `npm install` | `npm run lint` | `npm run typecheck` | `npm run test` |
| --- | --- | --- | --- | --- |
| `matjeroapps/admin` | pass | pass | pass | pass (`admin: locale foundation ok`) |
| `matjeroapps/seller` | pass | pass | pass | pass |
| `matjeroapps/supplier` | pass | pass | pass | pass (`supplier: locale foundation ok`) |

The locale check resolves `../../scripts/check-locales.mjs` from inside each web
workspace, which is why `scripts/check-locales.mjs` was copied into every
sibling.

### Boundary and hygiene checks

| Check | Result |
| --- | --- |
| Sibling imports of `core/internal/...` | none |
| Exported type defined in two places across Core + siblings | none (only `Dependencies`, once per sibling in its own actor package — three distinct types, no shared definition) |
| Phase identifiers in production/runtime filenames | none (only historical `docs/implementation/phase-*.md`, which are documentation) |
| Duplicated Go `package` clauses (heredoc artifact) | none |
| Web app file counts vs monorepo baseline | admin 8/8, seller 8/8, storefront 9/9, supplier 8/8 |
| Integration folders | README-only, one file each |
| `git init` / remotes in sibling folders | none |

### Copy fidelity

All 12 asset copy pairs were verified byte-identical with `diff -qr` before the
Core originals were deleted, following the mandated
`inventory → create → copy → validate → compare → delete` sequence.

## 11. Core infrastructure updates

`Makefile` and `.github/workflows/ci.yml` were narrowed to Core-only scope,
because the actor APIs, `cmd/openapi-gen`, `internal/openapi`, `docs/api`, and
all four web apps have left the repository.

**`Makefile`**: `test` is now `go-test docker-config migrate-check`. The four
frontend targets were removed. `docker-build` now builds the two workers
(`general-worker`, `scheduler`) instead of the admin API and two web apps.

**CI**: the `frontend` job was removed entirely. In `backend`, the gofmt glob
became `find apps packages pkg internal -name '*.go'` and the three
OpenAPI-drift steps were removed. The `infrastructure` job now builds the two
worker images and keeps `docker compose config --quiet` plus the migration file
checks. `security` keeps `go list -m all` and gitleaks but drops `npm ci` /
`npm audit`.

**`docker/go-app.Dockerfile`**: `ARG APP_PATH` default retargeted to
`./apps/workers/general-worker`.

**`docker-compose.yml`** is unchanged and stays in Core as the single source of
local infrastructure (postgres 17, redis 7, rabbitmq 4, zitadel v3.3.2). All
sibling READMEs point at it.

## 12. Invariants preserved

- **Migrations remain centralized in Core `migrations/`**, including
  `000007_theme_engine_schema`. No sibling contains a migration file, and every
  sibling README states this explicitly.
- **Theme domain stays in Core** (`modules/themes`: domain, repository, validation,
  persistence). `matjeroapps/seller` owns only the theme HTTP surface, the dashboard
  screens, and storefront rendering. The `THEME_PREVIEW_SECRET` fail-closed 503
  `preview_unavailable` behavior is unchanged and documented in the seller
  README.
- **Commerce business logic stays in Core**: Store, SellerListing, Product,
  Inventory, Order, Payment, and theme persistence.
- **Runtime deployability preserved**: `seller-api`, `seller-web`,
  `storefront-api`, and `storefront-web` remain four separate build and run
  targets inside `matjeroapps/seller`.

## 13. Change statistics (Core commit)

```
112 files changed, 814 insertions(+), 23124 deletions(-)
 41 renames (internal/ → pkg/)
 52 deletions (extracted to siblings)
 14 modifications
  5 additions (modules/actorhttp, modules/contracts, modules/openapi/routes.go,
               modules/openapi/http_test.go, extraction manifest)
```

The large deletion count reflects code moving out to siblings, not code being
lost. Every deleted file has a validated destination.

## 14. Repository publication

The six GitHub repositories now exist under the `matjeroapps` organization:

| GitHub repository | Canonical Go module |
| --- | --- |
| <https://github.com/matjeroapps/core> | `github.com/matjeroapps/core` |
| <https://github.com/matjeroapps/admin> | `github.com/matjeroapps/admin` |
| <https://github.com/matjeroapps/seller> | `github.com/matjeroapps/seller` |
| <https://github.com/matjeroapps/supplier> | `github.com/matjeroapps/supplier` |
| <https://github.com/matjeroapps/supplier-hub> | — (no Go code yet) |
| <https://github.com/matjeroapps/seller-hub> | — (no Go code yet) |

Core is the historical repository: it was transferred from `AFZidan/matjero` to
`matjeroapps/core` and retains all history and pull requests. The five sibling
repositories were created empty and receive a single initial commit carrying the
extracted baseline, with provenance recorded in the commit body (source
repository, source split PR #9, pre-extraction baseline SHA, and the Core
dependency revision).

That sequence is now complete. All five siblings are pushed, each code-bearing
sibling pins Core at pseudo-version `v0.0.0-20260831221729-6a3a841a5736` with no
committed `replace` directive, every actor CI is green, and fresh clones build
without the workspace. The outcome is recorded in
[`multi-repo-publication-report.md`](multi-repo-publication-report.md).

Two invariants continue to apply:

1. Hub repositories stay README-only until connector work begins; no fabricated
   application CI is added for them.
2. Contributors keep the untracked `go.work` at the workspace root for local
   cross-repository development. It is git-ignored and must never be committed.

