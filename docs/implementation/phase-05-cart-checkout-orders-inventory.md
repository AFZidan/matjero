# Matjero — Phase 5: Cart, Checkout, Orders and Inventory Transactions

**Status:** Specification — DO NOT implement until this document is merged.
**Base SHA:** `4ebd38a496bf0d58ce3c6f286cca08436460da55`
**Canonical branch:** `feature/p5-phase-spec`

---

## 1. Executive Summary

Phase 5 introduces the complete transactional commerce loop for Matjero. It
implements Cart management, Checkout Session processing, atomic Order creation,
and authoritative Inventory Reservations. This phase transforms the platform
from a catalog-browsing service into a transactional marketplace while preserving
every existing architectural invariant: PostgreSQL authority, RabbitMQ async
backbone, repository independence (ADR-017), market isolation, and strict
multi-tenant boundaries.

The checkout critical path is executed within **ONE single authoritative
PostgreSQL transaction**:

```
BEGIN
  1. SELECT checkout_session FOR UPDATE (by id)
     - If status = 'expired': ROLLBACK → return checkout_expired

  2. SELECT cart FOR UPDATE (by id & store_id)
     - Assert cart status = 'active' (or 'finalized' during replay)

  3. Load Cart Items in deterministic order (by seller_listing_id, sku_id ASC)

  4. Compute server-side canonical fingerprint from:
     - checkout_session_id
     - cart_id
     - sorted cart lines (seller_listing_id, sku_id, quantity, expected_unit_price_minor, expected_currency_code)
     - shipping_address_snapshot & contact_email
     - customer_id (if present)

  5. If checkout_session.status = 'finalized':
     - Compare computed fingerprint with stored finalize_fingerprint
     - If matching: SELECT order WHERE checkout_session_id = $1 → COMMIT → return Order
     - If mismatch: ROLLBACK → return idempotency_conflict

  6. If open:
     - Freeze single confirmation deadline: $deadline = transaction_now + configured_duration
     - Lock candidate Inventory Snapshots FOR UPDATE (ordered deterministically by id ASC)
     - Revalidate Store, Listings, SKUs, and Prices (compare with expected_unit_price_minor → price_changed)
     - Validate Supplier Offer → Supplier Product → Product lineage (for Supplier-sourced listings)
     - Reserve Inventory (insert held reservation with expires_at = $deadline + record movement)
     - Allocate Store Order Number (from store_order_sequences)
     - Insert Order (with confirmation_deadline_at = $deadline)
     - Insert Order Items (referencing inventory_reservation_id, fulfillment_location_id, supplier_offer_id)
     - Insert Order Address & Order Timeline entry
     - Insert Outbox Event (commerce.order.created)
     - Update Checkout Session (status = 'finalized', finalize_fingerprint = $computed_fingerprint, finalized_at = now())
     - Update Cart (status = 'checked_out')
COMMIT
```

All steps commit or roll back together. Any validation failure (e.g. `price_changed`, `insufficient_inventory`) causes a transaction rollback, leaving `checkout_sessions` in `open` state so the Customer may correct inputs and retry.

---

## 2. Goals

- Implement the `customers`, `customer_addresses`, `carts`, `cart_items`,
  `checkout_sessions`, `store_order_sequences`, `orders`, `order_items`,
  `order_addresses`, `order_timeline`, and `order_notes` aggregates.
- Extend `fulfillment_locations` to support Seller-owned locations (Store-scoped
  inventory).
- Define a strictly consistent, oversell-proof atomic single-transaction checkout.
- Activate the Transactional Outbox (ADR-006) with a multi-publisher lease/claim design and per-event lease renewal.
- Define the Consumer Inbox (ADR-007) semantics for Phase 5 consumers.
- Establish the Order State Machine with explicit transition authority and single frozen `confirmation_deadline_at`.
- Activate reservation expiry and Order cancellation compensation workflows.
- Provide API surfaces in `storefront-api` (anonymous guest path) and `seller-api`
  (dashboard path) in the `matjeroapps/seller` repository, calling `core-api` over
  authenticated internal HTTP.

---

## 3. Non-Goals

Phase 5 must NOT implement:

- **Shipping provider integration** — Phase 6 owns shipment creation, tracking,
  rate calculation, label generation, and all provider adapters.
- **Payment gateway integration** — Phase 7 owns payment authorization, capture,
  refund, and provider webhooks.
- **Tax engine** — No tax calculation in Phase 5. `total_minor = subtotal_minor`.
- **Promotions, coupons, or loyalty** — Future phases.
- **Multi-Store checkout** — Carts are strictly isolated to one Store.
- **Kafka** — RabbitMQ remains the sole async backbone (ADR-018).
- **Cross-repository compile-time coupling** — ADR-017 remains mandatory.
- **Customer IAM / Provider selection** — Phase 5 supports Guest Checkout as its primary operational path. Customer login/token integration is DEFERRED until a dedicated Customer IAM provider decision is made.
- **Supplier runtime work** — Phase 5 adds no Supplier API surfaces.
- **Admin runtime work** — Phase 5 adds no Admin API surfaces.
- **SHIPPED / OUT_FOR_DELIVERY / DELIVERED state activation** — Defined in schema but inactive without Phase 6 shipping authority.
- **RETURNED state activation** — Defined in schema but owned by a future returns workflow.

---

## 4. Existing Phase 4 / Commerce Baseline

Phase 4 delivered:

- Multi-tenant catalog: `products`, `variants`, `skus`, `seller_listings`,
  `seller_listing_prices`.
- Supplier Retail Capability: `supplier_seller_affiliations` (ADR-019).
- Market isolation enforced at the DB FK level across `stores`, `seller_listings`,
  `supplier_offers`, `fulfillment_locations`.
- `inventory_snapshots` (`on_hand_qty`, `reserved_qty`, `version` per SKU ×
  Fulfillment Location).
- `inventory_reservations` (`reservation_token`, `status`, `quantity`, `expires_at`).
  Existing status values: `held`, `consumed`, `released`, `expired`.
- `inventory_movements` — append-only ledger tracking inventory state changes.
- `outbox_events` and `processed_events` schemas present.
- Core acts as sole PostgreSQL authority. Actor services communicate with Core
  exclusively via authenticated HTTP (`/internal/v1/...`, ADR-017).

---

## 5. Key Business Decisions

| Topic | Decision |
|---|---|
| Customer identity | Core maintains a lightweight `customers` profile per Store. Guest checkout is the primary operational target. Customer schema fields (`identity_provider`, `identity_subject`) remain password-free and forward-compatible; actual Customer IAM provider integration is DEFERRED. |
| Customer FK delete policy | `ON DELETE RESTRICT` for Customer composite references `(customer_id, store_id)` on `carts`, `checkout_sessions`, and `orders`. `customer_addresses` uses `ON DELETE CASCADE` from `customers`. |
| Store + Market composite FKs | Composite FK `(store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT` on `customers`, `carts`, and `orders` to prevent Store/Market mismatches. |
| Cart identity & prices | Identified by a high-entropy bearer token digest (`cart_token_digest`) in an HttpOnly cookie. `cart_items.expected_unit_price_minor` and `expected_currency_code` are `NOT NULL`. |
| Cart locking | Parent `carts` row is locked (`FOR UPDATE`) during all Cart item mutations (add, update, remove, merge) and during checkout finalization. Mutations on `checked_out` carts fail deterministically. |
| Single-transaction checkout | No two-phase `finalizing` fence, no durable `failed` status in DB. Canonical checkout session states: `open`, `finalized`, `expired`. Validation failures roll back and leave session `open`. |
| Idempotency fingerprint | Core computes `finalize_fingerprint` server-side from canonical deterministic serialization of checkout inputs after locking the cart and loading cart lines. Clients NEVER submit a trusted fingerprint. |
| Non-circular FKs | `orders.checkout_session_id` FK → `checkout_sessions(id)` (UNIQUE, NOT NULL). `order_items.inventory_reservation_id` FK → `inventory_reservations(id)` (UNIQUE, NOT NULL). No reciprocal columns. |
| Operational source lineage | `order_items` internal lineage columns (`supplier_offer_id`, `fulfillment_location_id`, `inventory_reservation_id`) use `ON DELETE RESTRICT` so historical source lineage is never erased. |
| Tenant composite FKs | Database enforces Store isolation via composite UNIQUE keys on `(id, store_id)` for `customers`, `carts`, `checkout_sessions` and matching composite FKs across relationships. |
| Order sequence | Monotonic per-Store order numbers generated via atomic updates on a dedicated `store_order_sequences` table. Formatted as stable Store-local strings (e.g. `#100001`). |
| Single frozen deadline | Compute `$deadline = transaction_now + configured_duration` ONCE inside checkout. Use exact same `$deadline` value for `orders.confirmation_deadline_at` and `inventory_reservations.expires_at`. |
| Inventory movement delta | `quantity_delta` in `inventory_movements` strictly represents `on_hand_qty` delta. Reservation events (`held`, `released`, `expired`) write `quantity_delta = 0`. Confirmation (`consumed`) writes `quantity_delta = -quantity`. Restock writes `quantity_delta = +quantity`. |
| Outbox lease & renewal | Multi-publisher outbox claims use explicit columns (`publish_claim_id`, `publish_claimed_at`, `publish_attempts`, `next_attempt_at`). Events near lease expiry are renewed atomically before publishing. |
| Seller-owned inventory | `fulfillment_locations` supports Store-scoped ownership (`store_id NOT NULL`, `supplier_id NULL`) alongside Supplier-scoped ownership. |
| Price change handling | Authoritative price re-read inside checkout transaction. Mismatch with Cart snapshot rejects with `price_changed`. |

---

## 6. Repository Boundaries & Architecture

Phase 5 obeys ADR-017. No cross-repository Go imports. All inter-service
collaboration is HTTP-based.

`matjeroapps/seller` repository owns:
- `apps/storefront-api` (Go binary, public anonymous API)
- `apps/seller-api` (Go binary, authenticated Seller API)
- `web/storefront` (Next.js web storefront app)
- `web/seller` (React seller dashboard app)

`seller-hub` is currently README-only and receives **NO Phase 5 runtime changes**.

```
                         Customer Browser
                                │
                                ▼
                ┌───────────────────────────────┐
                │   web/storefront (Next.js)    │  ← seller repo (web/storefront)
                └───────────────┬───────────────┘
                                │ HTTP
                                ▼
                ┌───────────────────────────────┐
                │  apps/storefront-api (Go)     │  ← seller repo (apps/storefront-api)
                │  /v1/storefront/...           │
                │  (anonymous public routes)    │
                └───────────────┬───────────────┘
                                │ authenticated HTTP
                                │ X-Matjero-Service: seller
                                │ X-Matjero-Storefront-Host: <trusted host>
                                ▼
                ┌───────────────────────────────┐
                │  apps/core-api (Go)           │  ← core repo
                │  /internal/v1/...             │
                │  PostgreSQL authority         │
                └───────────────┬───────────────┘

                       Seller Dashboard Browser
                                │
                                ▼
                ┌───────────────────────────────┐
                │   web/seller (React)          │  ← seller repo (web/seller)
                └───────────────┬───────────────┘
                                │ HTTP + OIDC JWT
                                ▼
                ┌───────────────────────────────┐
                │  apps/seller-api (Go)         │  ← seller repo (apps/seller-api)
                │  /v1/seller/...               │
                │  (authenticated seller routes)│
                └───────────────┬───────────────┘
                                │ authenticated HTTP
                                │ X-Matjero-Service: seller
                                ▼
                ┌───────────────────────────────┐
                │  apps/core-api (Go)           │  ← core repo
                │  /internal/v1/...             │
                └───────────────┬───────────────┘
```

---

## 7. Customer Model & IAM Scope

### 7.1 Identity and Scope

Core maintains a lightweight `customers` aggregate per Store:

- **Guest Checkout:** Fully supported. Email and delivery address captured at checkout. `identity_provider` and `identity_subject` remain NULL.
- **Authenticated Customer Schema:** Fields (`identity_provider`, `identity_subject`) are present and constrained by partial unique index `(store_id, identity_provider, identity_subject)`.
- **Runtime Customer IAM Provider:** DEFERRED. Phase 5 does NOT invent or integrate an end-user Customer OIDC provider. `storefront-api` remains anonymously callable for Phase 5 guest checkout. No OIDC middleware is added to `storefront-api` in Phase 5.

### 7.2 Table Blueprints

#### `customers` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | NOT NULL |
| `market_code` | `CHAR(2)` | NOT NULL |
| `identity_provider` | `TEXT` | Nullable |
| `identity_subject` | `TEXT` | Nullable |
| `email` | `TEXT` | Nullable |
| `display_name` | `TEXT` | Nullable |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('active', 'blocked')) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key for composite FKs:** `UNIQUE (id, store_id)`.
**Store/Market Composite FK:** `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT`.
**Partial Index:** `UNIQUE (store_id, identity_provider, identity_subject) WHERE identity_provider IS NOT NULL AND identity_subject IS NOT NULL`.

#### `customer_addresses` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `customer_id` | `UUID` | NOT NULL |
| `store_id` | `UUID` | NOT NULL |
| `label` | `TEXT` | Nullable |
| `recipient_name` | `TEXT` | NOT NULL |
| `phone` | `TEXT` | Nullable |
| `address_line_1` | `TEXT` | NOT NULL |
| `address_line_2` | `TEXT` | Nullable |
| `city` | `TEXT` | NOT NULL |
| `region` | `TEXT` | Nullable |
| `postal_code` | `TEXT` | Nullable |
| `country_code` | `CHAR(2)` | NOT NULL |
| `is_default` | `BOOLEAN` | NOT NULL DEFAULT false |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Composite FK:** `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE CASCADE`.

---

## 8. Cart Aggregate & Mutation Locking

### 8.1 Identity & Bearer Token Digest

Carts are identified externally by a high-entropy `cart_token` held in an `HttpOnly` cookie.
Core stores `cart_token_digest` (SHA-256 hash). The internal key is `carts.id` (UUID).

### 8.2 Cart Parent Row Locking Invariant

Every Cart mutation (`add_item`, `update_quantity`, `remove_item`, `merge_cart`) MUST lock the parent `carts` row first:

```sql
SELECT * FROM carts WHERE id = $1 FOR UPDATE;
```

If `carts.status == 'checked_out'`, the mutation MUST fail deterministically with error code `cart_expired` or `conflict`. No line additions, updates, or removals are permitted on a checked-out Cart.

### 8.3 Table Blueprints

#### `carts` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | NOT NULL |
| `market_code` | `CHAR(2)` | NOT NULL |
| `customer_id` | `UUID` | Nullable |
| `cart_token_digest` | `TEXT` | NOT NULL UNIQUE (SHA-256) |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('active', 'checked_out', 'abandoned', 'expired')) |
| `expires_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Store/Market Composite FK:** `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT`.
**Customer Composite FK:** `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE RESTRICT`.
**Partial Index:** `UNIQUE (customer_id, store_id) WHERE status = 'active' AND customer_id IS NOT NULL`.

#### `cart_items` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `cart_id` | `UUID` | NOT NULL FK → `carts(id)` ON DELETE CASCADE |
| `seller_listing_id` | `UUID` | NOT NULL FK → `seller_listings(id)` ON DELETE RESTRICT |
| `sku_id` | `UUID` | NOT NULL FK → `skus(id)` ON DELETE RESTRICT |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0 AND quantity <= 10000) |
| `expected_unit_price_minor` | `BIGINT` | NOT NULL (display snapshot captured at add-to-cart) |
| `expected_currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique Constraint:** `UNIQUE (cart_id, seller_listing_id, sku_id)`.

---

## 9. SKU / Listing / Supplier Source Semantics & Lineage

Checkout validates that every Cart Item maintains strict catalog and sourcing lineage:

1. `seller_listing.status == 'active'`
2. `seller_listing.store_id == cart.store_id`
3. `seller_listing.market_code == store.market_code`
4. `sku → variant → product_id == seller_listing.product_id`

### 9.1 Supplier-Sourced Listing Validation (`supplier_offer_id NOT NULL`)

The exact database schema lineage is:
```
SellerListing.supplier_offer_id → SupplierOffer.id
  ├── SupplierOffer.supplier_product_id → SupplierProduct.id
  │     └── SupplierProduct.product_id  == SellerListing.product_id  ✓
  ├── SupplierOffer.supplier_id          == candidate FulfillmentLocation.supplier_id ✓
  └── SupplierOffer.market_code        == Store.market_code ✓
```

*Note on Schema Authority:* There is NO `supplier_offer.product_id` column in the database. Product lineage is verified via `SupplierOffer → SupplierProduct → product_id`.

### 9.2 Seller-Owned Listing Validation (`supplier_offer_id IS NULL`)

The candidate `fulfillment_location` must belong to the Store:
```
FulfillmentLocation.store_id == cart.store_id  ✓
FulfillmentLocation.market_code == store.market_code ✓
```

Public Storefront DTOs **never** expose internal supplier data (`supplier_id`, `supplier_offer_id`, `fulfillment_location_id`, wholesale costs, or reservation tokens).

---

## 10. Checkout Session Aggregate

### 10.1 Blueprint & State Machine

A Checkout Session represents intent to finalize an order. Its canonical status states are:
- `open`: Active checkout session eligible for finalization.
- `finalized`: Transaction completed successfully; linked Order created.
- `expired`: Pre-finalization window elapsed without completion.

No persistent `finalizing` or `failed` states are stored in the database. Validation failures roll back and leave the session in `open` state.

#### `checkout_sessions` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `cart_id` | `UUID` | NOT NULL |
| `customer_id` | `UUID` | Nullable |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('open', 'finalized', 'expired')) |
| `expires_at` | `TIMESTAMPTZ` | NOT NULL (pre-finalization session lifetime) |
| `shipping_address_snapshot` | `JSONB` | Nullable (address snapshot) |
| `contact_email` | `TEXT` | Nullable |
| `finalize_fingerprint` | `TEXT` | Nullable (server-computed deterministic SHA-256 fingerprint) |
| `finalized_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Composite FKs:**
- `FOREIGN KEY (cart_id, store_id) REFERENCES carts(id, store_id) ON DELETE RESTRICT`
- `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE RESTRICT`

*Note:* `checkout_sessions.expires_at` governs pre-finalization session validity. Post-checkout confirmation deadline is stored separately on `orders.confirmation_deadline_at`.

---

## 11. Price Validation & Calculation

1. Inside the atomic checkout transaction, Core re-reads current `seller_listing_prices` (`amount_minor`, `currency_code`).
2. If current price ≠ `cart_item.expected_unit_price_minor`, Core rolls back and returns `price_changed` with the current authoritative price.
3. Order totals are calculated deterministically:
   ```
   line_total_minor = unit_price_minor * quantity
   subtotal_minor   = SUM(line_total_minor)
   total_minor      = subtotal_minor
   ```
4. Client-submitted price totals are NEVER trusted.

---

## 12. Single-Transaction Checkout & Server-Computed Idempotency

### 12.1 Server-Computed Idempotency Fingerprint

The client submits semantic inputs only (`session_id`, delivery address, contact email). It does NOT submit an opaque fingerprint string.

After locking the Checkout Session and parent Cart, and after loading Cart lines in deterministic order, Core computes `finalize_fingerprint` as a SHA-256 digest of a canonical JSON payload containing:
- `checkout_session_id`
- `cart_id`
- Sorted list of Cart line items: `(seller_listing_id, sku_id, quantity, expected_unit_price_minor, expected_currency_code)`
- Address snapshot & contact details
- Customer context (`customer_id` if present)

### 12.2 Single-Transaction Flow & Execution Order

Checkout is executed in **ONE PostgreSQL transaction**:

```sql
BEGIN;

-- 1. Lock Checkout Session row
SELECT * FROM checkout_sessions WHERE id = $session_id FOR UPDATE;

-- 2. Check expired
IF status = 'expired' THEN
  ROLLBACK;
  RETURN checkout_expired;
END IF;

-- 3. Lock Parent Cart Row
SELECT * FROM carts WHERE id = checkout_session.cart_id AND store_id = checkout_session.store_id FOR UPDATE;
ASSERT carts.status = 'active' OR checkout_session.status = 'finalized';

-- 4. Load Cart Items in deterministic order (by seller_listing_id, sku_id ASC)
-- 5. Compute server-side canonical fingerprint from loaded inputs

-- 6. Check replay state if finalized
IF checkout_session.status = 'finalized' THEN
  IF checkout_session.finalize_fingerprint = $computed_fingerprint THEN
    SELECT * FROM orders WHERE checkout_session_id = $session_id;
    COMMIT;
    RETURN existing_order;
  ELSE
    ROLLBACK;
    RETURN idempotency_conflict;
  END IF;
END IF;

-- 7. If status = 'open': proceed with checkout execution
--    a. Freeze single confirmation deadline: $deadline = now() + configured_confirmation_duration
--    b. Validate Store status = 'active' & market match
--    c. Validate Listings, SKUs, and reread listing prices (compare to expected_unit_price_minor → price_changed)
--    d. Validate Supplier Offer → Supplier Product → Product lineage (for Supplier listings)
--    e. Lock candidate inventory_snapshots (ordered by id ASC)
--    f. Reserve inventory (insert inventory_reservations status='held', expires_at = $deadline)
--    g. Allocate Order Number from store_order_sequences
--    h. Insert Order (confirmation_deadline_at = $deadline)
--    i. Insert Order Items (referencing inventory_reservation_id, fulfillment_location_id, supplier_offer_id)
--    j. Insert Order Address & Order Timeline (to_status='pending', actor_type='checkout')
--    k. Insert Outbox Event (commerce.order.created)
--    l. Update Checkout Session: status = 'finalized', finalize_fingerprint = $computed_fingerprint, finalized_at = now()
--    m. Update Cart: status = 'checked_out'

COMMIT;
```

### 12.3 Concurrency Behavior & Failure Scenarios

- **Concurrent finalization requests:** The second request blocks on `SELECT ... FOR UPDATE` on `checkout_sessions`. When the first transaction commits, the second request wakes, loads cart lines, computes fingerprint, reads `status = 'finalized'`, verifies fingerprint match, and returns the existing Order.
- **Crash before COMMIT:** Entire transaction rolls back; session remains `open`; cart remains `active`.
- **Crash after COMMIT before HTTP response:** Retried request hits step 6, sees `status = 'finalized'`, verifies fingerprint, and returns the existing Order.
- **Validation failures:** Validation errors (e.g. `price_changed`, `insufficient_inventory`) cause ROLLBACK, leaving `checkout_sessions` in `open` state so the Customer may retry.

---

## 13. Order Sequence Allocation

Order numbers are allocated from a dedicated per-Store sequence table within the checkout transaction.

#### `store_order_sequences` Table

| Column | Type | Constraints |
|---|---|---|
| `store_id` | `UUID` | PK FK → `stores(id)` ON DELETE RESTRICT |
| `next_value` | `BIGINT` | NOT NULL DEFAULT 100001 |

#### Atomic Allocation Query

```sql
INSERT INTO store_order_sequences (store_id, next_value)
VALUES ($store_id, 100002)
ON CONFLICT (store_id) DO UPDATE
SET next_value = store_order_sequences.next_value + 1
RETURNING next_value - 1;
```

Formatted order number: `#` + `allocated_value` (e.g. `#100001`).
Authoritative uniqueness: `UNIQUE (store_id, order_number)` on `orders`.

---

## 14. Inventory Reservation Lifecycle & Movement Semantics

### 14.1 Reservation Status States

- `held`: Active reservation created at checkout. Holds stock (`reserved_qty += quantity`). `expires_at` is set to `$deadline` (matching `orders.confirmation_deadline_at`).
- `consumed`: Order confirmed by Seller. Stock decremented (`reserved_qty -= quantity`, `on_hand_qty -= quantity`). Terminal.
- `released`: Order cancelled before confirmation. Stock released (`reserved_qty -= quantity`). Terminal.
- `expired`: Confirmation deadline elapsed. Stock released (`reserved_qty -= quantity`). Terminal.

### 14.2 `inventory_movements` Delta Semantics

In accordance with Core repository semantics, `quantity_delta` in `inventory_movements` represents **`on_hand_qty` delta**:

| Event | `movement_type` | `quantity_delta` | `on_hand_qty` change | `reserved_qty` change |
|---|---|---|---|---|
| Reservation `held` | `reservation_held` | `0` | `0` | `+quantity` |
| Reservation `released` (cancel PENDING) | `reservation_released` | `0` | `0` | `-quantity` |
| Reservation `expired` (timeout) | `reservation_expired` | `0` | `0` | `-quantity` |
| Reservation `consumed` (confirm) | `reservation_consumed` | `-quantity` | `-quantity` | `-quantity` |
| Order cancellation restock (cancel CONFIRMED/PROCESSING) | `order_cancellation_restock` | `+quantity` | `+quantity` | `0` |

---

## 15. Concurrency Strategy & Lock Ordering

To prevent deadlocks across concurrent commerce operations, all operations must acquire PostgreSQL locks in strict order:

1. `checkout_sessions` row (by `id`)
2. `carts` parent row (by `id`)
3. `inventory_snapshots` rows (ordered by `id ASC`)
4. `store_order_sequences` row (during order creation)
5. `orders` row (for status transitions)

---

## 16. Fulfillment Location Ownership

`fulfillment_locations` supports dual ownership branches:

- **Supplier-owned:** `supplier_id NOT NULL`, `supplier_market_id NOT NULL`, `store_id NULL`.
- **Seller-owned (Phase 5):** `supplier_id NULL`, `supplier_market_id NULL`, `store_id NOT NULL`.

#### Ownership CHECK Constraint

```sql
CHECK (
  (supplier_id IS NOT NULL AND supplier_market_id IS NOT NULL AND store_id IS NULL)
  OR
  (supplier_id IS NULL AND supplier_market_id IS NULL AND store_id IS NOT NULL)
)
```

Composite FK for Seller branch: `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT`.

---

## 17. Order Aggregate & Non-Circular FK Blueprints

#### `orders` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_number` | `TEXT` | NOT NULL |
| `store_id` | `UUID` | NOT NULL |
| `market_code` | `CHAR(2)` | NOT NULL |
| `customer_id` | `UUID` | Nullable |
| `checkout_session_id` | `UUID` | NOT NULL UNIQUE FK → `checkout_sessions(id)` ON DELETE RESTRICT |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('pending', 'confirmed', 'processing', 'ready_for_shipping', 'shipped', 'out_for_delivery', 'delivered', 'cancelled', 'returned')) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `subtotal_minor` | `BIGINT` | NOT NULL CHECK (subtotal_minor >= 0) |
| `total_minor` | `BIGINT` | NOT NULL CHECK (total_minor >= 0) |
| `confirmation_deadline_at` | `TIMESTAMPTZ` | NOT NULL (frozen Seller confirmation deadline) |
| `cancellation_reason` | `TEXT` | Nullable |
| `aggregate_version` | `BIGINT` | NOT NULL DEFAULT 1 |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique Constraints:** `UNIQUE (store_id, order_number)`, `UNIQUE (checkout_session_id)`.
**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Index:** `CREATE INDEX orders_status_deadline_idx ON orders (status, confirmation_deadline_at);`.
**Store/Market Composite FK:** `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT`.
**Composite FKs:**
- `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE RESTRICT`
- `FOREIGN KEY (checkout_session_id, store_id) REFERENCES checkout_sessions(id, store_id) ON DELETE RESTRICT`

#### `order_items` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `seller_listing_id` | `UUID` | Nullable FK → `seller_listings(id)` ON DELETE SET NULL |
| `product_id` | `UUID` | Nullable FK → `products(id)` ON DELETE SET NULL |
| `variant_id` | `UUID` | Nullable FK → `variants(id)` ON DELETE SET NULL |
| `sku_id` | `UUID` | Nullable FK → `skus(id)` ON DELETE SET NULL |
| `supplier_offer_id` | `UUID` | Nullable FK → `supplier_offers(id)` ON DELETE RESTRICT |
| `fulfillment_location_id` | `UUID` | NOT NULL FK → `fulfillment_locations(id)` ON DELETE RESTRICT |
| `inventory_reservation_id` | `UUID` | NOT NULL UNIQUE FK → `inventory_reservations(id)` ON DELETE RESTRICT |
| `product_title_snapshot` | `TEXT` | NOT NULL |
| `sku_code_snapshot` | `TEXT` | NOT NULL |
| `unit_price_minor` | `BIGINT` | NOT NULL CHECK (unit_price_minor >= 0) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0) |
| `line_total_minor` | `BIGINT` | NOT NULL CHECK (line_total_minor >= 0) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

*Lineage Invariant:* Operational source lineage columns (`supplier_offer_id` when non-null, `fulfillment_location_id`, `inventory_reservation_id`) use `ON DELETE RESTRICT` so historical fulfillment origin can never be erased. Catalog display references (`seller_listing_id`, `product_id`, `variant_id`, `sku_id`) use `ON DELETE SET NULL` because historical text snapshots (`product_title_snapshot`, `sku_code_snapshot`, `unit_price_minor`, `currency_code`) preserve customer-visible receipts.

#### `order_addresses` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL UNIQUE FK → `orders(id)` ON DELETE RESTRICT |
| `address_type` | `TEXT` | NOT NULL CHECK (address_type IN ('shipping')) |
| `recipient_name` | `TEXT` | NOT NULL |
| `phone` | `TEXT` | Nullable |
| `address_line_1` | `TEXT` | NOT NULL |
| `address_line_2` | `TEXT` | Nullable |
| `city` | `TEXT` | NOT NULL |
| `region` | `TEXT` | Nullable |
| `postal_code` | `TEXT` | Nullable |
| `country_code` | `CHAR(2)` | NOT NULL |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

#### `order_timeline` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `from_status` | `TEXT` | Nullable |
| `to_status` | `TEXT` | NOT NULL |
| `actor_type` | `TEXT` | NOT NULL CHECK (actor_type IN ('checkout', 'customer', 'seller', 'admin', 'scheduler', 'system')) |
| `actor_subject` | `TEXT` | Nullable |
| `reason` | `TEXT` | Nullable |
| `metadata` | `JSONB` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

#### `order_notes` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `author_subject` | `TEXT` | NOT NULL |
| `visibility` | `TEXT` | NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal')) |
| `body` | `TEXT` | NOT NULL |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

---

## 18. Order State Machine

| From | To | Authority | Precondition | Inventory Effect | Outbox Event |
|---|---|---|---|---|---|
| — | `pending` | Checkout | Checkout validation pass | `held` (reserved_qty += qty) | `commerce.order.created` |
| `pending` | `confirmed` | Seller | — | `consumed` (on_hand -= qty, reserved -= qty) | `commerce.order.status_changed` |
| `pending` | `cancelled` | Customer / Seller | — | `released` (reserved -= qty) | `commerce.order.status_changed` |
| `pending` | `cancelled` | Scheduler | `confirmation_deadline_at <= now()` | `expired` (reserved -= qty) | `commerce.order.status_changed` |
| `confirmed` | `processing` | Seller | — | None | `commerce.order.status_changed` |
| `confirmed` | `cancelled` | Seller | — | Restock: `on_hand += qty` (movement `order_cancellation_restock`) | `commerce.order.status_changed` |
| `processing` | `ready_for_shipping` | Seller | — | None | `commerce.order.status_changed` |
| `processing` | `cancelled` | Seller | — | Restock: `on_hand += qty` | `commerce.order.status_changed` |

Transitions from `ready_for_shipping` onward (`shipped`, `out_for_delivery`, `delivered`) belong to Phase 6. `returned` belongs to a future returns phase.

---

## 19. Reservation Expiry Workflow (Two-Stage Locking)

To prevent lock retention across non-transactional boundaries, the reservation expiry scheduler executes in two explicit stages:

### Stage 1: Bounded Candidate Discovery

Query candidate Order IDs whose frozen confirmation deadline has elapsed:

```sql
SELECT id FROM orders
WHERE status = 'pending'
  AND confirmation_deadline_at <= now()
ORDER BY confirmation_deadline_at ASC
LIMIT $batch_size;
```

### Stage 2: Per-Order Transactional Expiry

For EACH candidate Order ID from Stage 1, execute a dedicated PostgreSQL transaction:

```sql
BEGIN;

-- Lock Order row & re-verify status & deadline
SELECT * FROM orders
WHERE id = $order_id
  AND status = 'pending'
  AND confirmation_deadline_at <= now()
FOR UPDATE;

-- If row not found (already confirmed, cancelled, or handled by another worker)
IF no_row THEN
  ROLLBACK;
  CONTINUE to next candidate;
END IF;

-- Lock linked inventory snapshots in deterministic id ASC order
SELECT * FROM inventory_snapshots
WHERE id = ANY($snapshot_ids)
ORDER BY id ASC
FOR UPDATE;

-- Lock linked held reservations
SELECT * FROM inventory_reservations
WHERE id = ANY($reservation_ids)
  AND status = 'held'
FOR UPDATE;

-- Execute state changes
UPDATE orders
SET status = 'cancelled',
    cancellation_reason = 'confirmation_timeout',
    aggregate_version = aggregate_version + 1,
    updated_at = now()
WHERE id = $order_id AND status = 'pending';

UPDATE inventory_reservations
SET status = 'expired', updated_at = now()
WHERE id = ANY($reservation_ids) AND status = 'held';

UPDATE inventory_snapshots
SET reserved_qty = reserved_qty - $quantity,
    version = version + 1,
    updated_at = now()
WHERE id = $snapshot_id;

INSERT INTO inventory_movements (id, inventory_snapshot_id, movement_type, quantity_delta, on_hand_qty, reserved_qty, reason)
VALUES (...); -- quantity_delta = 0 for expired reservation

INSERT INTO order_timeline (order_id, from_status, to_status, actor_type, reason)
VALUES ($order_id, 'pending', 'cancelled', 'scheduler', 'confirmation_timeout');

INSERT INTO outbox_events (event_type, aggregate_id, payload)
VALUES ('commerce.order.status_changed', $order_id, ...);

COMMIT;
```

---

## 20. Outbox Multi-Publisher Lease & Per-Event Renewal

### 20.1 Schema Extension & Partial Index

Migration F adds claim columns and partial index to `outbox_events`:

| Column | Type | Default / Nullability |
|---|---|---|
| `publish_claim_id` | `UUID` | NULL |
| `publish_claimed_at` | `TIMESTAMPTZ` | NULL |
| `publish_attempts` | `INTEGER` | NOT NULL DEFAULT 0 |
| `next_attempt_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

```sql
CREATE INDEX outbox_events_unpublished_claim_idx
ON outbox_events (next_attempt_at, created_at, event_id)
WHERE published_at IS NULL;
```

### 20.2 Bounded Claim & Per-Event Lease Renewal

Runtime duration configuration: `OUTBOX_CLAIM_LEASE_DURATION` (default 30 seconds).

1. **Batch Claim:**
```sql
BEGIN;
SELECT event_id FROM outbox_events
WHERE published_at IS NULL
  AND next_attempt_at <= now()
  AND (
    publish_claim_id IS NULL
    OR publish_claimed_at < (now() - OUTBOX_CLAIM_LEASE_DURATION)
  )
ORDER BY next_attempt_at ASC, created_at ASC, event_id ASC
LIMIT $batch_size
FOR UPDATE SKIP LOCKED;

UPDATE outbox_events
SET publish_claim_id = $worker_claim_uuid,
    publish_claimed_at = now()
WHERE event_id = ANY($claimed_ids);
COMMIT;
```

2. **Per-Event Renewal Before Publish:**
   Before publishing each event in the claimed batch:
   - Check remaining lease duration (`now() - publish_claimed_at`).
   - If remaining lease time < 10 seconds (near expiry), atomically renew:
     ```sql
     UPDATE outbox_events
     SET publish_claimed_at = now()
     WHERE event_id = $event_id
       AND publish_claim_id = $worker_claim_uuid
       AND published_at IS NULL;
     ```
   - If renewal update affects 0 rows (lease lost or reassigned), **skip publishing that event**.

3. **Publisher Confirm Acknowledge:**
```sql
UPDATE outbox_events
SET published_at = now(),
    publish_claim_id = NULL,
    publish_claimed_at = NULL
WHERE event_id = $1
  AND publish_claim_id = $worker_claim_uuid
  AND published_at IS NULL;
```

4. **Broker Failure Backoff:**
```sql
UPDATE outbox_events
SET publish_attempts = publish_attempts + 1,
    publish_claim_id = NULL,
    publish_claimed_at = NULL,
    next_attempt_at = now() + (interval '1 second' * power(2, LEAST(publish_attempts, 6)))
WHERE event_id = $1
  AND publish_claim_id = $worker_claim_uuid;
```

Events are NEVER deleted or silently discarded due to publish failure.

---

## 21. Event Catalog

- `commerce.order.created` (schema_version: 1) — Emitted when a PENDING Order is created.
- `commerce.order.status_changed` (schema_version: 1) — Emitted on Order status transitions.

Payloads are privacy-safe: no PII, no wholesale costs, no internal location or reservation IDs.

---

## 22. RabbitMQ Topology & Consumer Inbox

- Exchange: `commerce.events` (topic, durable).
- Routing Keys: `order.created`, `order.status_changed`.
- Consumer Inbox: Handled via `processed_events` table using `(consumer_name, event_id)` unique constraint committed atomically in the same PostgreSQL transaction as consumer side-effects.

---

## 23. API Boundaries

- `storefront-api` (in `matjeroapps/seller`): Public anonymous routes `/v1/storefront/carts`, `/v1/storefront/checkout/sessions`.
- `seller-api` (in `matjeroapps/seller`): Authenticated routes `/v1/seller/stores/{store_id}/orders`.
- `core-api` (in `matjeroapps/core`): Internal authenticated routes `/internal/v1/...`.

---

## 24. Error Contract

Standardized HTTP error codes: `not_found` (404), `unauthorized` (401), `forbidden` (403), `invalid_argument` (400), `validation_error` (422), `market_mismatch` (422), `insufficient_inventory` (409), `price_changed` (409), `listing_unavailable` (409), `cart_expired` (409), `checkout_expired` (409), `idempotency_conflict` (409), `invalid_order_transition` (409), `conflict` (409), `internal_error` (500).

---

## 25. Security & Privacy

- Bearer Cart Token Digest: Token held in HttpOnly cookie; SHA-256 stored in DB.
- Tenant Isolation: DB composite UNIQUE keys and Store/Market composite FKs enforce Store boundary.
- Price Tampering Defense: Core re-reads authoritative listing prices inside checkout transaction.
- Internal Data Sanitation: Public DTOs omit internal IDs and costs.

---

## 26. Database Migration Plan

Phase 5 proposes migrations to `core`:
- Migration A: `fulfillment_locations` ownership extension (support Seller-owned locations with Store/Market composite FK).
- Migration B: `customers` & `customer_addresses` tables (with Store/Market composite FK).
- Migration C: `carts` & `cart_items` tables (with Store/Market composite FK & NOT NULL price snapshot fields).
- Migration D: `checkout_sessions` table (with `finalize_fingerprint`).
- Migration E: `store_order_sequences`, `orders` (with Store/Market composite FK & `confirmation_deadline_at`), `order_items` (with ON DELETE RESTRICT operational lineage), `order_addresses`, `order_timeline`, `order_notes`.
- Migration F: `outbox_events` claim extension columns & partial index.

---

## 27. OpenAPI Ownership Plan

OpenAPI contracts are owned per-repository and validated against drift in CI:

- **Core Internal API:** `core/docs/api/internal/openapi.json` (updated by P5.1–P5.6).
- **Seller Storefront API:** `seller/docs/api/storefront/openapi.json` (in `matjeroapps/seller`, updated by P5.7).
- **Seller Dashboard API:** `seller/docs/api/seller/openapi.json` (in `matjeroapps/seller`, updated by P5.8).

P5.0 itself changes zero generated OpenAPI documents.

---

## 28. Capability Rollout Gates

Feature exposure is controlled at actor boundaries via capability-named environment flags:

- `STOREFRONT_CHECKOUT_ENABLED` — Controls cart, checkout, and order placement routes in `storefront-api`.
- `SELLER_ORDER_MANAGEMENT_ENABLED` — Controls order management routes in `seller-api`.

Partially implemented functionality must not be exposed to traffic until the entire vertical slice is ready.

---

## 29. Observability

Operational metrics tracked:
- `checkout_attempts_total` (counter) & `checkout_failures_total` (counter, labeled by reason)
- `inventory_reservation_conflicts_total` (counter)
- `reservation_expiry_processed_total` (counter)
- `order_status_transition_total` (counter) & `invalid_order_transition_total` (counter)
- `outbox_unpublished_count` (gauge) & `outbox_oldest_unpublished_age_seconds` (gauge)
- `outbox_publish_failures_total` (counter)
- `consumer_duplicate_events_total` (counter)

---

## 30. Known Risk Matrix

| Risk | Mitigation |
|---|---|
| Oversell under high concurrency | `SELECT FOR UPDATE` + `CHECK (reserved_qty <= on_hand_qty)` |
| Duplicate Orders on retry | Checkout session locking + `UNIQUE (checkout_session_id)` + server fingerprint comparison |
| Stale prices charged | Authoritative listing price re-read + `price_changed` rejection |
| Cross-Store tenant IDOR | Host-derived store context + DB composite FKs `(store_id, market_code)` & `(customer_id, store_id)` |
| Reservation leakage | Single frozen `confirmation_deadline_at` + automated two-stage scheduler expiry job |
| Confirmation timeout races | Atomic status check `WHERE status = 'pending'` + `FOR UPDATE` |
| Duplicate RabbitMQ delivery | Consumer Inbox idempotency via `processed_events` atomic commit |
| Outbox backlog | Bounded claim lease loop + per-event renewal + gauge metrics & alerting |
| Customer PII leakage | Privacy-safe domain event payloads + log masking |
| Future Shipping/Payment compatibility | Full 9-state Order state model with inactive future states |

---

## 31. Phase 5 Definition of Done

- [ ] Core database migrations complete & verified.
- [ ] All database composite FK constraints (`store_id, market_code`) and index definitions verified.
- [ ] Concurrent last-unit stock checkout tests green.
- [ ] Durable checkout idempotency and HTTP-replay loss tests green.
- [ ] Inventory reservation lifecycle & movement `quantity_delta` tests green.
- [ ] Order state-machine table-driven tests green.
- [ ] Outbox multi-publisher claim lease and confirm reliability tests green.
- [ ] Seller Storefront Cart & Checkout complete (in `matjeroapps/seller`).
- [ ] Seller Order Management complete (in `matjeroapps/seller`).
- [ ] Multi-tenant isolation & security audit tests green.
- [ ] OpenAPI documentation current & drift-checked across repositories.
- [ ] Zero cross-repository compile-time dependencies (ADR-017).
- [ ] All CI builds green across `core` and `seller` repositories.
- [ ] Zero Phase 6 (Shipping) or Phase 7 (Payment) functionality leaked into Phase 5.

---

## 32. Testing Strategy & Mandatory Test Matrix

Must include deterministic unit and integration tests (zero sleep-based concurrency tests):

### 32.1 Concurrency & Stock Allocation Tests
1. **Two available units under N checkouts:** N concurrent checkouts for a 2-unit stock → exactly 2 succeed.
2. **Same SKU across multiple fulfillment locations:** Allocates from location with lowest `inventory_snapshot.id ASC` deterministically.
3. **Multi-item in-tx rollback:** Item A succeeds, Item B stock fails → full rollback; zero surviving reservations.
4. **Supplier Offer cross-location isolation:** Supplier Offer cannot allocate Seller-owned location or another Supplier's location.
5. **Seller-owned listing isolation:** Seller-owned listing (supplier_offer_id IS NULL) cannot allocate Supplier location.

### 32.2 Idempotency & Replay Tests
6. **10 concurrent identical finalizes:** Exactly 1 Order created; all 10 return exact same Order.
7. **HTTP response loss retry:** Client retries after network drop → returns same committed Order.
8. **Fingerprint mismatch:** Same finalized session + different semantic fingerprint → `idempotency_conflict`.
9. **Validation failure state:** Validation failure (e.g. `price_changed`) rolls back and leaves Checkout Session `open` for retry.

### 32.3 Reservation & State Machine Race Tests
10. **Confirm vs expiry race:** Concurrent Seller confirm + scheduler expiry → exactly one wins; no double inventory release.
11. **Cancel vs expiry race:** Concurrent Customer cancel + scheduler expiry → exactly one wins.
12. **Confirm vs cancel race:** Concurrent Seller confirm + Customer cancel → exactly one wins.
13. **Expiry retry:** Expiry run on already-expired reservation → no-op (no double decrement).
14. **Cancellation retry:** Cancel on already-cancelled Order → no-op (no double restock).
15. **Seller confirm consumption:** Seller confirm transitions reservation `held → consumed` exactly once.
16. **Seller cancel restock:** Seller cancel after confirmation restocks `on_hand_qty` exactly once.

### 32.4 Historical Immutability Tests
17. **Listing price change:** Changing listing price after order → Order amount unchanged.
18. **Product title change:** Changing product title after order → Order title snapshot unchanged.
19. **SKU code change:** Changing SKU code after order → Order SKU code snapshot unchanged.
20. **Customer address change:** Customer editing address after order → Order address snapshot unchanged.
21. **Supplier wholesale price change:** Supplier changing wholesale price → Customer Order unchanged.
22. **Source record deletion block:** Attempting to delete `SupplierOffer` or `FulfillmentLocation` while referenced by Order lineage → rejected by `ON DELETE RESTRICT`.

### 32.5 Outbox Publisher Lease Tests
23. **Batch lease renewal:** Long-running publish batch renews lease before expiration.
24. **Multi-publisher isolation:** Two worker instances claiming batch concurrently → zero overlapping events claimed.
25. **Stale lease recovery:** Worker crash after claim → lease recovered after `OUTBOX_CLAIM_LEASE_DURATION`.
26. **Stale ACK rejection:** Stale worker acknowledge after lease reassignment fails gracefully.
27. **Broker failure backoff:** Broker failure releases claim and schedules `next_attempt_at` backoff.
28. **Confirm-then-crash duplicate:** Publish confirm crash recovery generates duplicate event handled cleanly by Consumer Inbox.
29. **Consumer Inbox duplicate suppression:** Duplicate `event_id` delivery to same consumer → side-effect executes once.
30. **Multi-consumer event delivery:** Same `event_id` delivered to two different consumer names → each processes once independently.

### 32.6 State Machine Matrix Tests
31. **All Phase 5 enabled transitions:** Table-driven test asserting success, timeline entry, and outbox event.
32. **All Phase 5 rejected transitions:** Table-driven test asserting `invalid_order_transition` error and zero side-effects.

---

## 33. Repository Impact Matrix

| Repository | Impact | Description |
|---|---|---|
| **core** (`matjeroapps/core`) | **High** | Schema migrations; Customer/Cart/Checkout/Order aggregates; Outbox multi-publisher claim worker; Core internal API routes. |
| **seller** (`matjeroapps/seller`) | **High** | `apps/storefront-api` (Cart/Checkout routes); `apps/seller-api` (Order routes); `web/storefront` (Cart/Checkout UI); `web/seller` (Order Management UI). |
| **seller-hub** (`matjeroapps/seller-hub`) | **None** | README-only repository. NO Phase 5 runtime changes. |
| **admin** (`matjeroapps/admin`) | **None** | No Phase 5 runtime changes. |
| **supplier** (`matjeroapps/supplier`) | **None** | No Phase 5 runtime changes. |
| **supplier-hub** (`matjeroapps/supplier-hub`) | **None** | README-only repository. |

---

## 34. Implementation Unit Breakdown

### P5.0 — Phase Specification *(this document)*
**Repository:** core · **Branch:** feature/p5-phase-spec

### P5.1 — Customer + Cart Core Domain
**Repository:** core · **Dependencies:** P5.0 merged
Backend work: Migrations A, B, C; Cart aggregate; Parent Cart row locking on all item mutations; Store/Market composite FKs; NOT NULL expected price fields.

### P5.2 — Checkout Session + Server-Computed Fingerprint
**Repository:** core · **Dependencies:** P5.1 merged
Backend work: Migration D; CheckoutSession aggregate (`open`, `finalized`, `expired`); Fingerprint computation logic.

### P5.3 — Order Aggregate + Sequences + State Machine
**Repository:** core · **Dependencies:** P5.2 merged (requires `checkout_sessions` table)
Backend work: Migration E; `store_order_sequences`; Order aggregate with `confirmation_deadline_at` & operational lineage RESTRICT FKs; Order State Machine.

### P5.4 — Inventory Reservation Lifecycle + Allocation + Expiry
**Repository:** core · **Dependencies:** P5.3 merged (requires `order_items` table)
Backend work: Reservation state machine (`held`, `consumed`, `released`, `expired`); Movement `quantity_delta` rules; Two-stage expiry scheduler job based on `confirmation_deadline_at`.

### P5.5 — Atomic Single-Transaction Checkout
**Repository:** core · **Dependencies:** P5.2 + P5.3 + P5.4 merged
Backend work: Full single-transaction checkout implementation; Correct fingerprint ordering; Multi-item stock lock & reserve; Idempotent replay handling (10 concurrent finalizes → 1 Order).

### P5.6 — Outbox Multi-Publisher Claim & Delivery Reliability
**Repository:** core · **Dependencies:** P5.5 merged
Backend work: Migration F; Bounded claim loop with `FOR UPDATE SKIP LOCKED`, partial index, and per-event lease renewal; Publisher confirms & backoff logic; Domain event emission.

### P5.7 — Storefront API + Storefront Web Cart & Checkout
**Repository:** seller (`apps/storefront-api`, `web/storefront`) · **Dependencies:** P5.6 merged (requires reliable transaction & event foundation)
Backend & Frontend work: Storefront cart & checkout routes; Bearer cart cookie management; Next.js checkout UI.

### P5.8 — Seller API + Seller Web Order Management
**Repository:** seller (`apps/seller-api`, `web/seller`) · **Dependencies:** P5.6 merged (requires full Core order lifecycle, inventory, and outbox foundation)
Backend & Frontend work: Seller Order management routes & React dashboard UI.

### P5.9 — Concurrency & Security Hardening
**Repository:** core + seller · **Dependencies:** P5.7 + P5.8 merged
Execution of full concurrency & security audit test suites.

### P5.10 — Phase Completion
**Repository:** core · **Dependencies:** P5.9 merged
Phase 5 completion report & final verification.

---

## 35. Dependency Graph

```
P5.0 (merged)
  │
  └── P5.1 (Customer + Cart Domain)
        │
        └── P5.2 (Checkout Session + Fingerprint)
              │
              └── P5.3 (Order Aggregate + Sequences)
                    │
                    └── P5.4 (Inventory Lifecycle + Expiry)
                          │
                          └── P5.5 (Atomic Single-Transaction Checkout)
                                │
                                └── P5.6 (Outbox Publisher Lease & Claim)
                                      / \
                                     /   \
                              P5.7          P5.8
                      (Storefront API+Web) (Seller API+Web)
                         [seller repo]     [seller repo]
                                    \     /
                                     \   /
                              P5.9 (Hardening)
                                       │
                              P5.10 (Completion)
```

---

## 36. Self-Review: Architecture Assertions

| Question | Answer | Enforcement |
|---|---|---|
| Is Supplier Offer product lineage correctly validated? | **YES** | Verified via `SupplierOffer → SupplierProduct → product_id == SellerListing.product_id`. |
| Are Store and Market pair mismatches prevented in DB? | **YES** | Composite FK `(store_id, market_code) REFERENCES stores(id, market_code)` on `customers`, `carts`, `orders`. |
| Is confirmation deadline frozen once at checkout? | **YES** | Computed `$deadline` ONCE in tx and stored on `orders.confirmation_deadline_at` & `inventory_reservations.expires_at`. |
| Is reservation expiry locking safe? | **YES** | Stage 1 reads candidate Order IDs; Stage 2 locks Order & snapshots in dedicated transaction. |
| Does outbox worker renew lease during long batches? | **YES** | Per-event renewal check before publish if remaining lease < 10 seconds. |
| Is operational source lineage preserved on Order Items? | **YES** | `supplier_offer_id`, `fulfillment_location_id`, `inventory_reservation_id` use `ON DELETE RESTRICT`. |
| Can a crash during checkout leave Checkout Session stuck FINALIZING? | **NO** | Single PostgreSQL transaction; rollback leaves session OPEN; no persistent FINALIZING or FAILED state in DB. |
| Can two publisher instances routinely claim/publish the same outbox row? | **NO** | Exclusive bounded lease via `publish_claim_id` set within `FOR UPDATE SKIP LOCKED`. |
| Can a stale outbox claim recover? | **YES** | Re-claimable when `publish_claimed_at < (now() - OUTBOX_CLAIM_LEASE_DURATION)`. |
| Can publish-confirm crash cause duplicate delivery? | **YES** | At-least-once behavior; handled by Consumer Inbox `processed_events`. |
| Can Checkout Session and Order FK references disagree? | **NO** | Single directional `orders.checkout_session_id` UNIQUE FK. |
| Can Reservation and Order Item FK references disagree? | **NO** | Single directional `order_items.inventory_reservation_id` UNIQUE FK. |
| Can Store A Customer be persisted into Store B Cart/Order? | **NO** | Database composite FKs enforce `(id, store_id)` matching across all aggregates. |
| Can a client choose its own idempotency fingerprint? | **NO** | Core computes `finalize_fingerprint` server-side after loading cart lines. |
| Can Cart mutate while checkout reads it? | **NO** | Parent Cart row is locked (`FOR UPDATE`) during finalize and during all mutations. |
| Can P5.3 run before checkout_sessions exists? | **NO** | P5.3 strictly depends on P5.2. |
| Can P5.8 be deployed before inventory lifecycle exists? | **NO** | P5.8 strictly depends on P5.6 merged. |
| Can an implementation agent invent the order_number algorithm? | **NO** | Atomic allocation via `store_order_sequences` table is strictly specified. |
| Can Phase 5 accidentally add customer OIDC without a Customer IAM decision? | **NO** | Guest checkout is the operational target; Customer IAM integration is explicitly DEFERRED. |
| Are inventory_movement quantity_deltas consistent? | **YES** | `quantity_delta` represents `on_hand_qty` delta (0 for held/released/expired; -qty for consumed; +qty for restock). |
