# Multi-Repository Publication — Implementation Report

## 1. Summary

The six Matjero repositories are published, wired to a real remote Core
dependency, and validated independently of the local Go workspace. This closes
the sequence opened by the folder split ([`multi-repo-folder-split-report.md`](multi-repo-folder-split-report.md),
Core PR #9) and the workspace normalization (Core PR #10).

This step was **publish, wire and validate** — no application, API, frontend or
domain logic was rewritten. The only source change made here was replacing the
temporary `github.com/matjeroapps/core v0.0.0` placeholder with the published
Core pseudo-version in the three actor modules.

## 2. Core

| Field | Value |
| --- | --- |
| Repository | <https://github.com/matjeroapps/core> |
| Go module | `github.com/matjeroapps/core` |
| `main` SHA | `6a3a841a5736971500c5fe523abac31da39ab223` |
| Merge commit subject | Merge pull request #10 from matjeroapps/docs/normalize-workspace-paths |
| Pseudo-version | `v0.0.0-20260831221729-6a3a841a5736` |
| CI on `main` | run `33445524764` — success |

No release tag was created. A repository-extraction baseline is not a product
release, so siblings pin an exact pseudo-version resolved by Go from the
canonical `main` commit:

```
GOWORK=off go list -m github.com/matjeroapps/core@6a3a841a5736971500c5fe523abac31da39ab223
github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736
```

Semantic release tags will be introduced deliberately later.

## 3. Published repositories

| Repository | Go module | Published SHA | Scope |
| --- | --- | --- | --- |
| [`matjeroapps/core`](https://github.com/matjeroapps/core) | `github.com/matjeroapps/core` | `6a3a841a5736971500c5fe523abac31da39ab223` | Domain, shared packages, migrations, infrastructure |
| [`matjeroapps/admin`](https://github.com/matjeroapps/admin) | `github.com/matjeroapps/admin` | `aa8a7362b5e17aba8c8c6207366e1efc1284727a` | `admin-api`, `admin-web` |
| [`matjeroapps/seller`](https://github.com/matjeroapps/seller) | `github.com/matjeroapps/seller` | `c0a6ca4a9e9b7587c5726399d08835f82ed817f2` | `seller-api`, `seller-web`, `storefront-api`, `storefront-web` |
| [`matjeroapps/supplier`](https://github.com/matjeroapps/supplier) | `github.com/matjeroapps/supplier` | `77ceb5257841623d58f6f73f7e5322c3caedc7ed` | `supplier-api`, `supplier-web` |
| [`matjeroapps/seller-hub`](https://github.com/matjeroapps/seller-hub) | — (no Go code yet) | `e282c9e305f0cec49450d13667361c2c87a4c701` | README-only ownership statement |
| [`matjeroapps/supplier-hub`](https://github.com/matjeroapps/supplier-hub) | — (no Go code yet) | `bedf4517a153aae06cf17ed109c5923d9d4845be` | README-only ownership statement |

Each sibling repository was created new and received its extracted baseline as a
normal fast-forward push to `main`. No history was overwritten and no force push
was used anywhere.

Admin history is `first commit` → `initial commit` → `chore: pin Matjero Core to
published pseudo-version`; Seller and Supplier carry a single extraction commit
plus (for Seller) the Core pin. The bootstrap exception is now closed: further
changes to any repository go through branch → PR → CI → manual merge.

## 4. Core dependency wiring

All three code-bearing siblings pin the identical Core version:

```
github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736
```

The temporary workspace `replace github.com/matjeroapps/core v0.0.0 => ./core`
was removed from the local `go.work`. `use` alone is sufficient for local
cross-repository development, and it leaves no filesystem path in any published
`go.mod`.

`GOWORK=off go mod tidy` then `GOWORK=off go list -m all` was run per repository
so the committed `go.mod`/`go.sum` are proven to resolve Core from GitHub rather
than from the sibling checkout:

| Repository | `GOWORK=off go list -m all` — Core entry |
| --- | --- |
| `admin` | `github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736` |
| `seller` | `github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736` |
| `supplier` | `github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736` |

Recorded `go.sum` hashes:

```
github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736 h1:Z1ZW8AefANmUhnDPdM5siGXK168ZrkmJV5MY787ysig=
github.com/matjeroapps/core v0.0.0-20260831221729-6a3a841a5736/go.mod h1:cvKv9a3egii4vJ6tA1pNXJrbo/znlOUR4znMDybityI=
```

Zero committed `replace` directives exist in any repository.

## 5. GitHub CI

CI ran on the actual remote repositories, not only locally.

| Repository | Run | Jobs | Result |
| --- | --- | --- | --- |
| `admin` | `33448846677` | backend, frontend, openapi, security | all success |
| `seller` | `33448840822` | backend, frontend, openapi, security | all success |
| `supplier` | `33448294324` | backend, frontend, openapi, security | all success |
| `core` | `33445524764` | CI | success |

No job required a fix: the first run on each published head was green.

Hub repositories have no workflows, matching their README-only status. No
application CI was fabricated for them.

Every security job installs the pinned MIT-licensed gitleaks CLI
(`v8.30.1`) exactly as Core does, rather than `gitleaks-action@v3`, which
requires a paid license under organization accounts.

## 6. Fresh clone validation

`core`, `admin`, `seller` and `supplier` were cloned into
`/tmp/matjero-independent-test`, outside the workspace root, with no `go.work` or
`go.work.sum` copied in. `go env GOWORK` was empty there.

| Repository | Checks | Result |
| --- | --- | --- |
| `core` | `go build`, `go vet`, `go test`, `docker compose config --quiet`, `make migrate-check` | pass |
| `admin` | `go list -m all`, `go build`, `go vet`, `go test`, OpenAPI stale check, `npm ci/lint/typecheck/test/build`, `npm audit --audit-level=high` | pass |
| `seller` | same, covering `seller-api`, `storefront-api`, `seller-web`, `storefront-web` and both OpenAPI documents | pass |
| `supplier` | same, covering `supplier-api` and `supplier-web` | pass |

Each actor clone downloaded Core from the module proxy; none required a sibling
`../core` checkout to exist.

Seller retains four independent applications after publication: `apps/seller-api`,
`apps/storefront-api`, `web/seller` and `web/storefront`. They were not merged.

## 7. OpenAPI drift

Every actor OpenAPI document was regenerated with each repository's own
generator (`go run ./cmd/openapi-gen`) both in the workspace and in the fresh
clone, followed by `git diff --exit-code -- docs/api`.

| Document | Result |
| --- | --- |
| `admin/docs/api/admin/openapi.json` | no diff |
| `seller/docs/api/seller/openapi.json` | no diff |
| `seller/docs/api/storefront/openapi.json` | no diff |
| `supplier/docs/api/supplier/openapi.json` | no diff |

Publication changed no HTTP contract. Pinning a real Core version produced zero
specification drift, including no metadata churn.

## 8. Security and hygiene audit

| Check | Scope | Result |
| --- | --- | --- |
| gitleaks `v8.30.1`, git history | admin, seller, supplier | no leaks |
| gitleaks `v8.30.1`, working tree | admin, supplier | no leaks |
| gitleaks `v8.30.1`, working tree | seller | 6 findings, all in untracked, git-ignored `web/storefront/.next/` build output; nothing committed |
| `npm audit --audit-level=high` | admin, seller, supplier | 0 vulnerabilities |
| Secret files | all siblings | only `.env.example` templates with local placeholder credentials; no `.env`, `*.pem` or SSH keys |
| `github.com/AFZidan/...` imports | all siblings | none |
| `github.com/matjeroapps/core/internal/...` imports | all siblings | none |
| Absolute local paths (`/var/www/personal/...`) | committed files | none; matches existed only in ignored `.next/` artifacts |
| Relative sibling paths (`../core` and friends) | committed files | none |
| Production filenames containing `phase4`, `phase-4`, `phase_4`, `p4_` | all siblings | none |

## 9. Dependency direction

```
admin ──────► core
seller ─────► core
supplier ───► core
```

No cross-actor imports exist: Admin imports neither Seller nor Supplier, Seller
imports neither Admin nor Supplier, and Supplier imports neither Admin nor
Seller. The hub repositories declare no Go module and fabricate no Core
dependency; they will depend on Core public packages when connector work begins.

## 10. Remote content scope

| Repository | Verified contents |
| --- | --- |
| `core` | zero `*-api` / `*-web` actor application paths |
| `admin` | `apps/admin-api`, `web/admin` only |
| `seller` | `apps/seller-api`, `apps/storefront-api`, `web/seller`, `web/storefront` |
| `supplier` | `apps/supplier-api`, `web/supplier` |
| `seller-hub` | `README.md` only |
| `supplier-hub` | `README.md` only |

## 11. Final workspace state

`/var/www/personal/matjero/go.work` now contains only a `use` block:

```
use (
    ./admin
    ./core
    ./seller
    ./supplier
)
```

`go work sync` succeeds, and `go build ./...` still works in each sibling with
the workspace active, so local cross-repository development is unaffected.

`go.work` and `go.work.sum` remain untracked in all four Go repositories and are
git-ignored by each sibling. They are local development artifacts and must never
be committed.
