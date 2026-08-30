# Phase 2 Implementation Specification: Commerce Domain Foundation

## Objective

Establish the core commerce-domain aggregates, persistence invariants, and application boundaries that later seller, supplier, storefront, order, payment, shipping, and integration phases will build on.

This phase must make invalid commercial state impossible to persist where the schema can enforce it, and must keep all monetary and market-scoped behavior deterministic and auditable.

## Scope

In scope:

- Supplier, seller, store, catalog, supplier commerce, seller commerce, fulfillment-location, and inventory foundation modeling.
- Market-scoped ownership and cross-market integrity rules.
- Translation-friendly catalog schema.
- Deterministic money representation and helper utilities.
- Strong reservation groundwork in PostgreSQL.
- Search-ready domain identifiers, searchable fields, and event surfaces for future indexing.
- Core repositories, services, and domain guards for the above entities.
- Tests that prove market isolation, translation behavior, and concurrency-sensitive invariants.

Out of scope:

- Seller storefront rendering and themes.
- Checkout, carts, orders, payments, shipping, returns, and finance posting flows.
- External integrations, sync jobs, and webhooks.
- Dedicated search engines and a `search-api` microservice.
- Search indexes, analytics, and event fan-out.
- UI work beyond future API consumption requirements.

## Dependencies

- Phase 0 engineering foundation.
- Phase 1 identity, locale negotiation, and market reference data.
- Existing ADRs:
  - ADR-001 PostgreSQL Source of Truth
  - ADR-002 Market Isolation
  - ADR-003 Money Representation
  - ADR-004 Fulfillment Location
  - ADR-005 Inventory Reservation
  - ADR-008 Double-Entry Ledger
  - ADR-009 Integration Deployment Model
  - ADR-010 Storefront Tenant Resolution
  - ADR-011 Theme Security
  - ADR-013 ZITADEL OIDC and JWT Validation
  - ADR-014 Locale Negotiation and Direction
  - ADR-015 Market Reference Data and Launch Seed

## Architecture Decisions

1. Commerce Core stays server-side and repository-backed. Actor APIs remain thin transport layers.
2. Market scope is carried explicitly on every market-sensitive aggregate and enforced in both Go logic and PostgreSQL constraints.
3. Supplier and seller are separate aggregates with their own membership/settings tables.
4. Store is always single-market.
5. Product is catalog truth, while offer/listing captures commercial availability and pricing.
6. Inventory belongs to SKU plus fulfillment location, not to a seller listing.
7. Money is stored as minor units with ISO currency, never floating point.
8. Translations are normalized tables keyed by entity ID plus locale.
9. Future extensibility is preserved by keeping seller-owned inventory, supplier-backed inventory, and marketplace exposure as separate concepts rather than overloading a single table.
10. Search readiness is designed as a derived-read-model contract, not as a Phase 2 runtime dependency.

## Aggregate Boundaries

### Supplier aggregate

- Owns supplier identity, status, and settings.
- Has many supplier members.
- Has many supplier markets.
- A supplier market scopes fulfillment locations and offers.

### Seller aggregate

- Owns seller identity, status, and settings.
- Has many seller members.
- A seller can later own many stores.

### Store aggregate

- Owns a store, store domain records, and store settings.
- Belongs to exactly one market.
- References a seller as its owner.

### Catalog aggregate

- Owns product, category, variant, SKU, attribute, attribute value, and media metadata.
- Products are not market-specific by default.
- Translations are separate child records.

### Supplier commerce aggregate

- Owns supplier product links, offers, prices, and availability.
- Offer is market-specific.
- Offer is the commercial bridge between a supplier and a catalog product.

### Seller commerce aggregate

- Owns seller listings, seller prices, and listing status.
- Listing belongs to exactly one store and one supplier offer or seller-owned product target.
- Listing market must match store market and supplier offer market when linked.

### Inventory aggregate

- Owns fulfillment locations, inventory snapshots, and reservations.
- Inventory is tracked per SKU and fulfillment location.
- Reservation changes are transactional and strongly consistent.

## Database Model

The phase introduces a new migration chain after the Phase 1 market reference tables.

### Common patterns

- UUID primary keys for commerce aggregates.
- `market_code CHAR(2)` on all market-scoped records.
- `status TEXT NOT NULL` for lifecycle state.
- `created_at` and `updated_at` timestamps.
- Aggregate version fields where needed for incremental indexing and concurrency-sensitive workflows.
- Normalized translation tables with `(entity_id, locale)` primary keys.
- Check constraints for state and money sanity.
- Composite foreign keys where market isolation must be enforced at the database boundary.

### Core tables

- `suppliers`
- `supplier_members`
- `supplier_markets`
- `supplier_settings`
- `seller_identities` or `sellers`
- `seller_members`
- `seller_settings`
- `stores`
- `store_domains`
- `store_settings`
- `products`
- `product_translations`
- `categories`
- `category_translations`
- `variants`
- `skus`
- `attributes`
- `attribute_translations`
- `attribute_values`
- `attribute_value_translations`
- `media_metadata`
- `supplier_products`
- `supplier_offers`
- `supplier_offer_prices`
- `supplier_offer_availability`
- `seller_listings`
- `seller_listing_prices`
- `seller_listing_status_history` if needed for auditability
- `fulfillment_locations`
- `inventory_snapshots`
- `inventory_reservations`
- Search-ready event contracts for product, store, supplier, offer, listing, category, attribute, variant, and SKU changes.

### Key relational rules

- `stores (id, market_code)` is uniquely constrained so `(id, market_code)` can be a target for composite foreign keys.
- `supplier_offers (id, market_code)` is uniquely constrained for the same reason.
- `seller_listings` stores both `store_id` and `supplier_offer_id` plus its own `market_code`.
- `seller_listings(store_id, market_code)` must reference `stores(id, market_code)`.
- `seller_listings(supplier_offer_id, market_code)` must reference `supplier_offers(id, market_code)`.
- Inventory snapshots reference fulfillment location and SKU.
- Fulfillment locations are market-scoped through supplier market ownership.

## Transaction Boundaries

- Creating or updating a supplier market, offer, listing, or inventory snapshot happens inside a single PostgreSQL transaction.
- Market invariant checks happen before write attempts and are repeated with database constraints.
- Reservation creation, release, and expiry updates are transactional and row-safe.
- Translation writes for a single entity should be transactional with the base entity write.

## Domain Invariants

- A supplier may operate in multiple markets.
- A supplier market belongs to one supplier and one market.
- A seller may later own multiple stores in different markets.
- Every store belongs to exactly one market.
- A seller listing cannot cross markets.
- An offer cannot cross markets.
- Product translations must be locale-keyed and non-overwriting.
- Monetary values must be non-negative integer minor units.
- Inventory cannot be decremented below zero by reservation logic.
- Stock availability must be derived from snapshots and reservations, not by inference from listing records.

## Money Representation

- Use `amount_minor int64` plus `currency_code CHAR(3)` for persisted monetary amounts.
- Do not use float types in Go for any money or percentage logic.
- Percentages, rounding, and currency precision live in a shared money helper package.
- Validate currency code against market currency where the value is market-scoped.

## Market Isolation Rules

- `Store Market = Seller Listing Market = Supplier Offer Market`.
- This rule is enforced twice:
  - In application/domain logic before persistence.
  - In PostgreSQL via composite foreign keys and supporting unique constraints.
- Market changes are not casual updates once dependent commercial records exist.
- Cross-border sourcing remains unsupported in Phase 2.

## Inventory Ownership Model

- Inventory belongs to `SKU + Fulfillment Location`.
- Fulfillment location is owned by a supplier market for now.
- Inventory snapshots represent available/on-hand/reserved counts.
- Seller listings do not own inventory.

## Inventory Reservation Model

- Initial reservation strategy is a conditional update against snapshot rows in PostgreSQL.
- Reservation rows record quantity, status, expiry, and external correlation fields.
- Reservation lifecycle:
  - created
  - held
  - released
  - expired
  - consumed
- Concurrency safety depends on row-level locking or atomic conditional updates, not Redis.

## Search Readiness Architecture

- Products, stores, suppliers, categories, attributes, variants, SKUs, supplier offers, and seller listings must have stable IDs and clear searchable/filterable fields.
- Product, category, store, and supplier translations must remain normalized so future multilingual indexing can build Arabic and English documents without schema redesign.
- Category and attribute modeling must support future faceted filtering.
- Market, availability, pricing, status, supplier, store, category, and attribute data must be projectable into search documents.
- Aggregate `updated_at` values and/or version fields must be available for incremental indexing.
- Domain events for product, store, supplier, offer, listing, category, attribute, variant, SKU, and inventory-relevant changes must carry enough identity and change metadata for a future indexer to build Product, Store, and Supplier search indexes.
- Search remains a derived read model. PostgreSQL stays the source of truth.
- No business write operation may depend on the search index being available.
- Initial search may use PostgreSQL where appropriate.
- A future Search Indexer should be able to consume domain events and build dedicated Product, Store, and Supplier indexes.
- Do not add a `search-api` microservice in Phase 2.

## Application Services

- `SuppliersService`
- `SellersService`
- `StoresService`
- `CatalogService`
- `SupplierCommerceService`
- `SellerCommerceService`
- `InventoryService`
- `MoneyService` or shared money helper

Each service:

- validates invariants at the boundary,
- uses repositories inside a transaction where needed,
- keeps transport concerns out of domain logic.

## Repository Boundaries

- One repository package per domain area, not one repository per HTTP route.
- Repositories expose persistence operations only.
- Repositories do not enforce request authorization.
- Repositories do not shape HTTP responses.

## Authorization Hooks

- Phase 2 stays compatible with the existing actor principal model from Phase 1.
- Transport-level role checks remain in the actor APIs.
- Resource ownership checks for supplier, seller, and store access are implemented in commerce services once membership tables exist.

## Observability

- Keep structured logging around mutating commerce operations.
- Add correlation-aware error wrapping in service and repository layers.
- Record enough context to trace entity ID, market code, and actor principal without logging secrets.

## Testing Strategy

- Unit tests for money helpers and invariant checks.
- Repository integration tests against PostgreSQL for:
  - normalized translations,
  - composite market isolation,
  - store single-market enforcement,
  - supplier multi-market support,
  - reservation concurrency behavior,
  - non-negative inventory guards.
- Service tests for boundary validation and authorization-adjacent ownership checks.

## Implementation Tasks

### P2.1 Commerce schema and shared primitives

Goal:

- Add shared money helpers and base commerce table conventions.

Dependencies:

- Phase 1 market tables and currency reference data.

Branch name:

- `feature/p2-commerce-domain-foundation`

Implementation:

- Create shared money types/helpers.
- Add migration scaffolding for commerce domain tables.
- Add reusable audit/check constraint conventions.

Database changes:

- Base types and utility constraints for commerce tables.

Tests:

- Money helper unit tests.

Acceptance criteria:

- No float-based money logic exists in the new phase code.
- Money formatting and validation behave deterministically.

### P2.2 Supplier, seller, and store foundations

Goal:

- Implement supplier, seller, store, member, settings, and store-domain ownership primitives.

Dependencies:

- P2.1

Branch name:

- `feature/p2-commerce-foundation-ownership`

Implementation:

- Create supplier, seller, store, membership, domain, and settings models.
- Add repository/service methods for creating and reading these entities.
- Enforce store single-market ownership.

Database changes:

- `suppliers`, `supplier_members`, `supplier_markets`, `supplier_settings`
- `sellers`, `seller_members`, `seller_settings`
- `stores`, `store_domains`, `store_settings`

Tests:

- Supplier can span multiple markets.
- Seller can exist independently of a single country.
- Store cannot change market once dependent records exist.

Acceptance criteria:

- Ownership and market scope are represented explicitly and read back correctly.

### P2.3 Catalog and translation foundation

Goal:

- Implement catalog entities with normalized translation tables.

Dependencies:

- P2.1

Branch name:

- `feature/p2-catalog-foundation`

Implementation:

- Create product, category, variant, SKU, attribute, attribute value, and media metadata models.
- Persist translations in normalized child tables.
- Add locale-aware read/write helpers.

Database changes:

- `products`, `product_translations`, `categories`, `category_translations`, `variants`, `skus`, `attributes`, `attribute_values`, `media_metadata`

Tests:

- Product and category translations support multiple locales without schema changes.

Acceptance criteria:

- No `name_ar` / `name_en` columns exist for catalog content.

### P2.4 Supplier offers and seller listings

Goal:

- Implement market-scoped commercial availability and listing foundations.

Dependencies:

- P2.2, P2.3

Branch name:

- `feature/p2-offers-listings`

Implementation:

- Create supplier products, offers, pricing, availability, seller listings, and seller pricing.
- Enforce store/offer market isolation in application logic and the database.
- Ensure seller listing pricing is separate from supplier cost.

Database changes:

- `supplier_products`, `supplier_offers`, `supplier_offer_prices`, `supplier_offer_availability`, `seller_listings`, `seller_listing_prices`

Tests:

- Cross-market listings are rejected.
- Multiple suppliers can offer the same conceptual product.
- One supplier can offer in multiple markets with distinct prices and availability.

Acceptance criteria:

- Invalid cross-market commercial records cannot be persisted.

### P2.5 Fulfillment locations and inventory reservation groundwork

Goal:

- Implement fulfillment-location, inventory snapshot, and reservation primitives.

Dependencies:

- P2.2, P2.4

Branch name:

- `feature/p2-inventory-foundation`

Implementation:

- Create fulfillment locations per supplier market.
- Add inventory snapshot and reservation records.
- Implement atomic reservation/restore operations.

Database changes:

- `fulfillment_locations`, `inventory_snapshots`, `inventory_reservations`

Tests:

- Reservation concurrency and non-negative inventory behavior.

Acceptance criteria:

- Inventory is anchored to SKU plus fulfillment location.

### P2.6 Service wiring, auth hooks, and validation sweep

Goal:

- Wire the new commerce services into the repo structure and validate the whole phase.

Dependencies:

- P2.1 through P2.5

Branch name:

- `feature/p2-commerce-validation`

Implementation:

- Add service wiring and repository interfaces.
- Add actor-facing hooks where needed for future Phase 3 use.
- Run migration, unit, and integration validation.

Database changes:

- No new schema unless validation uncovers a missing invariant.

Tests:

- Full targeted test sweep for all new commerce packages.

Acceptance criteria:

- Phase 2 core domain foundations are implemented and validated.

### P2.7 Search-readiness contract

Goal:

- Record search-ready domain, event, and projection requirements without introducing a dedicated search runtime.

Dependencies:

- P2.1 through P2.6

Branch name:

- `feature/p2-search-readiness`

Implementation:

- Ensure aggregates expose stable IDs, `updated_at`, and/or version fields suitable for incremental indexing.
- Define domain event payload expectations for search-relevant entities.
- Keep translation tables and facet-capable catalog fields normalized.
- Preserve the ability to extract a future search service without changing Commerce Core write paths.

Database changes:

- No dedicated search index or search service schema.

Tests:

- Verify search-relevant entities expose stable identifiers and projection-friendly fields.

Acceptance criteria:

- Commerce Core can later feed a dedicated search indexer without redesigning the domain model.

## Dependencies Between Tasks

- P2.1 unblocks all later units.
- P2.2 and P2.3 can proceed in parallel after P2.1, but P2.4 depends on both.
- P2.5 depends on ownership and offer/listing market context.
- P2.6 depends on all prior units.
- P2.7 depends on all prior units and is documentation-plus-contract work only.

## Acceptance Criteria

- Supplier, seller, store, catalog, offer, listing, fulfillment, and inventory foundation tables exist.
- Market isolation is enforced in PostgreSQL and Go logic.
- Translations are normalized.
- Inventory reservation logic is concurrency-safe at the database boundary.
- Money is represented without floats.
- Search-ready IDs, translations, and event contracts are in place for future indexing.
- Tests prove the important invariants.

## Definition of Done

- The Phase 2 implementation spec is checked in.
- The commerce foundation schema and packages are implemented.
- The targeted tests for new commerce packages pass.
- The repository remains aligned with Phase 1 markets, locale, and auth foundations.
- No unresolved Phase 2 architecture ambiguity remains in the code or docs.
