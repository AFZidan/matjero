# ADR-017: Repository Independence and Runtime Service Boundaries

## Status

Accepted

Supersedes the compile-time coupling model described in
`docs/implementation/multi-repo-folder-split-report.md` and
`docs/implementation/multi-repo-publication-report.md`. Those reports remain
historically accurate; the model they describe is no longer the target
architecture.

## Context

The Matjero multi-repository split extracted `admin`, `seller`, and `supplier`
into separate GitHub repositories, but deliberately left them compiling against
Core's public Go packages (`packages/*`, `pkg/*`). At the time this was
presented as acceptable because the repositories were private and the shared
packages were "generic technical code plus shared business models".

That reasoning does not hold:

1. **Repository privacy is not an architecture.** It is an access-control
   setting that can change, and it couples the ability to build an actor
   repository to credentials for a different repository. A private-module
   workaround (Git credentials, `GOPRIVATE`, netrc in CI) makes every actor
   build, Docker build, and CI job depend on secret material that has nothing to
   do with the actor's own code.
2. **Compile-time coupling makes independent deployment impossible.** An actor
   cannot be built, tested, linted, or containerised from a clean clone without
   resolving another repository's module. That breaks fresh-clone builds,
   reproducible Docker builds, and per-repository CI.
3. **Shared business models leak ownership.** Actors construct
   `commerce.Repository` and query Core-owned tables directly. That makes the
   commerce schema a de facto published API and lets business invariants be
   bypassed by any actor that can reach the database.
4. **A shared code repository would only relocate the problem.** It reintroduces
   a coupling point that every repository must build against, and it becomes a
   cross-team bottleneck for changes that are local to one actor.

The dependency inventory in
`docs/implementation/repository-independence-inventory.md` records every
coupling that must be removed.

## Decision

### The Repository Independence Rule

No Matjero repository may import source code, Go packages, workspace modules,
generated build-time artifacts, vendored code, or other compile-time code from
another Matjero repository.

Each repository must be independently cloneable, buildable, testable, lintable,
Docker-buildable, CI-runnable, and deployable without requiring the source tree,
Go module, Git credentials, generated files, or build artifacts of another
Matjero repository.

### Allowed

- Versioned runtime HTTP/JSON APIs between services.
- Runtime event contracts between services.
- Deliberate, small duplication of generic technical helpers across
  repositories.

### Disallowed

- Cross-repository Go imports (`github.com/matjeroapps/<other-repo>/...`).
- A shared source repository.
- Vendoring another Matjero repository.
- Git submodules or subtrees pointing at another Matjero repository.
- `go replace` directives pointing at a sibling checkout.
- A committed `go.work` / `go.work.sum`, or any repository that requires one.
- Generated clients or OpenAPI documents downloaded from another repository
  during build.
- Direct actor access to Core-owned database tables.
- Actor-to-actor service calls for Core business workflows.

### Core becomes a runtime business capability boundary

Core stops being consumed as a shared Go library. It remains the sole owner of
commerce business rules, stores, markets, catalog, seller listings, supplier
offers, themes, inventory, availability, orders, payments orchestration,
finance, central migrations, workers, the scheduler, and authoritative
PostgreSQL access.

Core exposes these capabilities through a new independently deployable
application, `apps/core-api`, under a versioned internal namespace
`/internal/v1`.

### Generic technical code is localized, not shared

Small generic helpers (config loading, HTTP response writing, locale
negotiation, pagination parsing, OIDC verification, OpenAPI document building)
are copied into each actor repository under `internal/`, trimmed to only what
that repository actually uses. Cross-repository DRY is explicitly not a goal;
repository independence outranks avoiding small duplicated technical helpers.

### Business logic is never copied

Catalog eligibility, price-source rules, tenant isolation, market isolation,
availability, privacy rules, search, filters, pagination, store resolution,
theme business rules, inventory rules, pricing rules, listing eligibility,
supplier offer rules, market consistency, order rules, payment rules, and
finance rules remain Core-owned and are reached only through the runtime API.

### Service-to-service authentication

`/internal/v1` is authenticated with a per-caller bearer service token
(`Authorization: Bearer <token>` plus `X-Matjero-Service: seller|admin|supplier`),
compared in constant time against separately configured tokens per caller. This
is the initial mechanism and is deliberately replaceable by ZITADEL
client-credentials / OAuth2 M2M without changing application contracts.

### Forwarded actor identity

Actor APIs remain responsible for validating end-user authentication. Once
validated, an actor may forward a minimal verified actor context (subject,
business identity, roles where required) to Core over the authenticated internal
connection. Actors must strip any client-supplied internal identity headers
before setting trusted values. Service authentication does not authorise
arbitrary resource access: Core still enforces ownership and business
invariants.

### Storefront tenant security

The P4.3 host security boundary is preserved. The actor extracts the trusted
original storefront host using its own proxy policy and forwards it to Core in a
dedicated internal header, `X-Matjero-Storefront-Host`. Core resolves the host
itself. Client-supplied copies of that header are discarded by the actor, and
Core never trusts a client-selectable store UUID as tenant authority.

## Consequences

### Positive

- Every repository builds, tests, and containerises from a clean clone with no
  credentials for another repository.
- Core's database becomes a private implementation detail rather than a
  published schema.
- Business invariants are enforced in exactly one place.
- Actor release cadence is decoupled from Core's Go package surface.
- CI can mechanically prevent regression (see below).

### Negative

- Generic technical helpers are duplicated across repositories. Accepted: the
  duplication is small, mechanical, and bounded, and it buys independence.
- Every Core business capability consumed by an actor needs an HTTP endpoint and
  a local client. This is a one-time cost per capability.
- Actor tests can no longer exercise real business data; they must use local
  fake Core HTTP servers. Business correctness moves to Core's own PostgreSQL
  tests, which is where it belongs.
- Runtime latency is introduced where a Go call used to be. Accepted; P4.4
  caching is the planned mitigation and is explicitly out of scope here.

### Operational

- Core internal API is for a private service network with TLS outside trusted
  local development. It is not a public API, is not exposed through the public
  storefront domain, and has no browser CORS.
- `/internal/v1` is a stable contract. Additive compatible changes are allowed;
  breaking changes require deliberate version handling. Core commit SHAs and Go
  package versions are never part of a runtime API contract.
- Service tokens must never appear in Git, OpenAPI examples, Docker image
  layers, logs, or test snapshots.

### Guardrails

Each actor repository's CI fails when `go.mod` or any active `*.go` file
references another `github.com/matjeroapps/<other-repo>` module. Core's CI
generates its internal OpenAPI document from route source and fails on a stale
spec. These checks are permanent so future contributors cannot silently
reintroduce compile-time coupling.

## Migration sequence

1. Core: `apps/core-api`, service auth, internal contracts, internal OpenAPI,
   tests, CI, Docker. (This ADR's implementing PR.)
2. Seller: remove Core module, localize generic code, add
   `internal/coreclient`, P4.3 regression against a stub Core.
3. Admin: same process.
4. Supplier: same process.
5. Final cross-repository audit, runtime integration smoke, completion report.

Each stage ends at a manual merge gate. No actor is migrated against Core code
that exists only on an unmerged branch.
