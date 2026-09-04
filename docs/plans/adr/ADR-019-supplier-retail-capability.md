# ADR-019: Supplier Retail Capability and Explicit Supplier-Seller Affiliation

## Status

Accepted

## Context

In the Matjero commerce platform, business entities operate as Suppliers (wholesalers / product source owners) and/or Sellers (retail merchants operating consumer-facing storefronts).

Previously, system capabilities assumed a business entity operated exclusively as either a Supplier or a Seller. However, real-world business models frequently require a Supplier to operate direct-to-consumer retail stores while remaining a wholesale supplier on the platform. Furthermore, a Supplier-operated retail store may sell both its own products and products sourced from other platform Suppliers.

To support direct retail operations by Suppliers without compromising tenant boundaries, platform security, or data integrity, Core must establish a canonical architecture for retail capabilities and explicit affiliations.

## Decisions

### 1. Capabilities vs Account Types

Supplier and Seller are **capabilities**, not mutually exclusive account types.

A single business entity may possess:
- Supplier capability (wholesaling, catalog management, offer creation, fulfillment)
- Seller / Retail capability (storefront management, custom domains, retail listings)

The canonical ownership and capability link is:

```
Supplier
   ↓ (explicit 1:1 affiliation)
Seller
   ↓
Store
```

### 2. Store Ownership Invariant

`Store` records remain strictly owned by `Seller`:

- `Store` → `seller_id`

**Invariants**:
- Never introduce `store.supplier_id`.
- Never make `stores` polymorphic (e.g. `seller_id OR supplier_id`).
- Stores remain 100% seller-owned database entities.

### 3. Explicit Supplier ↔ Seller Affiliation

Affiliation between a Supplier profile and a Seller profile MUST be explicit, persisted, and Core-owned in PostgreSQL (`supplier_seller_affiliations`).

**Critical Identity Rule**:
Core MUST NOT infer Supplier ↔ Seller affiliation from shared attributes such as:
- Same ZITADEL subject
- Same email address
- Same company name
- Same code
- Same owner or member lists

The affiliation table enforces a strict **1:1 relationship**:
- One Supplier profile ↔ at most one retail Seller profile.
- One Seller profile ↔ at most one Supplier profile.

### 4. Core as the Single Authority

The Supplier service NEVER calls the Seller service to execute Core business capabilities. Core is the authoritative domain and data boundary.

Core provides dedicated Supplier-scoped internal APIs (`/internal/v1/suppliers/{supplierID}/...`) to manage retail capabilities and stores cleanly without cross-service RPC.

### 5. Atomic Retail Capability Provisioning

A Supplier explicitly enables its retail capability via Core.

Provisioning MUST execute as a single atomic PostgreSQL transaction creating:
- `sellers` (Seller profile)
- `seller_settings` (Default seller settings)
- `seller_members` (Initial owner membership)
- `supplier_seller_affiliations` (Explicit 1:1 link)

If any operation fails, the transaction rolls back completely, leaving no orphan records.

### 6. Initial Seller Membership & Isolation

The authenticated Supplier subject that explicitly enables the retail capability becomes an active Seller member (`role = owner`, `status = active`) for the new Seller profile.

Supplier membership is NOT implicitly converted into Seller membership. Existing `supplier_members` rows are NOT copied into `seller_members`.

### 7. Explicit Listing Source Semantics & Derivation

The product flow for any storefront listing follows:

```
Store
  ↓
Seller Listing
  ↓
Supplier Offer
  ↓
Supplier
```

Source classification is derived at query time based on persisted relations:
- `source Supplier == affiliated Supplier` → **OWN** product/source
- `source Supplier != affiliated Supplier` → **NETWORK** product/source

**Invariants**:
- Do NOT persist `source_type` (e.g. `own` or `network`) as duplicated state.
- For Supplier-operated retail stores, selling own goods still references `supplier_offer_id`. `NULL` `supplier_offer_id` remains reserved for future seller-owned inventory models.
- Listing source integrity is strictly enforced: `seller_listing.supplier_offer_id` MUST point to an offer for the exact same `product_id` as the `seller_listing.product_id`.

### 8. Own-Products-First Sourcing Semantics

For Supplier sourcing and catalog discovery interfaces:
- Offers from the affiliated Supplier are prioritized before Network Supplier offers:
  ```sql
  CASE
    WHEN supplier_offer.supplier_id = affiliated_supplier_id THEN 0
    ELSE 1
  END
  ```
- This prioritization rule applies **ONLY** to Supplier internal sourcing and discovery tools.
- It does **NOT** alter public storefront product ranking, search relevance, or consumer-visible sorting.

### 9. Public Storefront Privacy

Customer-facing Storefront APIs MUST NOT expose internal sourcing metadata, including:
- Supplier identity or affiliation
- OWN vs NETWORK classification
- `supplier_offer_id`
- Wholesale pricing or supplier margins

### 10. Domain Management Future Compatibility

Supplier custom domain management will be executed via Core's reusable access primitive:
`RequireSupplierRetailStoreAccess(ctx, subject, supplierID, storeID)`.

This ensures future Supplier domain workflows remain direct from `supplier-api` → Core without service-to-service calls to `seller-api`.
