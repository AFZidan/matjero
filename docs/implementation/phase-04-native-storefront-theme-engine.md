# Phase 4 — Native Storefront + Theme Engine

> **Architecture note (current).** Units P4.4 (Storefront Caching) and P4.5
> (Storefront Rendering) and every later unit must assume the runtime boundary
> introduced by
> [ADR-017](../plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md):
>
> ```
> storefront-api ──HTTP──► core-api ──► CatalogRepository ──► PostgreSQL
> ```
>
> not
>
> ```
> storefront-api ──Go import──► github.com/matjeroapps/core/pkg/storefront
> ```
>
> The P4.3 business implementation (eligible listings, price-source rules, tenant
> and market isolation, availability, privacy rules, search, filters, pagination)
> stays in Core and is reached through `/internal/v1/storefront/*`. Seller owns a
> local Core client and its own public DTOs. Completed P4.3 outcomes are
> unchanged; only the transport between the two services changed.
>
> **This boundary is now live and verified.** The Repository Independence
> Refactor is merged across all four repositories and its runtime smoke confirmed
> the storefront path above end to end, including tenant isolation, supplier
> price privacy, locale negotiation, and generic 503 behaviour when Core is
> unavailable. P4.4 must be built against this boundary from the start. Evidence:
> [repository-independence-report.md](repository-independence-report.md).

## 1. Objective

Deliver the first production-ready native Matjero storefront experience and a reusable, safe, versioned theme engine that supports multiple seller stores, custom store domains/subdomains, Arabic/English, RTL/LTR, SEO, caching, and future marketplace/search expansion.

## 2. Scope

### In Scope
- Trusted host → store resolution (platform subdomains + custom domains)
- Theme engine: platform-controlled, versioned, schema-driven, draft/publish configuration
- Seller theme management via Seller API + Seller Dashboard (with real ZITADEL PKCE auth)
- Public catalog read model (store-scoped, joined SQL queries)
- One complete built-in production theme (with swap-proof registry architecture)
- Public storefront pages: Home, Category/Collection, Product Listing, Product Detail, Search/Browse, Store Not Found/Inactive, 404
- Localization: Arabic/English, RTL/LTR, path-prefix locale URLs (`/en/...`, `/ar/...`)
- SEO: server-rendered metadata, canonical, hreflang, Open Graph, Twitter cards, robots directives, Product JSON-LD, sitemap, robots.txt
- Caching: Redis with revision-based versioned keys, event-free invalidation via revision bumps
- Security: tenant isolation, theme safety (no seller JS), host validation, supplier-privacy leakage prevention, price-source enforcement
- OpenAPI: all new/changed endpoints update generated specs; CI stale-spec check stays green
- Frontend tests: Vitest + Testing Library (unit/component) + Playwright E2E (multi-tenant isolation, key flows)
- Full validation suite per unit + final phase completion

### Out of Scope (Phase 5+)
Cart, Checkout, Orders, Inventory Reservation, Shipping, Payments, Refunds, Ledger, Settlements, External Integrations, Marketplace, Reputation, Intelligence, Protected Access, Third-party Theme Marketplace, Theme Developer Ecosystem, Arbitrary Seller JavaScript/Code, Kafka, Dedicated Search Engine, Owned Warehouse, Cross-border Commerce, OpenAPI Breaking-Change Detection (stays deferred).

## 3. Dependencies

- Phase 3 (Admin/Supplier/Seller Platforms) — merged to `main`
- Phase 0–2 foundations (engineering, identity/localization/markets, commerce domain) — merged to `main`
- ADR-010 (storefront tenant resolution), ADR-011 (theme security), ADR-016 (search readiness) — accepted
- Existing `store_domains` table + `StoreDomain` model + `CreateStoreDomain` repo method
- Existing `storefront-api` BFF (public, `RequireAuth: false`)
- Existing `seller-api` (authenticated, roles: seller_owner/manager/staff)
- Existing `Repository.ListSupplierCatalog` JOIN LATERAL pattern (template for public catalog queries)
- Existing `packages/config`, `packages/i18n`, `packages/httpx`, `packages/money`, `internal/testdb`
- Existing OpenAPI generation (`cmd/openapi-gen`, `internal/openapi`) + CI stale-spec check

## 4. Architecture

### 4.1 Storefront Request Flow

```
Customer
   ↓
CDN / Edge
   ↓
Next.js Storefront (web/storefront)
   ↓
storefront-api (public, tenant from validated Host)
   ↓
Store Resolver (trusted host → store → market → locale → theme)
   ↓
Commerce Core / Read Models (store-scoped)
   ↓
PostgreSQL (+ Redis cache, non-authoritative)
```

One Next.js storefront application serves many seller stores. The storefront resolves the tenant dynamically from the request host.

### 4.2 Tenant Resolution (Critical Security Boundary)

```
HTTP Host
   ↓
Store Domain Resolver
   ↓
Store
   ↓
Market
   ↓
Locale
   ↓
Theme Installation
   ↓
Public Catalog
```

- For public storefront traffic, tenant identity is derived **exclusively** from a trusted domain-to-store mapping.
- Client-supplied `store_id` / `seller_id` is **never** trusted on public routes when the request host already determines the tenant.
- The `X-Storefront-Host` header convention (already seeded in `web/storefront/src/lib/api.ts`) is the carrier for the original host through the Next.js → storefront-api hop.
- Unknown, inactive, pending, verified (not-yet-active), or disabled domain mappings fail safely to a Store Not Found page (no moderation details leaked). Only `active` domain mappings resolve publicly.

### 4.3 Subdomain Support

- Platform-generated store subdomains: `<store-code>.<platform-domain>`
- The platform domain is **configuration-driven** (`packages/config`), never hardcoded in application code.
- Resolver handles: normalized hostnames (lowercase), port stripping in development, reserved subdomains, duplicate detection, inactive stores, disabled domains.
- Comprehensive resolver tests required.

### 4.4 Custom Domain Foundation

- Custom domains map to a Store with lifecycle states: `PENDING`, `VERIFIED`, `FAILED`, `DISABLED`, `ACTIVE`.
- Verification: DNS TXT token check (seller adds domain → PENDING with token → check endpoint performs public DNS TXT lookup → VERIFIED → activation → ACTIVE). No DNS-provider integrations.
- **`VERIFIED` is NOT `ACTIVE`.** A domain that has passed ownership verification but has not yet been activated must NOT resolve publicly. Only `ACTIVE` domains (platform or custom) resolve. `PENDING`, `VERIFIED`, `FAILED`, and `DISABLED` all fail safe.
- Application-level domain mapping/resolution foundation only.

### 4.5 Theme Domain Model

Entities:
- **Theme**: platform-controlled theme definition (key, name, type FREE/PREMIUM, status)
- **Theme Version**: immutable after publication; carries the component registry version + configuration schema
- **Theme Asset**: metadata/URIs for images/fonts (CDN/object storage)
- **Theme Configuration Schema**: JSON Schema per version, validated
- **Theme Installation**: store → theme_version (one active per store, DB-enforced)
- **Theme Configuration**: draft + published, validated JSONB, revision counter
- **Theme License**: type (FREE/PREMIUM) — modeled for future compatibility; Phase 4 ships free built-in themes only

### 4.6 Theme Versioning

- Theme releases are versioned. Deployed theme code/schema does not mutate unpredictably for all stores.
- A Store installation resolves to a known Theme Version.
- Policy:
  - Installing a theme: creates a Theme Installation pointing to a Theme Version
  - Upgrading versions: seller-initiated; configuration compatibility validated against the new version's schema
  - Deprecated versions: marked deprecated; existing installations continue until upgraded
  - Configuration migration: where relevant, schema-driven migration of stored config
  - Theme versions are immutable after publication

### 4.7 No Arbitrary Seller JavaScript (Mandatory)

- Seller-controlled themes/configuration must NOT allow arbitrary executable JavaScript.
- No unrestricted `custom_js` feature.
- No seller-uploaded server-executable code.
- Themes remain: platform-controlled, versioned, schema-driven, validated, safe.
- Seller customization is data/configuration, not arbitrary code execution.

### 4.8 Theme Configuration

- Seller-customizable theme settings through a validated JSON Schema.
- Examples: logo, favicon, colors, typography, header layout, footer, navigation, announcement bar, hero, banners, homepage sections, product card style, category layout, spacing/layout options.
- Avoid creating columns for every possible theme setting — use a schema-driven JSONB model.
- Configuration must be validated, limits enforced, unsafe HTML/scripts rejected, configuration versions compatible with Theme Versions.
- No arbitrary unvalidated JSON.

### 4.9 Theme Draft / Preview / Publish

- Workflow: Published Configuration + Draft Configuration (seller edits draft without breaking live storefront).
- Seller can: edit draft, preview, publish, discard/reset.
- Preview is authorized and must not expose another seller's unpublished configuration.
- Public users only see published configuration.
- Preview mechanism: seller-api issues a short-lived signed, store-scoped preview token; storefront renders draft configuration only when a valid token for the resolved store is presented.

### 4.10 Seller Theme Management

- Extend Seller API + Seller Dashboard with theme-management workflows.
- Seller can: view available platform themes, view current theme, install/select a theme, configure allowed settings, preview changes, publish changes, inspect active version.
- No third-party theme purchasing.
- No marketplace theme submissions.

### 4.11 Built-In Themes

- Architecture must demonstrate theme-driven rendering (not hardcoded to one design).
- One complete default production theme shipped.
- Tests + registry architecture prove switching Theme definitions does not require changing commerce logic.
- Theme-specific presentation must not leak into Commerce Core.

### 4.12 Storefront Public Pages

- Home, Category/Collection, Product Listing/Catalog, Product Detail, Search/Browse, Store Not Found/Inactive, 404.
- No checkout.
- Product rendering supports: localized names/descriptions, price, availability, images/media, variants, SKU-relevant selectable options, categories, seller/store branding.
- No internal supplier information exposed to sellers/customers.

### 4.13 Supplier Privacy

- Supplier contact information is hidden from Sellers and customers by default.
- Public storefront pages must not expose: supplier contact details, supplier internal IDs where unnecessary, supplier cost, wholesale price, supplier operational metadata, fulfillment internal details.
- Public catalog responses contain only customer-facing data.
- Explicit tests for sensitive-field leakage.

### 4.14 Seller Pricing vs Supplier Cost

- Public storefront uses Seller Listing Price as the customer-facing price.
- Never expose Supplier Cost, Wholesale Price, Supplier Margin, Internal Platform Fee through storefront APIs or HTML payloads.
- Tests verify this.

### 4.15 Availability

- Public availability derives from the approved Commerce model.
- No storefront-owned stock.
- Inventory ownership = SKU + Fulfillment Location.
- Storefront displays a derived availability state.
- Phase 4 does not reserve stock.
- Never mutate inventory from storefront browse/read operations.

### 4.16 Public Catalog Read Model

- Clean storefront/public catalog query layer.
- Do not expose raw internal domain aggregates directly.
- A storefront product projection may need: store, market, listing, product, translations, category, variants, SKU availability, seller price, media.
- Build efficient read models/projections where appropriate (joined SQL queries).
- Avoid N+1 queries.
- Do not duplicate the transactional source of truth.

### 4.17 Search Architecture

- Search remains PostgreSQL-backed in Phase 4.
- No Elasticsearch, OpenSearch, Meilisearch, Typesense.
- Preserve the search abstraction established in Phase 2 (event payloads in `search.go`/`events.go`).
- Storefront APIs must not expose contracts that force PostgreSQL-specific search behavior.
- Future flow remains possible: Domain Events → Search Indexer → Dedicated Search Engine → Storefront Search.
- Search/filtering supports: keyword, category, price range, availability, attributes where supported, sort.
- Do not overbuild advanced ranking yet.

### 4.18 Localization

- Storefront fully supports Arabic, English, RTL, LTR.
- Includes: navigation, product pages, categories, homepage sections, theme configuration, metadata, empty states, errors, forms where present, responsive UI.
- No user-visible strings bypass the localization architecture.

### 4.19 Localized URL Strategy

- Path-prefix strategy: `/en/products/...`, `/ar/products/...`
- Root `/` redirects to the store's market default locale.
- Requirements: stable canonical URLs, locale awareness, localized slugs where supported, no duplicate-content chaos, correct tenant/store context.

### 4.20 SEO (Mandatory)

- Server-rendered metadata: title, description, canonical URL, Open Graph, Twitter/social metadata, hreflang, robots directives, structured metadata.
- Product pages: `Product` schema JSON-LD (only real data — no fabricated reviews/ratings/stock/pricing).
- Store/category pages: appropriate metadata.
- Sitemap, robots.txt.

### 4.21 Sitemap

- Store-scoped sitemap data.
- Include active/public: products, categories, important static pages.
- One Store cannot leak another Store's URLs.
- Handle locale variants consistently.
- Large future catalogs must move toward sitemap indexes without redesign.

### 4.22 Responsive Design

- Production-quality on mobile, tablet, desktop.
- Mobile storefront performance especially important.
- Test critical layouts at common viewport sizes.
- Avoid dashboard-style UI on customer storefronts.

### 4.23 Accessibility

- Semantic HTML, keyboard navigation, focus states, labels, image alt text, heading hierarchy, accessible navigation, reasonable contrast validation where theme configuration allows.
- Do not claim full WCAG certification unless actually tested.

### 4.24 Media

- Use existing Product/Media metadata.
- Public pages use CDN/object-storage URLs where available.
- Do not proxy large media through Go APIs unnecessarily.
- Support: responsive images where practical, image dimensions/aspect handling, lazy loading, appropriate alt text.
- Theme assets use safe controlled delivery.

### 4.25 Caching

- Deliberate storefront caching strategy.
- Cache keys always include Store/Tenant context.
- Key components: `store_id`, `market_id` where relevant, `locale`, `resource/version`, `theme version/config revision`, `query/filter identity`.
- Never create cross-store cache collisions.
- No dangerous wildcard Redis scans for invalidation.
- Prefer: versioned cache namespaces, targeted invalidation, event-driven invalidation, revision-based cache keys.
- Strategy: Redis + revision-based versioned keys. Revisions bumped transactionally on writes. Short TTL safety net. No outbox relay in Phase 4 (deferred to Phase 5+; no second event system).
- Split across the ADR-017 boundary: Core owns the authoritative per-store revision (`storefront_revisions`) and bumps it inside the transaction of the business write; Seller owns the Redis client and the cache keys and never holds revision truth. Since tenant identity on the public path is the trusted host, Seller needs no Core store identifier to key its cache safely.

### 4.26 Cache Invalidation

- Storefront must not indefinitely serve stale data after changes to: Store, Theme, Theme Configuration, Seller Listing, Product, Category, Seller Price, Availability.
- Use existing outbox/domain-event architecture where appropriate — but Phase 4 uses revision-based invalidation (no new event system).
- Exact consistency not required for every browse view, but invalidation behavior must be explicit and testable.
- Nothing is ever deleted to invalidate. A revision bump moves every later lookup into a new namespace, and the abandoned entries expire through the TTL safety net, which is what removes any need for a wildcard scan or a key registry.
- Invalidation is scoped to the stores whose public output changed. A record shared between stores (one supplier product or offer listed by several) bumps every store that lists it; a store that does not list it is untouched. Draft-only theme work never bumps, because it is invisible to customers until it is published.

### 4.27 Next.js Rendering Strategy

- Use Next.js intentionally.
- Evaluate: SSR, RSC, static generation, ISR/revalidation, dynamic rendering per page type.
- Do not mark every storefront page fully dynamic without reason.
- Do not statically generate all possible stores/products if that will fail at scale.
- Choose rendering strategies that allow: SEO, good TTFB, many tenants, cacheability, rapid product updates, future scale.
- Decision: RSC/SSR per page type with explicit `revalidate` windows backed by the revision-keyed Redis data layer. No full-static generation of all tenants.

### 4.28 Tenant Isolation (Critical)

- Cross-store data leakage is a critical security defect.
- Test explicitly that Store A cannot receive Store B's products, categories, theme, configuration, sitemap, metadata.
- Tenant context flows through all storefront queries.

### 4.29 Store Status

- Inactive/suspended/unpublished stores must not behave as normal public stores.
- Public behavior for statuses: `active`, `inactive`, `suspended`, `draft`.
- Do not leak moderation details publicly.

### 4.30 Theme Isolation

- A seller must not be able to: modify another seller's Theme Installation, preview another seller's draft, publish another seller's configuration, attach another Store's configuration, access unauthorized theme assets/settings.
- Authorization tests required.

### 4.31 Security

- Focused storefront security review.
- Test for: cross-store IDOR, hostname spoofing, unsafe forwarded-host handling, tenant confusion, XSS through theme settings, XSS through Product content, unsafe HTML, malicious URLs, open redirects, sensitive supplier data leakage, price/cost leakage, cache poisoning, unauthorized theme preview, path/slug manipulation.
- Do not blindly trust `Host` / `X-Forwarded-Host` unless requests come through explicitly trusted proxy configuration.
- Document trusted proxy behavior.

### 4.32 API Boundaries

- `storefront-api` remains the customer/public actor-facing API.
- Do not put storefront-specific presentation logic into `seller-api`, `supplier-api`, `admin-api`.
- Seller theme management belongs in `seller-api`.
- Public storefront reads belong in `storefront-api`.
- Both use shared domain/application capabilities rather than calling each other over HTTP for core business logic.

### 4.33 OpenAPI / Swagger (Mandatory)

- Every API endpoint added or changed in Phase 4 must update its generated OpenAPI contract.
- Relevant specs: `docs/api/storefront/openapi.json`, `docs/api/seller/openapi.json`.
- Ensure: correct domain tags, request schemas, response schemas, errors, security, pagination/filtering, public vs authenticated operations.
- Suggested Phase 4 tags: Storefront, Store Resolution, Catalog, Products, Categories, Search, Themes, Theme Configuration.
- Do NOT hand-edit generated specs as the primary source.
- Run `go run ./cmd/openapi-gen` and existing OpenAPI validation/stale-spec checks.
- CI must fail if generated specs are stale.

### 4.34 Public vs Authenticated OpenAPI

- Public product browsing → anonymous allowed.
- Seller theme configuration → authenticated + resource authorization.
- Do not mark seller management endpoints as public merely because Swagger exists.

### 4.35 Database Changes

- Migrations only where required for the Theme/Domain/Storefront foundation.
- Migration filenames describe their responsibility (no phase identifiers).
- Preserve migration ordering and rollback safety.
- New migrations: `000005_store_domain_lifecycle.{up,down}.sql`, `000006_theme_engine_schema.{up,down}.sql`.

### 4.36 Database Integrity

- Use PostgreSQL constraints for enforceable invariants:
  - Unique normalized domain mappings
  - Valid installation relationships
  - One active/published theme installation per store where required
  - Store ownership
  - Theme Version ownership
  - Configuration/version compatibility where feasible
- Do not rely exclusively on frontend validation.

### 4.37 Performance

- Review critical storefront paths for: N+1 queries, excessive database round trips, oversized payloads, unnecessary hydration, unnecessary client JavaScript, poor image behavior, cache misses, excessive theme configuration parsing.
- Storefront performance is a product requirement.
- Do not prematurely introduce large infrastructure.
- Measure representative pages before adding complexity.
- Document performance observations and bottlenecks.

### 4.38 Testing

#### Domain Tests
- Theme creation/version rules
- Theme Installation
- Configuration validation
- Draft/publish behavior
- Version compatibility

#### Store Resolver Tests
- Valid subdomain
- Valid custom domain
- Inactive domain
- Duplicate domain
- Unknown domain
- Normalized host
- Development host with port
- Spoofed/untrusted forwarded host

#### Repository/Integration Tests
- Theme persistence
- Domain mappings
- Public catalog projections
- Tenant-scoped queries
- Cache invalidation behavior where applicable

#### API Tests
- Public storefront routes
- Seller Theme management
- Authentication boundaries
- Response privacy

#### Frontend Tests
- Theme rendering
- Arabic/English
- RTL/LTR
- Responsive navigation
- Product rendering
- Category rendering
- 404/inactive store
- Seller theme editing/preview/publish

#### Security Tests
- Cross-store leakage
- Unauthorized Theme access

#### SEO Tests
- Canonical
- Hreflang
- Metadata
- Sitemap
- Robots
- Structured data

### 4.39 Multi-Tenant Regression Test

- Explicit test scenario with Store A and Store B, each having: different domain, different products, different prices, different theme, different locale/config.
- Verify requests for Store A never contain Store B information and vice versa.
- Mandatory Phase 4 acceptance test.

### 4.40 Out of Scope

Do NOT implement: Cart, Checkout, Inventory reservation during checkout, Orders, Shipping, Payments, Refunds, Ledger, Settlements, External supplier integrations, External seller integrations, Unified marketplace, Reputation scoring, Product Intelligence, Protected Product Access, Third-party Theme Marketplace, Theme developer ecosystem, Arbitrary custom theme code, Kafka deployment, Dedicated search engine, Owned warehouse, Cross-border commerce.

### 4.41 Full Validation

Before considering any feature complete, run:
```
go run ./cmd/openapi-gen
go test ./...
go vet ./...
go build ./...
npm run lint
npm run typecheck
npm run test
npm run build --workspaces --if-present
npm audit --audit-level=high
docker compose config --quiet
make migrate-check
```
Also run: PostgreSQL integration tests, API integration tests, tenant isolation tests, theme authorization tests, security tests, SEO tests, OpenAPI generation/validation/stale-spec checks, CI-equivalent validation.

### 4.42 OpenAPI Breaking-Change Detection

The previously deferred lightweight OpenAPI historical breaking-change checker is NOT a blocker for Phase 4. Do not expand Phase 4 unnecessarily just to implement it.

### 4.43 Final Phase 4 Implementation Report

Create `docs/implementation/phase-04-implementation-report.md` with all required sections.

### 4.44 What Is Next

The implementation report and final AI response must include a detailed "What Is Next" section based on the actual completed repository state.

### 4.45 Final Readiness Decision

End the implementation report with exactly one: `READY FOR PHASE 5` or `NOT READY FOR PHASE 5`.

### 4.46 Final Pull Request

After all Phase 4 implementation units completed and dependencies merged, perform final Phase 4 completion work on `feature/p4-phase-completion`. Commit, push, create the final PR targeting `main`. Do not auto-merge.

### 4.47 ALL GitHub CI Checks Must Be Green

After every PR, monitor ALL GitHub CI checks. Fix until ALL GREEN.

### 4.48 Phase 4 Completion Standard

Phase 4 is complete only when all completion criteria are met.

## 5. Implementation Units

### P4.0 — Phase Specification
- **Goal**: Document the complete Phase 4 plan.
- **Dependencies**: None.
- **Branch**: `feature/p4-phase-spec`
- **Backend Work**: Create `docs/implementation/phase-04-native-storefront-theme-engine.md`.
- **Frontend Work**: None.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**: None.
- **Acceptance Criteria**: Spec document created and reviewed.

### P4.1 — Store Resolution
- **Goal**: Trusted host → store resolution with lifecycle support.
- **Dependencies**: P4.0.
- **Branch**: `feature/p4-store-resolution`
- **Backend Work**:
  - Migration `000005_store_domain_lifecycle.{up,down}.sql`: evolve `store_domains` with lifecycle CHECK (PENDING/VERIFIED/ACTIVE/FAILED/DISABLED), `verification_token`, `domain_type` (platform/custom), `last_checked_at`; reserved-subdomain support.
  - Migration `000006_store_domain_integrity.{up,down}.sql`: enforce canonical lowercase domain uniqueness via a case-insensitive unique index on `lower(domain)` (replacing the original case-sensitive `UNIQUE(domain)` constraint and the redundant `store_domains_domain_idx`); defensive normalization of existing rows.
  - New `internal/storefront` package: `domain_resolver.go`, `store_resolver.go` — trusted host extraction (lowercase, port strip, forwarded-host only behind trusted-proxy config), PG lookup with fail-safe handling.
  - `packages/config`: platform domain + trusted proxy settings.
  - **Atomic store + platform subdomain creation**: `CreateStoreWithDomain` creates the store, its settings, and its primary platform domain in a single PostgreSQL transaction (rollback on any failure). Reserved-subdomain validation runs before any database write.
  - Canonicalization: `commerce.NormalizeDomain` is the single shared normalization routine used at both the write boundary (repository) and the read boundary (storefront resolver).
  - Repository read methods in `internal/commerce/platform_repository.go`.
- **Frontend Work**: None.
- **Database Changes**: `000005_store_domain_lifecycle.{up,down}.sql`.
- **API/OpenAPI Changes**: None (internal package).pending/verified/disabled domains fail safe; VERIFIED != ACTIVE enforced; canonical lowercase persistence; case-insensitive duplicate rejection; atomic store+domain creation rolls back on conflict; one-primary-domain-per-store invariant
- **Tests**: `resolver_test.go`, `tenant_isolation_test.go` (valid subdomain, custom domain, inactive, duplicate, unknown, normalized host, dev host with port, spoofed forwarded host).
- **Acceptance Criteria**: All resolver tests pass; unknown/inactive/unverified domains fail safe.

### P4.2 — Theme Engine Backend
- **Goal**: Platform-controlled, versioned theme engine with draft/publish configuration.
- **Dependencies**: P4.1.
- **Branch**: `feature/p4-theme-engine`
- **Backend Work**:
  - Migration `000007_theme_engine_schema.{up,down}.sql` (P4.1 consumed `000005`/`000006`): tables `themes`, `theme_versions`, `theme_installations`, `theme_configurations`, `theme_assets`.
    - `themes`: `key` UNIQUE, `type` (`free`/`premium`), `status` (`draft`/`active`/`deprecated`/`disabled`).
    - `theme_versions`: immutable after publication; `UNIQUE(theme_id, version)`; `configuration_schema` + `default_configuration` stored as JSONB on the version (no separate schema table); `status` (`draft`/`published`/`deprecated`); `published_at`/`deprecated_at`.
    - `theme_installations`: binds `store_id` → `theme_version_id`; partial unique index `theme_installations_one_active_per_store` enforces **one active installation per store** at the DB level; `status` (`active`/`inactive`).
    - `theme_configurations`: `draft_config` + `published_config` JSONB, `draft_revision`/`published_revision` (both `>= 0` check), `UNIQUE(installation_id)`.
    - `theme_assets`: metadata only (uri/integrity/metadata); binaries served via CDN/object storage later.
  - New `internal/themes` package (responsibility-based files, no phase prefixes): `theme.go`, `version.go`, `installation.go`, `configuration.go`, `asset.go`, `errors.go`, `repository.go`, `service.go`, `validator.go`, `preview.go`, `seed.go`.
    - `validator.go`: JSON-Schema (draft 2020-12) validation via `santhosh-tekuri/jsonschema/v6` + recursive unsafe-content rejection (`<script>`, `javascript:`, `on*=` handlers, `<iframe>`/`<object>`/`<embed>`/`<style>`, `expression(`). Seller customization is structured data only — no raw HTML/CSS/JS editor.
    - `service.go`: install (default config init, switching deactivates prior active install), draft update (schema + safety validated), atomic publish (validate → copy draft→published → bump `published_revision` in one tx), discard/reset, version upgrade (validates current draft against target schema, rejects incompatible), resource-level authorization via `StoreLookup` (seller may only manage own stores).
    - `preview.go`: HMAC-SHA256 signed, store-scoped, short-lived preview token (`store_id`, `installation_id`, `draft_revision`, `exp`); secret from `config.ThemePreviewSecret` (never hardcoded); tamper/forge/expiry resistant; caller compares `store_id`/`installation_id` to prevent cross-store use.
    - `seed.go`: idempotent `SeedBuiltInThemes` creates the built-in `matjero-default` FREE theme + published `1.0.0` version (default schema + default config guaranteed compatible). Safe to run on every startup.
  - Seller-api endpoints (all ownership-checked, tags `Themes` / `Theme Configuration`): `GET /v1/seller/themes`, `GET /v1/seller/themes/{key}/versions`, `GET /v1/seller/stores/{store_id}/theme`, `POST /v1/seller/stores/{store_id}/theme/install`, `GET|PUT /v1/seller/stores/{store_id}/theme/draft`, `POST /v1/seller/stores/{store_id}/theme/publish`, `POST /v1/seller/stores/{store_id}/theme/discard`, `POST /v1/seller/stores/{store_id}/theme/upgrade`, `POST /v1/seller/stores/{store_id}/theme/preview`.
- **Frontend Work**: None.
- **Database Changes**: `000007_theme_engine_schema.{up,down}.sql`.
- **API/OpenAPI Changes**: Seller API endpoints (Themes, Theme Configuration tags); regenerate `docs/api/seller/openapi.json` via `go run ./cmd/openapi-gen`.
- **Tests**: `theme_test.go` (type/status rules, schema validation, unsafe-content rejection, default-config compatibility); `repository_integration_test.go` (persistence, version uniqueness, one active installation per store, config persistence, publish copies draft + bumps revision, failed publish leaves published untouched, FK constraints, install/switch/upgrade, cross-store authorization, preview-token lifecycle, migration up/down/reapply).
- **Acceptance Criteria**: All theme domain + integration tests pass; seller authorization enforced; OpenAPI spec regenerated and in sync; `go vet/build/test`, `npm` lint/typecheck/test/build, `docker compose config`, `make migrate-check` green.

### P4.3 — Public Catalog
- **Goal**: Store-scoped public catalog read model.
- **Dependencies**: P4.2.
- **Branch**: `feature/p4-public-catalog` (in both `matjeroapps/core` and `matjeroapps/seller`).
- **Ownership after the multi-repository split**: Core owns the read model; Seller owns HTTP routing and the Storefront OpenAPI document.
- **Backend Work (Core — `github.com/matjeroapps/core`)**:
  - `pkg/storefront/catalog.go`: public DTOs, `CatalogScope` (constructible only from a host-resolved `ResolvedStore`), validated locale, bounded `Page`, domain-neutral `ProductQuery` (keyword/category/price/availability/sort), sentinel errors `ErrCatalogNotFound` / `ErrInvalidQuery`.
  - `pkg/storefront/catalog_repository.go`: `CatalogRepository` store-scoped joined queries (bootstrap, category tree, category by slug, product listing with filters, product detail, search) — modeled on the `ListSupplierCatalog` JOIN LATERAL pattern; one shared `eligibleListings` CTE defines public eligibility for every read.
  - Public DTOs are purpose-built customer-facing projections (no supplier IDs/contact/cost, no wholesale price, no platform fees, no internal fulfillment metadata, no inventory quantities). Public price is always the current Seller Listing price. Availability is derived read-only from SKU + active market fulfillment locations + inventory snapshots, with no reservation or inventory writes.
  - Bootstrap exposes published theme configuration only; draft configuration stays behind the P4.2 signed preview token.
- **Backend Work (Seller — `github.com/matjeroapps/seller`)**:
  - `apps/storefront-api` public routes (anonymous, tenant from validated host via the Core store resolver): bootstrap/store, categories, category by slug, products, product by slug, search.
  - Request validation and mapping from Core read DTOs to stable public API response contracts.
- **Frontend Work**: None.
- **Database Changes**: None.
- **API/OpenAPI Changes**: Storefront API public endpoints (Storefront, Catalog, Products, Categories, Search tags); regenerate `docs/api/storefront/openapi.json` in the Seller repository.
- **Tests**: Core repository integration (testdb): two-store isolation both directions, cross-store category/product slugs, market isolation, ar/en projections, non-public record exclusion, price source, filters/sort/pagination, search scoping, sensitive-field serialization leakage, read-only state assertions. Seller: API tests, host-resolution tests, contract leakage tests.
- **Acceptance Criteria**: All catalog tests pass; no supplier-privacy leakage; seller listing price only.

### P4.4 — Storefront Caching
- **Goal**: Redis-backed public storefront response cache, invalidated by authoritative per-store revisions.
- **Dependencies**: P4.3.
- **Branch**: `feature/p4-storefront-caching` (in both `matjeroapps/core` and `matjeroapps/seller`).
- **Ownership after the multi-repository split**: Core owns the authoritative revision state and bumps it transactionally; Seller owns the Redis client, the cache keys, and the cache behaviour. Core holds no Redis dependency and Seller holds no revision truth.
- **Backend Work (Core — `github.com/matjeroapps/core`)**:
  - Migration `000008_storefront_revisions.{up,down}.sql`: one `storefront_revisions` row per store (`store_id` PK/FK `ON DELETE CASCADE`, `revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1)`), backfilled for every existing store. A store with no row reads as revision 1, so a store created by an older path is never left without a generation.
  - `pkg/commerce/storefront_revision.go`: the store selectors for every storefront-affecting write and one atomic upsert bump (`INSERT ... ON CONFLICT DO UPDATE SET revision = revision + 1`). Bumps run on the transaction of the business write, so a rolled-back mutation never advances the revision and concurrent writes cannot lose a bump. New stores are initialized inside `createStoreInTx`.
  - Bumped writes: store status/profile, seller listing create/price/status, product translation/status/slug/categories, variant and SKU creation, category translation/status/slug/reparent (the whole subtree, so a rename or moderation of an ancestor invalidates every store publishing a descendant), supplier offer status and availability, inventory snapshot creation, inventory adjustment, inventory reservation, fulfillment location status. Records shared between stores (one product or offer listed by several stores) bump every affected store.
  - Deliberately not bumped: supplier wholesale price (never public), theme draft edit/discard/preview, seller/supplier/supplier-market status, and creation of products or offers no store lists yet.
  - `pkg/themes/storefront_revision.go`: the Theme Engine's own bump, used by install/switch, deactivate, configuration create, version upgrade, and publish. Publishing is the only configuration write that advances the revision, because a draft is invisible to customers.
  - `pkg/storefront/revision.go`: `RevisionReader` resolves a trusted host through the existing `StoreResolver` to a revision. An unknown host, an inactive domain and an inactive store are indistinguishable and yield no revision, which is what stops a downstream cache from serving a store that stopped resolving publicly.
  - `internal/coreapi`: `GET /internal/v1/storefront/revision` (seller service caller only, tenant from `X-Matjero-Storefront-Host`, `{"revision": <n>}`), and every successful public storefront read is labelled `X-Matjero-Storefront-Revision`. The revision is read before the catalog query, so the label is a lower bound on the payload's freshness and the read/write race cannot cache older data under a newer generation.
- **Backend Work (Seller — `github.com/matjeroapps/seller`)**:
  - Seller-owned Redis client and configuration; cache disabled by default and never required to start `storefront-api`.
  - Revisioned, tenant-safe cache keys over the six public GET routes, built from the trusted normalized host, locale, resource identity, canonicalized query, and the revision returned by Core. No wildcard invalidation: a revision change moves lookups into a new namespace and abandoned entries expire through a short TTL safety net.
  - Redis failure degrades to a normal Core call; Core unavailability does not serve unvalidated cached content.
- **Frontend Work**: None.
- **Database Changes**: `000008_storefront_revisions` (Core).
- **API/OpenAPI Changes**: Core internal API gains the revision route and the revision response header; regenerate `docs/api/internal/openapi.json`. Seller's public storefront contract is unchanged.
- **Tests**: Core (real PostgreSQL): initial revision, missing-row default, per-write bump scope in both directions, shared-record multi-store invalidation, non-public writes not bumping, rollback leaving the revision unchanged, concurrent bumps not lost, cascade on store deletion, theme publish vs draft, and through `core-api`: host-scoped revisions, service-auth and caller scoping, host spoofing, unknown/inactive storefronts, revision header on every public read and absent on failures. Seller: cache hit/miss, revision invalidation, cross-store and locale isolation, query canonicalization, cache-poisoning attempts, Redis-unavailable and Core-unavailable degradation, oversized payloads, and unchanged public OpenAPI.
- **Acceptance Criteria**: Revisions are authoritative and transactional; no cross-store collisions; no wildcard invalidation; graceful Redis degradation; no Core Go dependency in Seller; no RabbitMQ used for cache invalidation.

### P4.5 — Storefront Rendering
- **Goal**: Multi-tenant Next.js storefront with theme registry.
- **Dependencies**: P4.3 (parallel with P4.4).
- **Branch**: `feature/p4-storefront-rendering`
- **Backend Work**: None.
- **Frontend Work**:
  - `web/storefront`: middleware for host→tenant context + locale routing (`/en`, `/ar`, root redirect); real i18n dictionaries; `dir`/`lang` from locale; theme registry architecture (`theme.go`-style registry mapping theme key+version → component set); default theme component set; pages: Home, Category/Collection, Product Listing, Product Detail, Search/Browse, Store Not Found/Inactive, 404; responsive mobile-first; a11y baseline.
  - API client extended from `src/lib/api.ts`.
  - Vitest + Testing Library workspace config.
  - Next.js `output: "standalone"` + runtime stage in `docker/web-app.Dockerfile`.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**: Vitest (theme rendering, ar/en + RTL/LTR, product/category rendering, 404/inactive store, localization); swap-proof test (registry resolves a stub second theme without touching commerce/data code).
- **Acceptance Criteria**: All frontend tests pass; storefront renders correctly for both locales; theme registry proves swapability.

### P4.6 — Storefront SEO
- **Goal**: Complete SEO foundation.
- **Dependencies**: P4.5.
- **Branch**: `feature/p4-storefront-seo`
- **Backend Work**: None.
- **Frontend Work**:
  - Per-page server metadata: title, description, canonical, Open Graph, Twitter cards; hreflang (ar/en + x-default); robots directives; `Product` JSON-LD on product pages (real data only); store/category metadata.
  - `sitemap` (store-scoped, locale variants, index-ready) + `robots` route handlers.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**: Canonical, hreflang, metadata, sitemap content, robots, structured data — semantic assertions.
- **Acceptance Criteria**: All SEO tests pass; no store B URLs in store A sitemap.

### P4.7 — Seller Theme Management UI
- **Goal**: Seller dashboard theme management with real auth.
- **Dependencies**: P4.2 (parallel with P4.5/P4.6).
- **Branch**: `feature/p4-theme-management`
- **Backend Work**: None (endpoints from P4.2).
- **Frontend Work**:
  - `web/seller`: ZITADEL PKCE login (token acquisition/refresh wired into API client's `getAccessToken`); lightweight routing; theme screens: browse platform themes, current theme/version, install/select, edit draft configuration (schema-driven form), preview (opens storefront preview URL with issued token), publish, discard, version/upgrade inspection; ar/en + RTL/LTR.
  - Vitest coverage for theme editor flows.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**: Vitest (theme editor flows, auth wiring).
- **Acceptance Criteria**: Seller can log in, manage theme configuration, preview, publish; seller B cannot see seller A's themes.

### P4.8 — Custom Domain Lifecycle
- **Goal**: Custom domain verification and lifecycle.
- **Dependencies**: P4.1 (parallel with Phase B/C units).
- **Branch**: `feature/p4-theme-domain`
- **Backend Work**:
  - Seller-api: request custom domain (PENDING + verification token), check verification (public DNS TXT lookup → VERIFIED/FAILED), list own domains; activation rules.
  - Admin-api: list domains across stores, disable/re-enable, moderation view.
  - Resolver integration: only ACTIVE domains resolve publicly (VERIFIED is not routable).
- **Frontend Work**: Seller domain management UI (part of P4.7 or separate).
- **Database Changes**: `verification_token`, `domain_type`, `last_checked_at` columns (part of 000005).
- **API/OpenAPI Changes**: Seller + Admin domain endpoints; regenerate specs.
- **Tests**: Full lifecycle, duplicate rejection, unverified never resolves, disabled fails safe, resolver integration.
- **Acceptance Criteria**: All domain lifecycle tests pass; unverified/disabled domains fail safe.

### P4.9 — Performance, Security & Multi-Tenant Regression
- **Goal**: Hardening and full validation.
- **Dependencies**: P4.5, P4.6, P4.7.
- **Branch**: `feature/p4-storefront-performance`
- **Backend Work**: None.
- **Frontend Work**: None.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**:
  - Playwright E2E suite + new CI job (services: Postgres + Redis; run migrations, storefront-api, seller-api, seeded data, built storefront).
  - Mandatory Store A / Store B regression (different domains, products, prices, themes, locales — zero cross-store leakage both directions).
  - Sitemap isolation, theme preview authorization, host spoofing/forwarded-host attacks, path/slug manipulation, open-redirect checks.
  - Security test pass: cross-store IDOR, tenant confusion, XSS via theme config and product content, cache poisoning, price/cost leakage, unauthorized preview.
  - Performance review of critical paths; document findings + mitigations.
  - Full local validation suite (task §46) green.
- **Acceptance Criteria**: All E2E tests pass; all CI jobs green; performance findings documented.

### P4.10 — Phase Completion
- **Goal**: Finalize Phase 4.
- **Dependencies**: All units merged.
- **Branch**: `feature/p4-phase-completion`
- **Backend Work**: None.
- **Frontend Work**: None.
- **Database Changes**: None.
- **API/OpenAPI Changes**: None.
- **Tests**: None.
- **Acceptance Criteria**: Implementation report created; naming audit clean; final PR created; all CI green.

## 6. Branch Names

```
feature/p4-phase-spec
feature/p4-store-resolution
feature/p4-theme-engine
feature/p4-public-catalog
feature/p4-storefront-caching
feature/p4-storefront-rendering
feature/p4-storefront-seo
feature/p4-theme-management
feature/p4-theme-domain
feature/p4-storefront-performance
feature/p4-phase-completion
```

## 7. Definition of Done

- Multi-tenant storefront works
- Tenant isolation proven
- Public catalog works
- Theme engine is genuinely reusable
- Seller can manage/publish theme configuration
- Arabic/English + RTL/LTR work
- SEO foundation works
- Caching is tenant-safe
- Security tests pass
- OpenAPI is current
- All local validations pass
- PR created by the AI agent
- ALL GitHub CI checks GREEN

## 8. Notes

- The `grill-me` skill is not installed in this environment; the built-in clarifying-questions mechanism was used to grill the eight material Phase 4 architecture decisions. Decisions are documented in §4.
- Premium themes: schema models type (FREE/PREMIUM) + license entities for future compatibility; Phase 4 ships free built-in themes only.
- OpenAPI breaking-change checker stays deferred (per task §47).
- Outbox relay/search indexer deferred to Phase 5+ (revision-based invalidation instead; no second event system).
