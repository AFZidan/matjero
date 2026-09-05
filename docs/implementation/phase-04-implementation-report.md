# Phase 4 — Final Implementation Report & Readiness Gate

> **Phase Status**: Phase 4 Native Storefront & Theme Engine is fully implemented, audited, and verified across all Matjero micro-service repositories. All required Phase 4 units (P4.0 through P4.10), domain lifecycle, theme engine, public catalog, storefront caching, SEO, multi-tenant security regression, and the prerequisite Supplier Retail Capability are merged to `main` (P4.10 merged in Core PR #22 commit `9dffa2d700a1d0130efdad45beaf7e218bfa1f09`; Admin post-P4.10 CI correction merged in PR #7 commit `50b54117b201d10203d5b8b2bb943b3a6fb66d3c`) and verified with clean green CI pipelines.

---

## 1. Executive Summary

Phase 4 delivers the native Matjero storefront experience and a reusable, safe, versioned theme engine supporting multi-tenant seller stores, custom store domains/subdomains, Arabic and English localization with dynamic LTR/RTL layout switching, server-side SEO, revision-backed Redis response caching, strict repository independence (ADR-017), and full multi-tenant isolation.

All Phase 4 implementation units—from initial specification (P4.0) through domain lifecycle (P4.1, P4.8A-C), theme engine (P4.2, P4.7), public catalog (P4.3), storefront caching (P4.4), rendering (P4.5), SEO (P4.6), supplier retail capabilities (prerequisite), performance/security regression testing (P4.9), and final phase documentation (P4.10)—are merged and complete.

The codebase adheres strictly to all quality, security, and repository independence boundaries. Zero compile-time cross-repo dependencies remain. Generated OpenAPI specifications are 100% synchronized across all Go repositories (`core`, `seller`, `admin`, `supplier`).

---

## 2. Final Repository References

The canonical git commit SHAs recorded at Phase 4 completion baseline are:

- **Core (`matjeroapps/core`)**: `9dffa2d700a1d0130efdad45beaf7e218bfa1f09`
- **Seller (`matjeroapps/seller`)**: `d75f84c66cfca9ab4c79c8a3495c5080be28a7af`
- **Admin (`matjeroapps/admin`)**: `50b54117b201d10203d5b8b2bb943b3a6fb66d3c`
- **Supplier (`matjeroapps/supplier`)**: `0d601c81b9ecd5b486f45a690f4e9a716efd12d5`
- **Seller Hub (`matjeroapps/seller-hub`)**: `e282c9e305f0cec49450d13667361c2c87a4c701` *(informational)*
- **Supplier Hub (`matjeroapps/supplier-hub`)**: `bedf4517a153aae06cf17ed109c5923d9d4845be` *(informational)*

---

## 3. Phase 4 Unit / PR Evidence Matrix

| Unit | Repository | PR # | Feature Head SHA | Merge Commit SHA | Status | Evidence / Report Path | CI Status |
| :--- | :--- | :---: | :---: | :---: | :---: | :--- | :---: |
| **P4.0** | `core` | #6 | `cabad67a6cc42147a1b2a36769bf2fba29c35e11` | `0955f7ff930523f51b851338b3886a2d684c985a` | MERGED | `docs/implementation/phase-04-native-storefront-theme-engine.md` | PASS |
| **P4.1** | `core` | #7 | `12a7b427ef62b19658bd71d19bf0549760ca6276` | `016f8f45d6bd2c76621f61fa78240ee51701f804` | MERGED | `internal/storefront/store_resolver.go` | PASS |
| **P4.2** | `core` | #8 | `54ab9c68b4def05bf0aeb3e2cc9ddba87a7801ee` | `8378f32590dba083050ac2b619ba5cbf511a38dd` | MERGED | `modules/themes/service.go` | PASS |
| **P4.3** | `core` | #12 | `c6167b96201af306db7074a9c045c9f025b7d268` | `c63d084863d517599853b48bb3c262685a7b6ec9` | MERGED | `modules/storefront/catalog_repository.go` | PASS |
| **P4.3** | `seller` | #1 | `cc2149fc3a1dc64fb4a9720635ee4950217da137` | `b2441ee169f4ac614d3924c65ac74d314843d072` | MERGED | `internal/storefrontapi/router.go` | PASS |
| **Repo Indep.** | `core` | #13 | `cfba60226045c051174dc91094aa3a82700b382a` | `bfc9721cf628bed1985c917a66c053970e9fce1e` | MERGED | `docs/implementation/repository-independence-report.md` | PASS |
| **Repo Indep.** | `core` | #14 | `01432bf0872eb79bc184fb057df93951a1583d23` | `1336f05751c2c47001d833ca201b748bfa37f793` | MERGED | N/A | PASS |
| **Repo Indep.** | `core` | #15 | `9f76cade5e13b3ed56587da12b537762c45dde2c` | `c873f9e5605523d33f98ee300f7f8b8b7568a647` | MERGED | N/A | PASS |
| **Repo Indep.** | `seller` | #2 | `a93e4dad2397c5614747a59d47603f48d9865dd9` | `ad582046b3c2a93c8231cd2151c68c8c2b16d418` | MERGED | N/A | PASS |
| **Repo Indep.** | `seller` | #3 | `6c36eb203457cbe4f86489c41c555adee1fc1123` | `6600052f8bcef034cdd39383a5d5b6e02f9d0cb3` | MERGED | N/A | PASS |
| **Repo Indep.** | `admin` | #1 | `98cabc3edb31bef56097d08839d05ff1a77255fb` | `38c678171b77bdd65bf856e2de55229c945c5956` | MERGED | N/A | PASS |
| **Repo Indep.** | `admin` | #2 | `df7fba1879fd683d6465ed67aac9decd3f99bec9` | `c42c79bcf5698235784070ceb38e078caf38c982` | MERGED | N/A | PASS |
| **Repo Indep.** | `supplier` | #1 | `65b1fc4c22de9d1413d4074a1729016821f797f9` | `ba89cd608fbd91155de976aa108f040d6a8af38c` | MERGED | N/A | PASS |
| **Repo Indep.** | `supplier` | #2 | `e87d6d4586ed03f1a008fe6e39ab9bc3ba6c4b95` | `0d601c81b9ecd5b486f45a690f4e9a716efd12d5` | MERGED | N/A | PASS |
| **P4.4** | `core` | #16 | `6834206be529621c2e6d615d4f6506342bdad620` | `5f1e0b75de1b645a9b3b33e7c394a453a1570323` | MERGED | `modules/commerce/storefront_revision.go` | PASS |
| **P4.4** | `seller` | #4 | `638ec8d2e9f20e3dbb7227746ee03f3a6fa42eb6` | `a9d28d1f68252a0feba42205a73520897fb569b8` | MERGED | `internal/storefrontcache/cache.go` | PASS |
| **P4.5** | `seller` | #5 | `5b0cc0c47d821cf6dbd2a8b3b6c469d37c784b23` | `c4675d78420a7399aabc4a2dd4b17602d78adc72` | MERGED | `web/storefront/src/app/[locale]/(store)/` | PASS |
| **P4.6** | `seller` | #6 | `637dd1d386e08507c7093b1092045f748eff436e` | `c7798b6ca0782a7d5eac367037a695bf8569483c` | MERGED | `web/storefront/src/server/seo.ts` | PASS |
| **P4.7** | `core` | #18 | `0380854119f194c52f504768c609edf6961ee39c` | `e3e7feb2d04e45e964c1e5cbc82aabf4b12754db` | MERGED | `docs/implementation/storefront-theme-preview-runtime-report.md` | PASS |
| **P4.7** | `core` | #19 | `f0638c28255f827213d25f7273ceb1b44a18bbac` | `a94d73d8bb2387e695a3bf85b950df77dc416d54` | MERGED | N/A | PASS |
| **P4.7** | `seller` | #7 | `1490e4591d2e39f8142976be4b991d2675d7d4ed` | `664e7c124c844c0a50bb5ab6578dd9612558a1d6` | MERGED | N/A | PASS |
| **P4.7** | `seller` | #8 | `2cff72ef337a3f83c74b7cc3148424cca209d676` | `bf70c50af50ca8ed1f9a09ab4f5eb7bf2bee7892` | MERGED | N/A | PASS |
| **P4.7** | `seller` | #9 | `4e2a26f22a128218358cac574b5e955abeb1fefc` | `acc613d1c3fde592d2c4915de5127861cfca5456` | MERGED | `web/seller/src/components/ThemeEditorPanel.tsx` | PASS |
| **P4.7** | `seller` | #10 | `8ba140e1d48b12ad4e8a1a246142d8d56a2e3940` | `2c1a09b61d7ac809c229c81c861392f7d6d574ac` | MERGED | N/A | PASS |
| **P4.8A**| `core` | #20 | `781e8e928035708cbbcd73372b4f18e1751c7ed4` | `96cf98e5a1f1de3f388a86e316b5b59414d49d11` | MERGED | `modules/commerce/domain_lifecycle.go` | PASS |
| **P4.8B**| `seller` | #11 | `29b96a4716a30e9fe6253bf7d9c8fcaec45de531` | `1354a9ab2d4e001b5ac5ac13aaac958aa69505c7` | MERGED | `internal/sellerapi/domains.go` | PASS |
| **P4.8C**| `admin` | #3 | `e1b5c65b828a78cf910b04481ad509493d0161c0` | `5b9d16c26a52c4faa0cd7193d975841cb1ecde7f` | MERGED | N/A | PASS |
| **P4.8C**| `admin` | #4 | `6d22220c5a75610051855832c6b1f2308d334397` | `c611db032a35096663e0013f5211b943d582428a` | MERGED | `docs/implementation/admin-domain-moderation-report.md` | PASS |
| **P4.8C**| `admin` | #5 | `8f2dca2e102fb568b1276ff7767569fab11aa0cc` | `5c6d0b53f4118774fa96ce01a1a45605dbdfb390` | MERGED | N/A | PASS |
| **P4.8C**| `admin` | #6 | `26ea097c3bd87ad27bec5b76e8788a12a5b28bf6` | `5c3a3bacaa2131670294af1f254d417ed09310ae` | MERGED | N/A | PASS |
| **Post-P4.10 Fix**| `admin` | #7 | `c2a3b034564a3eae45fb508080134ad6b78b21af` | `50b54117b201d10203d5b8b2bb943b3a6fb66d3c` | MERGED | `.github/workflows/ci.yml` | PASS |
| **Prereq**| `core` | #21 | `d7e36be5cc1e64e50a913bef6521d135c645cca9` | `a466e5166e844a64215362c288f3c353f702d7a2` | MERGED | `docs/implementation/supplier-retail-capability-report.md` | PASS |
| **P4.9** | `seller` | #12 | `675f1c26958d758ab45b757dbc720008c6d0b9c2` | `d75f84c66cfca9ab4c79c8a3495c5080be28a7af` | MERGED | `docs/implementation/storefront-performance-security-regression-report.md` | PASS |
| **P4.10**| `core` | #22 | `a4be416aea0b903795b620bb7f60f9f09951c182` | `9dffa2d700a1d0130efdad45beaf7e218bfa1f09` | MERGED | `docs/implementation/phase-04-implementation-report.md` | PASS |

---

## 4. Final Runtime Architecture

```
Customer Browser
      │
      ▼ (HTTP/HTTPS)
Next.js Storefront App (web/storefront)
      │
      ▼ HTTP JSON (Host header)
Seller storefront-api BFF
      ├──► Seller Redis Cache (storefront response payloads, keyed by host + revision)
      │
      ▼ HTTP JSON (X-Matjero-Service: seller, Bearer Service Token, X-Matjero-Storefront-Host)
Core core-api
      ├──► PostgreSQL DB (authoritative state: store, domain, catalog, themes, revisions)
      └──► RabbitMQ Broker (asynchronous events: outbox foundation)
```

- **Customer to Storefront**: Clean web application routing using Next.js App Router.
- **Seller BFF Layer**: `storefront-api` exposes tenant-scoped public storefront endpoints, handles Redis caching, and forwards backend catalog calls to `core-api`.
- **Core Engine**: `core-api` acts as single source of truth backed exclusively by PostgreSQL.
- **Service Identity Header**: Inter-service requests supply `X-Matjero-Service: seller` (or `admin`, `supplier`) to declare caller capability identity. Executable binaries are named `seller-api`, `admin-api`, and `supplier-api`.
- **Async Messaging**: Event distribution uses **RabbitMQ** only ([ADR-018](../plans/adr/ADR-018-rabbitmq-asynchronous-messaging-backbone.md)). Kafka is not planned. Phase 4 domain and storefront synchronous HTTP/JSON operations execute directly and do not publish asynchronous domain events.
- **Database Ownership**: Seller, Admin, and Supplier micro-services operate 100% DB-free. Core owns PostgreSQL persistence.

---

## 5. Store & Domain Lifecycle

Tenant authority is derived strictly from host resolution:

1. Customer connects with HTTP `Host` header (or proxy-forwarded `X-Forwarded-Host` behind trusted reverse proxy).
2. Seller `storefront-api` normalizes the hostname (lowercase, port stripped) and passes it in the `X-Matjero-Storefront-Host` header to `core-api`.
3. Core `StoreResolver` performs domain resolution against PostgreSQL.
4. Client-supplied `store_id` or `seller_id` parameters in public requests are strictly ignored for tenant identity.

Domain lifecycle states:
- `PENDING`: Custom domain registered, awaiting DNS TXT verification. Non-routable.
- `VERIFIED`: DNS TXT verification token verified, awaiting activation. Non-routable.
- `FAILED`: DNS verification failed or expired. Non-routable.
- `DISABLED`: Domain disabled by platform admin moderation. Non-routable.
- `ACTIVE`: Domain active and verified. **Only `ACTIVE` domains resolve publicly.**

Non-routable states (`PENDING`, `VERIFIED`, `FAILED`, `DISABLED`, unknown) fail closed with generic 404 responses. Admin disabling of an active custom domain automatically falls back to the store's primary platform domain according to Core domain lifecycle rules.

---

## 6. Theme Engine

The Theme Engine follows platform-controlled governance ([ADR-011](../plans/adr/ADR-011-theme-security.md)):

- **Platform Controlled**: Themes and version definitions are created and published by platform administrators.
- **Versioned**: Theme versions (`1.0.0`, etc.) carry immutable JSON schemas and default configuration.
- **Schema-Driven**: Theme configurations are validated against the theme version's JSON Schema (`santhosh-tekuri/jsonschema/v6`).
- **No Arbitrary JavaScript**: Sellers cannot inject custom JavaScript or unvalidated HTML scripts.
- **Draft / Published Split**: Sellers edit `draft_config` without impacting live storefronts. Publishing atomically validates the configuration, updates `published_config`, and bumps the store's storefront revision.
- **Preview Mechanism**: Short-lived, HMAC-SHA256 signed, store-scoped preview tokens (`X-Matjero-Storefront-Preview`) allow sellers to preview draft configurations securely. Preview responses carry `Cache-Control: private, no-store`.

---

## 7. Public Catalog / Pricing / Privacy

The public catalog read model (`modules/storefront/catalog_repository.go`) exposes store-scoped product listings:

- **Seller Price Authority**: Customers see only the active **Seller Listing Price**.
- **Supplier Privacy**: Supplier cost, wholesale price, supplier margins, supplier identity, contact details, and internal fulfillment metadata are strictly stripped and never serialized in public DTOs.
- **Store-Scoped Catalog**: Products are filtered through an `eligibleListings` CTE enforcing active store, market, listing status, category status, and product status.
- **Search Implementation**: Product search is PostgreSQL-backed and tenant-scoped. Keyword matching uses bound case-normalized substring matching (`LOWER(name) LIKE $n OR LOWER(description) LIKE $n OR LOWER(product_slug) LIKE $n`) over localized product name, description, and slug. No dedicated external search engine, PostgreSQL full-text search index (`tsquery`), or stemming/transliteration infrastructure is used in Phase 4.

---

## 8. Storefront Rendering

The customer storefront (`web/storefront`) is built with Next.js App Router using React Server Components (RSC):

- **Dynamic Layout Generation**: Page layouts, hero banners, section blocks, product grids, and header/footer configurations are dynamically rendered based on the active theme JSON configuration.
- **Theme Component Registry**: Clean component registry architecture maps theme keys to visual presentation components.
- **RSC Rendering**: Server-side rendering minimizes client component surface area.

---

## 9. Localization

Native bilingual support for English (`en`) and Arabic (`ar`):

- **URL Prefix**: `/en/...` and `/ar/...` path prefixes with root `/` locale negotiation.
- **Directionality**: Dynamic HTML `dir="ltr"` (English) and `dir="rtl"` (Arabic) layout attributes.
- **RTL Isolation**: Flexbox, grid, and spacing utilities adapt seamlessly without layout breakdown.

---

## 10. SEO

Complete server-side SEO implementation:

- **Metadata**: Server-rendered `title`, `description`, `OpenGraph`, and `Twitter` cards on all public pages.
- **Canonical & Hreflang**: Explicit canonical links and multi-language `hreflang` tags (`en`, `ar`, `x-default`).
- **Structured Data**: `Product` JSON-LD schema with HTML entity escaping to prevent XSS.
- **Robots & Sitemap**: Dynamic store-scoped `/robots.txt` and `/sitemap.xml` listing active store products and categories. Preview pages carry `noindex, nofollow` headers and suppress JSON-LD generation.

---

## 11. Redis Caching / Revision Model

Storefront caching relies on a revision-backed cache model:

- **Seller Redis Ownership**: Redis belongs strictly to Seller `storefront-api`. Core has zero Redis dependency.
- **Authoritative Core Revisions**: Core maintains a PostgreSQL table `storefront_revisions` tracking an integer `revision` per store.
- **Revision Bump Inventory**: Storefront revision is bumped (`revision = revision + 1`) on:
  - Public Store profile/status changes
  - Seller Listing create/price/status changes
  - Public Product changes
  - Public Category changes
  - Inventory/availability changes
  - Theme install/version/publish actions
- **No Revision Bump On**: Custom domain creation/verification/activation (domain safety relies on StoreResolver verification per request), Supplier wholesale price/cost updates, or theme draft configuration edits.
- **Cache Keys**: Seller constructs tenant-safe Redis keys incorporating host, locale, endpoint, and current revision (`storefront:store:{host}:rev:{rev_id}:...`).
- **Zero Scan Deletion**: Invalidation is implicit—revision bumps immediately route new requests to a new key namespace. Old cache keys expire via TTL.
- **Probe Efficiency**: Warm Redis responses bypass backend payload construction while lightweight revision probes ensure cache freshness.

---

## 12. Seller Theme Management

Seller administrators manage storefront aesthetics via Seller Dashboard (`web/seller`):

- Browse available platform themes and active version updates.
- Install, configure, and edit theme settings through a schema-validated UI form.
- Generate secure draft preview URLs and test live changes in an isolated iframe / tab.
- Publish draft configurations atomically to live storefronts (`POST /v1/seller/stores/{store_id}/theme/publish`).

---

## 13. Admin Domain Moderation

Platform administrators control custom domain governance via Admin API and Web (`matjeroapps/admin`):

- Platform-wide domain listing & filtering (`GET /v1/admin/domains`).
- Admin moderation actions: Disable custom domain (`POST /v1/admin/domains/{id}/disable`), Re-enable custom domain (`POST /v1/admin/domains/{id}/enable`).
- Canonical ownership: Seller handles domain registration (`POST /v1/seller/stores/{store_id}/domains`), verification (`POST /v1/seller/stores/{store_id}/domains/{domain_id}/verify`), and activation (`POST /v1/seller/stores/{store_id}/domains/{domain_id}/activate`). Admin handles platform-wide audit, disable, and re-enable.
- Fully audited actions enforcing `RolePlatformAdmin` authorization.

---

## 14. Supplier Retail Capability Prerequisite

Merged prerequisite (Core PR #21, migration `000009_supplier_retail_capability`):

- **Explicit Affiliation**: Establishes 1:1 retail capability affiliation between Supplier and Seller.
- **Store Single Ownership**: `stores` table maintains single ownership (`seller_id` only, no `supplier_id` column).
- **OWN / NETWORK Sourcing**: Derived dynamically from Supplier Offer ownership without persisting artificial `source_type` columns.
- **Owner Governance**: Retail provisioning restricted exclusively to active Supplier `OWNER` users.
- **Core-owned internal Supplier Retail capability routes, accessible only with Supplier service identity. Supplier frontend / Supplier API retail bridge remains deferred:**
  - `GET  /internal/v1/suppliers/{supplierID}/retail-capability`
  - `POST /internal/v1/suppliers/{supplierID}/retail-capability`
  - `GET  /internal/v1/suppliers/{supplierID}/stores`
  - `POST /internal/v1/suppliers/{supplierID}/stores`

---

## 15. P4.9 Security / Multi-Tenant / Performance Evidence

Seller repository contains the complete P4.9 Playwright E2E suite (`tests/e2e/`), executing 41 end-to-end tests against a mock core server stub (`fake-core`) and Redis instance:

### Playwright E2E Baseline: **41 / 41 PASSED**

- **Tenancy & Isolation**: Verified Store A vs. Store B isolation across products, categories, pricing, themes, domain routing, and cache keys.
- **Security Tests**: Verified XSS prevention, price integrity, supplier privacy, host spoofing rejection, cross-store IDOR rejection, and preview token forgery resistance.
- **SEO & Localization**: Verified hreflang, canonical URLs, JSON-LD, sitemap isolation, and RTL layout rendering.

### Measured Structural Call Counts:

| Page Route | Cold Revision Probes | Cold Payload Calls | Warm Revision Probes | Warm Payload Calls |
| :--- | :---: | :---: | :---: | :---: |
| **Home Page (`/`)** | 3 | 3 | 3 | 0 |
| **Catalog (`/products`)** | 3 | 3 | 3 | 0 |
| **Product Detail (`/products/[slug]`)** | 4 | 4 | 4 | 0 |
| **Search (`/search?q=...`)** | 3 | 3 | 3 | 0 |
| **Sitemap (`/sitemap.xml`)** | 5 | 5 | 5 | 0 |

*Explanation*: Warm Redis requests eliminate Core PAYLOAD HTTP calls (0 payload calls), while lightweight revision probes intentionally remain to verify authoritative cache freshness.

### Final Runtime Integration Smoke Status
FINAL RUNTIME INTEGRATION SMOKE NOT EXECUTED. Playwright E2E tests were executed against a mock core server stub (`fake-core`), while Core PostgreSQL integration tests were executed independently. A joint live Core + Seller runtime integration smoke was not executed.

---

## 16. Repository Independence

Strict adherence to [ADR-017](../plans/adr/ADR-017-repository-independence-and-runtime-service-boundaries.md):

- **Zero Cross-Repo Code Imports**: No Go package in `seller`, `admin`, or `supplier` imports source code from `core` or sibling repositories.
- **No `go.work`**: Production builds compile each repository in strict isolation with `GOWORK=off`.
- **HTTP / JSON Contracts**: All inter-service communication occurs via HTTP APIs using explicit DTOs.
- **DB-Free Micro-Services**: Seller, Admin, and Supplier services operate without database connections. Core is the sole database owner.

---

## 17. Database / Migration State

Core PostgreSQL migration sequence is fully up to date:

- `000001_event_delivery_foundation`
- `000002_create_outbox_tables`
- `000003_create_stores_table`
- `000004_create_catalog_tables`
- `000005_store_domain_lifecycle`
- `000006_store_domain_integrity`
- `000007_theme_engine_schema`
- `000008_storefront_revisions`
- `000009_supplier_retail_capability`

All migrations have been verified for up/down reversibility (`make migrate-check`) and tested against live PostgreSQL integration test suites.

---

## 18. OpenAPI State

OpenAPI specifications (`docs/api/`) across all actor repositories are 100% current:

- `core`: `docs/api/core/openapi.json`
- `seller`: `docs/api/seller/openapi.json`, `docs/api/storefront/openapi.json`
- `admin`: `docs/api/admin/openapi.json`
- `supplier`: `docs/api/supplier/openapi.json`

Ran `go run ./cmd/openapi-gen` across all Go repositories with zero drift detected.

---

## 19. Security Review

- **Tenant Boundary**: Store identity is derived strictly from trusted domain host headers. Parametric override of `store_id` is prevented.
- **Fail-Closed Errors**: Non-routable domains (unverified, disabled, unknown) return generic 404s. Backend service errors return clean 503s without leaking internal IP addresses, stack traces, or authorization tokens.
- **Theme Safety**: Strict schema validation and unsafe content filtering eliminate XSS risks in theme settings.
- **Data Privacy**: Supplier wholesale costs, supplier contacts, and internal platform fees are strictly withheld from customer-facing responses.

---

## 20. Validation Results Per Repository

### Core (`matjeroapps/core`)
- `gofmt -l .`: Clean (0 unformatted files)
- `git diff --check`: Clean (0 whitespace/diff errors)
- `GOWORK=off go mod tidy`: 0 changes to `go.mod` / `go.sum`
- `GOWORK=off go build ./...`: PASS
- `GOWORK=off go vet ./...`: PASS
- `GOWORK=off go test -count=1 ./...`: PASS (All packages ok)
- `GOWORK=off go run ./cmd/openapi-gen`: 0 spec drift
- `make migrate-check`: PASS
- `make docker-build`: Built `commerce-core-api:foundation`, `commerce-general-worker:foundation`, `commerce-scheduler:foundation`

### Seller (`matjeroapps/seller`)
- `GOWORK=off go mod tidy`: 0 changes
- `GOWORK=off go build ./...`: PASS
- `GOWORK=off go vet ./...`: PASS
- `GOWORK=off go test -count=1 ./...`: PASS
- `npm ci`: PASS (0 vulnerabilities)
- `npm run lint`: PASS
- `npm run typecheck`: PASS
- `npm run test`: PASS (Unit & Presentation tests)
- `npm run build`: PASS (`seller-web` and `storefront-web` standalone builds)
- `./scripts/run-e2e.sh`: PASS (41/41 Playwright E2E tests green)
- `GOWORK=off go run ./cmd/openapi-gen`: 0 spec drift
- `make docker-build`: Built `matjero-seller-api`, `matjero-seller-web`, `matjero-storefront-api`, `matjero-storefront-web`

### Admin (`matjeroapps/admin`)
- `GOWORK=off go build ./...`: PASS
- `GOWORK=off go vet ./...`: PASS
- `GOWORK=off go test -count=1 ./...`: PASS
- `npm ci`: PASS
- `npm run lint`: PASS
- `npm run typecheck`: PASS
- `npm run test`: PASS (Vitest: 5/5 test files, 46/46 tests green)
- Security Audit CI Correction: Merged Admin PR #7 fixed npm audit reproducibility via `npm ci && npm audit --audit-level=high`. Post-merge main CI run 33901156074 is 5/5 GREEN (backend, frontend, independence, openapi, security).
- `GOWORK=off go run ./cmd/openapi-gen`: 0 spec drift

### Supplier (`matjeroapps/supplier`)
- `GOWORK=off go build ./...`: PASS
- `GOWORK=off go vet ./...`: PASS
- `GOWORK=off go test -count=1 ./...`: PASS
- `npm ci`: PASS
- `npm run lint`: PASS
- `npm run typecheck`: PASS
- `GOWORK=off go run ./cmd/openapi-gen`: 0 spec drift

---

## 21. GitHub CI Evidence

All Phase 4 merged pull requests achieved clean green CI status prior to merge:

- **Core (`matjeroapps/core`)**: PR #6, #7, #8, #12, #13, #14, #15, #16, #18, #19, #20, #21, #22 — **CI SUCCESS**
- **Seller (`matjeroapps/seller`)**: PR #1, #2, #3, #4, #5, #6, #7, #8, #9, #10, #11, #12 — **CI SUCCESS**
- **Admin (`matjeroapps/admin`)**: PR #1, #2, #3, #4, #5, #6, #7 — **CI SUCCESS**
- **Supplier (`matjeroapps/supplier`)**: PR #1, #2 — **CI SUCCESS**

Post-merge main pipelines across all primary repositories are verified **GREEN**.

---

## 22. Known Limitations

1. **Single-Worker Playwright Design**: The E2E test harness (`scripts/run-e2e.sh`) runs Playwright tests using `workers: 1` due to shared process state in the lightweight fake-core mock server.
2. **Reverse Proxy Configuration Requirement**: In production deployments, ingress reverse proxies (Cloudflare, Nginx, AWS ALB) must be configured to overwrite `X-Forwarded-Host` securely to prevent host header injection.
3. **Automated OpenAPI Breaking-Change Detection**: Automated historical OpenAPI breaking-change diff checking remains deferred per specification.
4. **End-to-End Live PostgreSQL Multi-Repo Harness**: E2E multi-tenant testing is conducted via Seller Playwright against `fake-core`, complemented by Core's PostgreSQL integration suite.

---

## 23. Deferred / Out-of-Scope Work

The following functional areas remain explicitly out of scope for Phase 4:

- Cart management, Checkout workflows, Order creation, Inventory reservation, Shipping/Fulfillment calculations, Payment gateway integration, Refunds, Ledger entries, and Financial settlements.
- Unified marketplace discovery, Seller/Supplier reputation scoring, Product intelligence.
- Third-party theme developer ecosystem / marketplace.
- Arbitrary seller JavaScript execution.
- Dedicated external search engines (Elasticsearch, Meilisearch, Typesense).
- **Kafka or alternative message brokers** (NOT PLANNED; RabbitMQ remains sole async backbone).

---

## 24. What Is Next

With Phase 4 (Native Storefront & Theme Engine) fully complete and verified, the project is ready to transition to **Phase 5 — Cart, Checkout, Orders and Inventory Transactions**.

Architectural highlights for Phase 5:
- **Entities**: Customer, Customer Address, Cart, Cart Item, Checkout Session, Order, Order Item, Order Timeline, Order Note.
- **Order Lifecycle**: Explicit state machine spanning `PENDING`, `CONFIRMED`, `PROCESSING`, `READY_FOR_SHIPPING`, `SHIPPED`, `OUT_FOR_DELIVERY`, `DELIVERED`, `CANCELLED`, `RETURNED`.
- **Checkout Critical Path**: Validate Store, Validate Market, Validate Listings, Validate Prices, Reserve Inventory, Create Order, Create Order Items, Create Outbox Events, Commit, Return Order.
- **Transactional Outbox**: PostgreSQL Transactional Outbox foundation.
- **Outbox Publishing**: Polling Outbox Publisher pushing to RabbitMQ Event Transport.
- **Consumer Inbox / Idempotency**: processed event identity is `consumer_name` + `event_id`. Duplicate event delivery must be ignored safely.
- **Event Ordering**: causal ordering uses `aggregate_id` + `aggregate_version`. Timestamps are not authoritative ordering.

All Phase 6 (Shipping method/provider) and Phase 7 (Payment gateway, settlement) capabilities remain strictly out of scope for Phase 5.

---

## 25. Final Readiness Decision

READY FOR PHASE 5
