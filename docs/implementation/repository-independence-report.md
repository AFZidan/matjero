# Repository Independence — Completion Report

Final deliverable of the Matjero Repository Independence Refactor.

This report records the completed migration of `core`, `seller`, `admin`, and
`supplier` from a compile-time coupled multi-repository layout to four fully
independent repositories that collaborate only through versioned runtime
HTTP/JSON contracts.

The architecture decision is recorded in
[docs/plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md](../plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md).
The Stage 0 dependency inventory is recorded in
[docs/implementation/repository-independence-inventory.md](repository-independence-inventory.md).

## Rule now enforced

> No Matjero repository may import source code, Go packages, workspace modules,
> generated build-time artifacts, vendored code, or other compile-time code from
> another Matjero repository.

> Cross-repository collaboration occurs ONLY through explicit runtime contracts,
> primarily versioned HTTP/JSON APIs.

## Relationship to earlier reports

The earlier reports
[docs/implementation/multi-repo-folder-split-report.md](multi-repo-folder-split-report.md)
and
[docs/implementation/multi-repo-publication-report.md](multi-repo-publication-report.md)
describe an architecture in which the actor repositories intentionally compiled
against Core's `packages/*` and `pkg/*` Go packages, justified by repository
privacy. Those reports remain historically accurate records of what was built at
the time. **That model is superseded by ADR-017 and is no longer the target
architecture.** They have not been rewritten, and the compile-time phase is not
presented as though it never existed.

## Pull requests

All eight pull requests are merged.

### Independence pull requests

| Repository | PR | Title intent | Merge commit |
| --- | --- | --- | --- |
| core | #13 | Core as runtime business capability boundary | `bfc9721cf628bed1985c917a66c053970e9fce1e` |
| seller | #2 | Remove Core compile-time dependency | `ad582046b3c2a93c8231cd2151c68c8c2b16d418` |
| admin | #1 | Remove Core compile-time dependency | `38c678171b77bdd65bf856e2de55229c945c5956` |
| supplier | #1 | Remove Core compile-time dependency | `ba89cd608fbd91155de976aa108f040d6a8af38c` |

### Defect pull requests raised by the final runtime smoke

Per the audit rule that a defect found during the audit must be fixed in its own
pull request and must not be hidden inside the documentation pull request, two
defect classes produced four focused pull requests.

| Repository | PR | Defect class | Merge commit |
| --- | --- | --- | --- |
| core | #14 | Non-atomic supplier composite writes | `1336f05751c2c47001d833ca201b748bfa37f793` |
| seller | #3 | Root router assembly panic | `6600052f8bcef034cdd39383a5d5b6e02f9d0cb3` |
| admin | #2 | Root router assembly panic | `c42c79bcf5698235784070ceb38e078caf38c982` |
| supplier | #2 | Root router assembly panic | `0d601c81b9ecd5b486f45a690f4e9a716efd12d5` |

## Repository state at audit close

| Repository | Branch | HEAD | Working tree |
| --- | --- | --- | --- |
| core | main | `1336f05751c2c47001d833ca201b748bfa37f793` | clean |
| seller | main | `6600052f8bcef034cdd39383a5d5b6e02f9d0cb3` | clean |
| admin | main | `c42c79bcf5698235784070ceb38e078caf38c982` | clean |
| supplier | main | `0d601c81b9ecd5b486f45a690f4e9a716efd12d5` | clean |

## Source dependency audit

Executed against fresh clones of post-merge `main` for every repository.

| Check | Result |
| --- | --- |
| Go imports of another Matjero repository | 0 across all four repositories |
| `go.mod` requires on another Matjero module | 0 |
| `go.sum` entries for another Matjero module | 0 |
| `replace` directives | 0 |
| Vendored copies of another repository | none |
| Git submodules | none |
| Docker `COPY` from a sibling repository path | none |
| CI steps cloning another Matjero repository | none |
| npm workspaces spanning repositories | none; all workspaces are intra-repository |

## Go module matrix

Every repository resolves exactly one Matjero module: its own.

| Repository | Module path | Matjero modules in `go list -m all` |
| --- | --- | --- |
| core | `github.com/matjeroapps/core` | 1 (itself) |
| seller | `github.com/matjeroapps/seller` | 1 (itself) |
| admin | `github.com/matjeroapps/admin` | 1 (itself) |
| supplier | `github.com/matjeroapps/supplier` | 1 (itself) |

`GOWORK=off go mod tidy` produces no diff in any repository.

## Database ownership audit

Core is the only repository with PostgreSQL access. No actor repository can
reach the Core database, by design and by absence of any driver.

| Repository | `jackc/pgx` files | `database/sql` files | `DATABASE_URL` references | `migrations/` directory |
| --- | --- | --- | --- | --- |
| core | owner | owner | owner | 7 migration pairs |
| seller | 0 | 0 | 0 | absent |
| admin | 0 | 0 | 0 | absent |
| supplier | 0 | 0 | 0 | absent |

Core owns 7 migration pairs and 14 Go files containing SQL. Actor repositories
contain no schema, no SQL, and no database driver of any kind.

## Fresh clone validation matrix

Each repository was cloned into an empty directory with no `go.work` present and
validated with `GOWORK=off` for every Go command.

| Repository | `go mod tidy` clean | `go build ./...` | `go vet ./...` | `go test ./...` | OpenAPI spec current |
| --- | --- | --- | --- | --- | --- |
| core | yes | PASS | PASS | PASS | current |
| seller | yes | PASS | PASS | PASS | current |
| admin | yes | PASS | PASS | PASS | current |
| supplier | yes | PASS | PASS | PASS | current |

No `go.work` file was created or required at any point during fresh clone
validation.

## Docker build matrix

All eleven container targets build from post-merge fresh clones with each
repository's own build context only.

| Repository | Target | Result |
| --- | --- | --- |
| core | core-api | PASS |
| core | general-worker | PASS |
| core | scheduler | PASS |
| seller | seller-api | PASS |
| seller | storefront-api | PASS |
| seller | seller-web | PASS |
| seller | storefront-web | PASS |
| admin | admin-api | PASS |
| admin | admin-web | PASS |
| supplier | supplier-api | PASS |
| supplier | supplier-web | PASS |

## CI regression guard matrix

Each actor repository has a dedicated `independence` job. All Go steps in these
jobs run with `GOWORK=off`.

| Guard | seller | admin | supplier |
| --- | --- | --- | --- |
| No other Matjero Go module in `go list -m all` | yes | yes | yes |
| No Matjero cross-repository import in Go sources | yes | yes | yes |
| No committed `go.work` or `go.work.sum` | yes | yes | yes |
| No `replace` directive in `go.mod` | yes | yes | yes |

Each guard was negative-tested against a scratch copy of the seller repository:
injecting a cross-repository import, a `go.work`, or a `replace` directive each
caused the corresponding guard to fail. A clean tree passes all four.

Core CI retains the `backend` job (gofmt, `go vet`, `go test`, internal OpenAPI
stale-spec check against `docs/api`), the `infrastructure` job (compose config
validation plus Docker builds for core-api, general-worker, and scheduler, and
migration presence checks), and the `security` job (module listing plus gitleaks
CLI scan).

Seller, admin, and supplier backend jobs no longer provision PostgreSQL, because
no actor test touches a database.

## Runtime contract surface

Core exposes 60 internal routes under `/internal/v1/*`, registered in
[internal/coreapi/router.go](../../internal/coreapi/router.go). The route tree is
guarded against specification drift by a test that walks the chi router and
compares it to the generated OpenAPI document.

### Service-to-service authentication

Every internal route requires both:

- `Authorization: Bearer <token>`
- `X-Matjero-Service: seller | admin | supplier`

The token is bound to the declaring caller. Comparison uses
`subtle.ConstantTimeCompare`. Every credential failure returns an identical
generic `401`; a valid caller reaching a route scoped to a different actor
returns `403`. Core refuses to start when no caller token is configured
(`ErrNoServiceCredentials`) and logs only the set of callers that have tokens,
never token values.

### Forwarded identity and tenant authority

- `X-Matjero-Subject` carries the authenticated end-user subject. Core resolves
  business identity from it and never trusts an actor-supplied business id.
- `X-Matjero-Storefront-Host` is the sole tenant authority for storefront
  routes. Core ignores the HTTP `Host` header entirely.

### Error contract

Core returns a stable set of internal error codes that actors map to their own
public responses: `not_found`, `invalid_argument`, `validation_error`,
`unauthorized`, `forbidden`, `conflict`, `market_mismatch`,
`insufficient_inventory`, `schema_mismatch`, `unsafe_content`,
`preview_unavailable`, `storefront_unavailable`, `unavailable`,
`internal_error`.

## Runtime smoke matrix

Executed against containers built from post-merge `main` clones, a dedicated
PostgreSQL database, and a throwaway RS256 OIDC stub standing in for ZITADEL,
which is not available in this environment.

### Service startup

| Service | `/healthz` | `/openapi.json` | `/docs` |
| --- | --- | --- | --- |
| core-api | 200 | n/a (internal) | n/a (internal) |
| storefront-api | 200 | 200 | 200 |
| admin-api | 200 | 200 | 200 |
| supplier-api | 200 | 200 | 200 |
| seller-api | 200 | 200 | 200 |

### Storefront tenant isolation and public contract

| Check | Result |
| --- | --- |
| Store A and store B each resolve from their own host | PASS |
| Six public storefront routes for store A | PASS |
| Store A product list excludes store B products, and the reverse | PASS |
| Store A category list excludes store B categories, and the reverse | PASS |
| Store A search excludes store B results, and the reverse | PASS |
| Store A cannot read a store B product slug (404), and the reverse | PASS |
| Store A cannot read a store B category slug (404), and the reverse | PASS |
| Listing price disclosed for both stores | PASS |
| Supplier wholesale price never disclosed publicly | PASS |
| No supplier identity, contact, or margin field in any public payload | PASS |
| Arabic and English translations negotiated correctly | PASS |
| Availability, category, price, sort, limit, and offset filters | PASS |
| Six malformed query parameter forms rejected with 400 | PASS |
| Unknown storefront host returns 404 | PASS |
| Inactive store returns 404 | PASS |

### Internal header spoofing from the public side

| Spoof attempt | Result |
| --- | --- |
| `X-Matjero-Storefront-Host` pointing at another store | ignored |
| `X-Forwarded-Host` pointing at another store | ignored |
| `X-Matjero-Service` plus a real Core token | ignored |
| `X-Matjero-Subject` naming another seller | ignored |
| `/internal/v1/*` requested through a public actor | 404 |

### Service authentication

| Check | Expected | Result |
| --- | --- | --- |
| No credentials | 401 | PASS |
| Invalid token | 401 | PASS |
| Seller token declaring `admin` | 401 | PASS |
| Admin token declaring `supplier` | 401 | PASS |
| Unknown service name | 401 | PASS |
| Seller calling an admin-scoped route | 403 | PASS |
| Supplier calling an admin-scoped route | 403 | PASS |
| Admin calling a storefront-scoped route | 403 | PASS |

### Admin runtime path (admin-api to core-api to PostgreSQL)

Overview, suppliers, sellers, stores, products, categories, and offers all
return 200. Listings and locations return 200 when given their required filter.
A supplier status mutation from `active` to `suspended` was verified directly in
PostgreSQL and then restored.

### Supplier runtime path

Profile, markets, locations, products, offers, and inventory all return 200.
Product creation, product category assignment, offer creation, inventory
adjustment, and movement listing were all verified row by row in PostgreSQL.

### Core unavailable

With `core-api` stopped:

| Check | Result |
| --- | --- |
| Actor `/healthz` still 200 (liveness independent of Core) | PASS |
| Storefront, admin, and supplier business routes return 503 | PASS |
| Response body is a generic `service temporarily unavailable` | PASS |
| No `connection refused`, `dial tcp`, host, port, container name, goroutine dump, source location, or token in any body | PASS |
| All actors recover on Core restart with no actor restart or rebuild | PASS |

## Security audit

| Check | core | seller | admin | supplier |
| --- | --- | --- | --- | --- |
| gitleaks (redacted, full history) | no leaks | no leaks | no leaks | no leaks |
| `npm audit --audit-level=high` | n/a | 0 vulnerabilities | 0 vulnerabilities | 0 vulnerabilities |
| Committed `.env`, `.pem`, `.key`, or credential file | none | none | none | none |
| Dockerfile `ARG`/`ENV` carrying a secret | none | none | none | none |

Runtime container logs for all five services were scanned for the three service
tokens, the theme preview secret, and the database credential string. None
appeared.

## Defects found and fixed during the audit

### Defect 1 — actor root router assembly panic

Every actor API entrypoint mounted the OpenAPI documentation sub-router at `/`
and then mounted the application router at `/`. chi permits only one `Mount` per
path, so the second call panicked at startup:

```
panic: chi: attempting to Mount() a handler on an existing path, '/'
```

All four actor APIs were affected: seller-api, storefront-api, admin-api, and
supplier-api.

This defect **predates the independence refactor**. The same two `Mount("/")`
calls exist at the pre-refactor commits `b2441ee` (seller), `aa8a736` (admin),
and `77ceb52` (supplier). It survived because there were **zero test files under
`apps/`** in any actor repository. No test ever assembled the entrypoint root
router, so build, vet, test, OpenAPI generation, and Docker builds were all green
while the resulting images could not start.

The fix replaces the documentation sub-router mount with direct registration on
the root router:

```go
func Register(r chi.Router, cfg RouterConfig) // registers spec + docs on the given router
func NewRouter(cfg RouterConfig) chi.Router   // retained for non-root prefixes
```

Each actor gained `internal/openapi/router_assembly_test.go` with four tests
that assemble the real root router: it must not panic, it must serve health,
docs, and application routes together, it must work with docs disabled, and
registration must be independent per router. These tests fail without the fix.

### Defect 2 — non-atomic Core supplier composite writes

Two Core composite operations wrote rows across several tables without a
transaction, so a failure partway through left permanent debris:

- Creating a supplier product with an unknown category id returned 400 but left
  the product row, its translation, and the supplier binding behind with no
  category attached.
- Creating a supplier offer with an invalid price returned 400 but left an offer
  row with neither price nor availability. An unpriced offer is invisible to
  storefront eligibility, so the supplier would see a created offer that can
  never sell.

The fix introduces [pkg/commerce/supplier_composites.go](../../pkg/commerce/supplier_composites.go):

```go
func (r Repository) CreateSupplierProductAtomically(ctx context.Context, supplierID string, draft ProductDraft) (Product, SupplierProduct, error)
func (r Repository) CreateSupplierOfferAtomically(ctx context.Context, supplierID string, draft OfferDraft) (SupplierOffer, error)
```

Every row for one logical creation is written inside a single transaction. Price
and quantity are validated before the transaction opens, so an invalid request
never begins a write. Ownership-enforcing service wrappers
`CreateSupplierProductWithDetailsForSubject` and
`CreateSupplierOfferWithDetailsForSubject` resolve the supplier from the
forwarded subject.

Eight integration tests cover both operations, including
`...RollsBackOnUnknownCategory` and `...LeavesNoUnpricedOffer`.

Post-merge runtime verification confirmed both behaviours:

| Scenario | Expected | Observed |
| --- | --- | --- |
| Product create with unknown category | 400, zero rows written | 400, product 0, binding 0, total product count unchanged |
| Product create success | 201, all row groups written | 201, product 1, translations 2, binding 1, category 1 |
| Offer create with negative price | 400, zero offer rows | 400, offer rows 0 |
| Offer create success | 201, offer plus price plus availability | 201, offer 1, price 1, availability 1 |

## Local Go workspace

`/var/www/personal/matjero/go.work` still exists outside every repository. It
contains only `use` directives for the four sibling checkouts and **no**
`replace` directive. It is not tracked by any repository and is listed in
`.gitignore`.

No validation step in this audit required it: every Go command was run with
`GOWORK=off`, and every fresh clone was validated in a directory where no
`go.work` was reachable.

The workspace file is now purely an optional developer convenience for
cross-repository navigation in an editor. It must never be committed, and it must
never gain a `replace` directive. It should not be removed without a clear
reason.

## Runtime transport rule: HTTP/JSON versus RabbitMQ

This section records the rule. No messaging implementation was added by this
refactor.

- **Synchronous capability calls use versioned HTTP/JSON.** When an actor needs
  a business answer to serve the request it is currently handling, it calls a
  Core `/internal/v1/*` route and waits.
- **RabbitMQ is for asynchronous work**: background jobs, fire-and-forget
  commands, deferred processing, and domain-event delivery. It is not a
  request/response substitute for a capability call.
- **The Core PostgreSQL database remains Core-owned.** No actor may connect to
  it under any circumstance.
- **RabbitMQ must never become a back door to arbitrary SQL.** A message whose
  payload is a query, a statement, or an instruction to run arbitrary SQL against
  the Core database is a violation of ADR-017 in exactly the same way a direct
  connection would be. Messages carry domain intent, not database access.

## Known limitations

1. **ZITADEL was not available in this environment.** Actor end-user
   authentication was exercised against a throwaway RS256 OIDC stub that issues
   tokens with the same subject, audience, and roles shape. JWT verification
   logic, audience binding, and role gating were exercised; ZITADEL-specific
   behaviour such as introspection, token revocation, and organisation claims was
   not.
2. **`/v1/admin/listings` and `/v1/admin/locations` require a filter.** Both
   return 400 without `store_id` and `supplier_id` respectively. This is a
   pre-existing Core repository contract: `ListSellerListings` requires a store
   id and `ListFulfillmentLocations` requires a supplier id. It is not a
   regression introduced by this refactor and was left unchanged.
3. **Older non-atomic supplier composite methods remain in
   `pkg/commerce`.** They are no longer used by the internal API surface. They
   were deliberately left in place rather than removed in a fix pull request, and
   should be retired in follow-up work.
4. **No cross-repository contract test suite exists yet.** Core's OpenAPI
   document is the contract, and Core has a drift guard against its own router,
   but no automated check verifies that a given actor's client matches a given
   Core version. Drift would currently surface at runtime.
5. **Independence guards are per-repository CI jobs, not an organisation
   policy.** They catch violations on pull requests within each repository. They
   cannot prevent a violation introduced in a repository whose workflow has been
   modified in the same change.

## Final independence matrix

| Repository | Other Matjero Go Modules | Core DB Access | Fresh Clone | Docker | CI |
| --- | --- | --- | --- | --- | --- |
| core | 0 | OWNER | PASS | PASS | GREEN |
| seller | 0 | NO | PASS | PASS | GREEN |
| admin | 0 | NO | PASS | PASS | GREEN |
| supplier | 0 | NO | PASS | PASS | GREEN |

## P4.4 readiness

Every dimension of the repository independence refactor passes:

- Eight pull requests merged across four repositories.
- Zero cross-repository compile-time dependencies of any kind.
- Every repository builds, vets, tests, generates its OpenAPI document, and
  containerises from a clean clone with no other Matjero repository present and
  no Go workspace.
- Core is the only repository with database access.
- Service-to-service authentication is fail-closed, caller-bound,
  constant-time, and non-disclosing.
- Storefront tenant resolution is authoritative on the Core side and immune to
  the spoofing attempts exercised.
- Both defect classes found during the audit are fixed, merged, covered by
  tests, and re-verified at runtime against merged code.
- CI guards exist in all three actor repositories and were negative-tested.

P4.4 may start from these four `main` commits.
