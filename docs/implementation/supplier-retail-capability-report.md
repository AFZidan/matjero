# Supplier Retail Capability + Explicit Supplier-Seller Affiliation Implementation Report

## Overview

This report documents the implementation of the Pre-P4.9 Core prerequisite: **Supplier Retail Capability and Explicit Supplier↔Seller Affiliation**.

This architecture enables a wholesale Supplier on the Matjero platform to operate direct-to-consumer retail stores under its own account while preserving complete tenant boundaries, store ownership invariants, listing source integrity, and public storefront privacy.

---

## Repository and Environment Metadata

- **Repository**: `/var/www/personal/matjero/core`
- **Branch**: `feature/supplier-retail-capability`
- **Base SHA**: `96cf98e5a1f1de3f388a86e316b5b59414d49d11`
- **Architectural Decision Record**: `docs/plans/adr/ADR-019-supplier-retail-capability.md` (Status: Accepted)
- **Database Migration**: `migrations/000009_supplier_retail_capability.up.sql` / `down.sql`

---

## Core Principles and Domain Invariants

### 1. Capabilities vs Account Types
Supplier and Seller are business **capabilities**, not mutually exclusive account types. The same entity may hold both a Supplier profile and a Seller profile linked explicitly via Core.

### 2. Store Ownership Invariant
Store ownership remains strictly owned by Seller (`stores.seller_id`). No `supplier_id` column was added to `stores`, and `stores` is strictly non-polymorphic.

```
Supplier
   ↓ (explicit 1:1 affiliation)
Seller
   ↓
Store (stores.seller_id)
```

### 3. Explicit Supplier ↔ Seller Affiliation & 1:1 Cardinality
Affiliations are stored in `supplier_seller_affiliations`:
- `supplier_id` (UUID, PRIMARY KEY, FK -> suppliers(id))
- `seller_id` (UUID, UNIQUE NOT NULL, FK -> sellers(id))
- `created_at` (TIMESTAMPTZ)

Core never infers affiliation from shared ZITADEL subjects, email addresses, codes, or member lists. Affiliation is explicitly persisted.

### 4. Core Authority
Supplier service never calls Seller service to execute Core business capabilities. Core provides authoritative internal APIs for Supplier retail operations.

### 5. Atomic Capability Provisioning & Membership Isolation
When a Supplier subject enables the retail capability, Core atomically creates:
- `sellers` profile
- `seller_settings`
- `seller_members` (initiating subject as active owner)
- `supplier_seller_affiliations`

All writes occur in a single PostgreSQL transaction. Existing `supplier_members` are NOT copied over; only the initiating subject receives explicit Seller membership.

### 6. Store Creation & Shared Primitive
Store creation logic was refactored into a shared private primitive `createStoreForSeller(...)` in `pkg/commerce`. Both `CreateStoreForSubject` (Seller flow) and `CreateSupplierStoreForSubject` (Supplier Retail flow) reuse identical reserved subdomain validation, platform domain allocation, and atomic store + domain creation logic.

### 7. Sourcing Invariants & Derive OWN vs NETWORK
- `Store` -> `SellerListing` -> `SupplierOffer` -> `Supplier`.
- Listing source integrity is strictly enforced: `seller_listing.supplier_offer_id` must point to an offer for the exact same `product_id` as `seller_listing.product_id`. Mismatches are rejected with validation error (`ErrInvalidInput`).
- Classification is derived at query time based on `offer.SupplierID == affiliation.SupplierID`:
  - `OWN`: Sourced from the affiliated Supplier.
  - `NETWORK`: Sourced from another platform Supplier.
- `source_type` is NOT persisted as duplicated state.
- For Supplier discovery, OWN products are prioritized (`CASE WHEN supplier_offer.supplier_id = affiliated_supplier_id THEN 0 ELSE 1 END`). Public storefront ranking is unchanged.

---

## Internal API Endpoints

The following internal endpoints are registered under `/internal/v1`:

1. `GET /internal/v1/suppliers/{supplierID}/retail-capability`
   - Returns explicit affiliation link and Seller profile.
2. `POST /internal/v1/suppliers/{supplierID}/retail-capability`
   - Provisions Seller profile and explicit 1:1 affiliation atomically.
3. `GET /internal/v1/suppliers/{supplierID}/stores`
   - Lists stores owned by the supplier's affiliated retail seller profile.
4. `POST /internal/v1/suppliers/{supplierID}/stores`
   - Creates a new retail store owned by the supplier's affiliated seller profile.

All endpoints require `X-Matjero-Service: supplier` (or `admin`), valid bearer service auth, and verified forwarded subject. `CallerSeller` cannot access supplier retail endpoints.

---

## Verification & Testing

### Automated Test Suite
- `pkg/commerce`:
  - `TestMigration000009_UpAndDown`: Validates 000009 up/down migration cycle and 1:1 database uniqueness constraints.
  - `TestSupplierRetailCapability_AtomicProvisioning`: Validates atomic creation of seller, settings, seller member, and affiliation, plus rollback on conflict.
  - `TestSupplierRetailCapability_MembershipIsolation`: Validates that only initiating subject receives seller owner role and other supplier members are isolated.
  - `TestSupplierRetailCapability_StoreOwnership`: Validates `store.seller_id` ownership invariant and `RequireSupplierRetailStoreAccess`.
  - `TestSupplierRetailCapability_SourceIntegrity`: Validates rejection when listing product_id does not match supplier offer product_id.
  - `TestSupplierRetailCapability_SourceDerivation_OwnAndNetwork`: Validates derivation of OWN vs NETWORK classification.
- `internal/coreapi`:
  - `TestSupplierRetailAPI_SecurityAndCapabilities`: Validates HTTP service authentication, caller isolation (seller caller rejected with 403), subject authorization, capability creation (201 Created), and store listing/creation (201 Created).
  - `TestSpecMatchesRouter`: Validates that router routes match OpenAPI spec declarations 1-to-1.

### Build and Quality Commands
- `gofmt -w . && git diff --check` -> Clean
- `GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum` -> Clean
- `GOWORK=off go build ./...` -> Clean
- `GOWORK=off go vet ./...` -> Clean
- `GOWORK=off go test -count=1 ./...` -> All 100% GREEN
- `GOWORK=off go run ./cmd/openapi-gen` -> OpenAPI spec updated and valid

---

## Explicitly Deferred Items

The following items are out of scope for this Core prerequisite unit and deferred to future phases (P4.9 / P5):
- Supplier frontend / Supplier Hub retail capability UI & bridge
- Supplier custom-domain management UI
- Full sourcing discovery engine / catalog selection UX
- Sourcing listing-selection UX
- Orders, checkout, and payment workflows
