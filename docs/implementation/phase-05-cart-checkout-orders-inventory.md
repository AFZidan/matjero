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
  SELECT checkout_session FOR UPDATE (check status: finalized → return order; open → proceed)
  → Lock Cart parent row FOR UPDATE (assert active)
  → Lock candidate Inventory Snapshots FOR UPDATE (ordered deterministically by id ASC)
  → Validate Store → Validate Market → Validate Listings → Revalidate Prices
  → Reserve Inventory (insert held reservation + record movement)
  → Allocate Store Order Number (from store_order_sequences)
  → Create Order → Create Order Items (linking reservation)
  → Create Order Address → Create Order Timeline entry
  → Create Outbox Event (commerce.order.created)
  → Update Checkout Session (status = finalized, set server-computed fingerprint)
  → Update Cart (status = checked_out)
COMMIT
```

All steps commit or roll back together. Any validation or stock failure causes an
immediate transaction rollback.

---

## 2. Goals

- Implement the `customers`, `customer_addresses`, `carts`, `cart_items`,
  `checkout_sessions`, `store_order_sequences`, `orders`, `order_items`,
  `order_addresses`, `order_timeline`, and `order_notes` aggregates.
- Extend `fulfillment_locations` to support Seller-owned locations (Store-scoped
  inventory).
- Define a strictly consistent, oversell-proof atomic single-transaction checkout.
- Activate the Transactional Outbox (ADR-006) with a multi-publisher lease/claim design.
- Define the Consumer Inbox (ADR-007) semantics for Phase 5 consumers.
- Establish the Order State Machine with explicit transition authority.
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
| Cart identity | Identified by a high-entropy bearer token digest (`cart_token_digest`). Token is held in an HttpOnly cookie. |
| Cart locking | Parent `carts` row is locked (`FOR UPDATE`) during all Cart item mutations (add, update, remove, merge) and during checkout finalization. Mutations on `checked_out` carts fail deterministically. |
| Single-transaction checkout | No two-phase `finalizing` fence. Checkout completes in ONE PostgreSQL transaction. Concurrent requests block on `checkout_sessions` row lock and return the committed Order upon waking. |
| Idempotency fingerprint | Core computes `finalize_fingerprint` server-side from canonical deterministic serialization of checkout inputs while holding locks. Clients NEVER submit a trusted fingerprint. |
| Non-circular FKs | `orders.checkout_session_id` FK → `checkout_sessions(id)` (UNIQUE, NOT NULL). `order_items.inventory_reservation_id` FK → `inventory_reservations(id)` (UNIQUE, NOT NULL). No reciprocal columns. |
| Tenant composite FKs | Database enforces Store isolation via composite UNIQUE keys on `(id, store_id)` for `customers`, `carts`, `checkout_sessions` and matching composite FKs across relationships. |
| Order sequence | Monotonic per-Store order numbers generated via atomic updates on a dedicated `store_order_sequences` table. Formatted as stable Store-local strings (e.g. `#100001`). |
| Inventory movement delta | `quantity_delta` in `inventory_movements` strictly represents `on_hand_qty` delta. Reservation events (`held`, `released`, `expired`) write `quantity_delta = 0`. Confirmation (`consumed`) writes `quantity_delta = -quantity`. Restock writes `quantity_delta = +quantity`. |
| Outbox lease claims | Multi-publisher outbox claims use explicit columns (`publish_claim_id`, `publish_claimed_at`, `publish_attempts`, `next_attempt_at`). `FOR UPDATE SKIP LOCKED` claims bounded batches with stale lease recovery. |
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
                └───────────────────────────────┘

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
                └───────────────────────────────┘
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
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `market_code` | `CHAR(2)` | NOT NULL FK → `markets(code)` |
| `identity_provider` | `TEXT` | Nullable |
| `identity_subject` | `TEXT` | Nullable |
| `email` | `TEXT` | Nullable |
| `display_name` | `TEXT` | Nullable |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('active', 'blocked')) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key for composite FKs:** `UNIQUE (id, store_id)`.
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
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `market_code` | `CHAR(2)` | NOT NULL FK → `markets(code)` |
| `customer_id` | `UUID` | Nullable |
| `cart_token_digest` | `TEXT` | NOT NULL UNIQUE (SHA-256) |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('active', 'checked_out', 'abandoned', 'expired')) |
| `expires_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Composite FK:** `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE SET NULL`.
**Partial Index:** `UNIQUE (customer_id, store_id) WHERE status = 'active' AND customer_id IS NOT NULL`.

#### `cart_items` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `cart_id` | `UUID` | NOT NULL FK → `carts(id)` ON DELETE CASCADE |
| `seller_listing_id` | `UUID` | NOT NULL FK → `seller_listings(id)` ON DELETE RESTRICT |
| `sku_id` | `UUID` | NOT NULL FK → `skus(id)` ON DELETE RESTRICT |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0 AND quantity <= 10000) |
| `expected_unit_price_minor` | `BIGINT` | Nullable (display snapshot captured at add-to-cart) |
| `expected_currency_code` | `CHAR(3)` | Nullable FK → `currencies(code)` |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique Constraint:** `UNIQUE (cart_id, seller_listing_id, sku_id)`.

---

## 9. SKU / Listing / Source Semantics

Checkout validates that every Cart Item maintains strict lineage:
1. `seller_listing.status == 'active'`
2. `seller_listing.store_id == cart.store_id`
3. `seller_listing.market_code == store.market_code`
4. `sku → variant → product_id == seller_listing.product_id`
5. If Supplier-sourced (`supplier_offer_id NOT NULL`): `supplier_offer.status == 'active'`, `supplier_offer.market_code == store.market_code`, `supplier_offer.product_id == seller_listing.product_id`, and selected location is owned by the Supplier.
6. If Seller-owned (`supplier_offer_id IS NULL`): selected location is owned by the Store (`store_id == cart.store_id`).

Public Storefront DTOs **never** expose internal supplier data (`supplier_id`, `supplier_offer_id`, `fulfillment_location_id`, wholesale costs, or reservation tokens).

---

## 10. Checkout Session Aggregate

### 10.1 Blueprint & State Machine

A Checkout Session represents intent to finalize an order. It has four allowed status states:
- `open`: Initial active checkout session.
- `finalized`: Transaction completed successfully; linked Order created.
- `expired`: Confirmation window elapsed without finalization.
- `failed`: Validation error encountered (e.g. unrecoverable catalog error).

No persistent `finalizing` status is stored in the database.

#### `checkout_sessions` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `cart_id` | `UUID` | NOT NULL |
| `customer_id` | `UUID` | Nullable |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('open', 'finalized', 'expired', 'failed')) |
| `expires_at` | `TIMESTAMPTZ` | NOT NULL |
| `shipping_address_snapshot` | `JSONB` | Nullable (address snapshot) |
| `contact_email` | `TEXT` | Nullable |
| `finalize_fingerprint` | `TEXT` | Nullable (server-computed deterministic SHA-256 fingerprint) |
| `finalized_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Composite FKs:**
- `FOREIGN KEY (cart_id, store_id) REFERENCES carts(id, store_id) ON DELETE RESTRICT`
- `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE SET NULL`

*Note:* `checkout_sessions.order_id` is removed to prevent circular FKs. Lineage is tracked solely via `orders.checkout_session_id`.

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

While holding row locks on `checkout_sessions` and `carts`, Core computes `finalize_fingerprint` as a SHA-256 digest of a canonical deterministic JSON string containing:
- `checkout_session_id`
- `cart_id`
- Sorted list of Cart line items: `(seller_listing_id, sku_id, quantity)`
- Sorted list of expected prices: `(unit_price_minor, currency_code)`
- Address snapshot & contact details
- Customer context (`customer_id` if present)

### 12.2 Single-Transaction Flow

Checkout is executed in **ONE PostgreSQL transaction**:

```sql
BEGIN;

-- 1. Lock Checkout Session row
SELECT * FROM checkout_sessions WHERE id = $session_id FOR UPDATE;

-- Check status
IF status = 'finalized' THEN
  -- Check fingerprint match
  IF finalize_fingerprint = $computed_fingerprint THEN
    SELECT * FROM orders WHERE checkout_session_id = $session_id;
    COMMIT;
    RETURN existing_order;
  ELSE
    ROLLBACK;
    RETURN idempotency_conflict;
  END IF;
ELSIF status = 'expired' THEN
  ROLLBACK;
  RETURN checkout_expired;
ELSIF status = 'failed' THEN
  ROLLBACK;
  RETURN checkout_failed;
END IF;

-- 2. Lock Parent Cart Row
SELECT * FROM carts WHERE id = $cart_id FOR UPDATE;
ASSERT carts.status = 'active';

-- 3. Lock candidate Inventory Snapshots in deterministic order
SELECT * FROM inventory_snapshots
WHERE id = ANY($snapshot_ids)
ORDER BY id ASC
FOR UPDATE;

-- 4. Revalidate Store, Listings, SKUs, and Prices (price_changed check)
-- 5. Allocate Inventory (update inventory_snapshots, insert inventory_reservations with status='held')
-- 6. Allocate Order Number from store_order_sequences
-- 7. Insert Order (referencing checkout_session_id)
-- 8. Insert Order Items (referencing inventory_reservation_id)
-- 9. Insert Order Address
-- 10. Insert Order Timeline (to_status='pending', actor_type='checkout')
-- 11. Insert Outbox Event (commerce.order.created)
-- 12. Update Checkout Session: status = 'finalized', finalize_fingerprint = $computed_fingerprint, finalized_at = now()
-- 13. Update Cart: status = 'checked_out'

COMMIT;
```

### 12.3 Concurrency Behavior & Failure Scenarios

- **Concurrent finalization requests:** The second request blocks on `SELECT ... FOR UPDATE` on `checkout_sessions`. When the first transaction commits, the second request wakes, reads `status = 'finalized'`, verifies `finalize_fingerprint`, and returns the existing Order.
- **Crash before COMMIT:** Entire transaction rolls back; session remains `open`; cart remains `active`.
- **Crash after COMMIT before HTTP response:** Retried request hits step 1, sees `status = 'finalized'`, and returns the existing Order.

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

- `held`: Active reservation created at checkout. Holds stock (`reserved_qty += quantity`).
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
4. `orders` row (for status transitions)
5. `store_order_sequences` row (during order creation)

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

Composite FK for Seller branch: `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code)`.

---

## 17. Order Aggregate & Non-Circular FK Blueprints

#### `orders` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_number` | `TEXT` | NOT NULL |
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `market_code` | `CHAR(2)` | NOT NULL FK → `markets(code)` |
| `customer_id` | `UUID` | Nullable |
| `checkout_session_id` | `UUID` | NOT NULL UNIQUE FK → `checkout_sessions(id)` ON DELETE RESTRICT |
| `status` | `TEXT` | NOT NULL CHECK (status IN ('pending', 'confirmed', 'processing', 'ready_for_shipping', 'shipped', 'out_for_delivery', 'delivered', 'cancelled', 'returned')) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `subtotal_minor` | `BIGINT` | NOT NULL CHECK (subtotal_minor >= 0) |
| `total_minor` | `BIGINT` | NOT NULL CHECK (total_minor >= 0) |
| `cancellation_reason` | `TEXT` | Nullable |
| `aggregate_version` | `BIGINT` | NOT NULL DEFAULT 1 |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique Constraints:** `UNIQUE (store_id, order_number)`, `UNIQUE (checkout_session_id)`.
**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Composite FKs:**
- `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE SET NULL`
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
| `supplier_offer_id` | `UUID` | Nullable FK → `supplier_offers(id)` ON DELETE SET NULL |
| `fulfillment_location_id` | `UUID` | Nullable FK → `fulfillment_locations(id)` ON DELETE SET NULL |
| `inventory_reservation_id` | `UUID` | NOT NULL UNIQUE FK → `inventory_reservations(id)` ON DELETE RESTRICT |
| `product_title_snapshot` | `TEXT` | NOT NULL |
| `sku_code_snapshot` | `TEXT` | NOT NULL |
| `unit_price_minor` | `BIGINT` | NOT NULL CHECK (unit_price_minor >= 0) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0) |
| `line_total_minor` | `BIGINT` | NOT NULL CHECK (line_total_minor >= 0) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

*Note:* `inventory_reservations` does NOT contain an `order_item_id` column. The relationship is strictly unidirectional via `order_items.inventory_reservation_id`.

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
| `pending` | `cancelled` | Scheduler | Confirmation deadline elapsed | `expired` (reserved -= qty) | `commerce.order.status_changed` |
| `confirmed` | `processing` | Seller | — | None | `commerce.order.status_changed` |
| `confirmed` | `cancelled` | Seller | — | Restock: `on_hand += qty` (movement `order_cancellation_restock`) | `commerce.order.status_changed` |
| `processing` | `ready_for_shipping` | Seller | — | None | `commerce.order.status_changed` |
| `processing` | `cancelled` | Seller | — | Restock: `on_hand += qty` | `commerce.order.status_changed` |

Transitions from `ready_for_shipping` onward (`shipped`, `out_for_delivery`, `delivered`) belong to Phase 6. `returned` belongs to a future returns phase.

---

## 19. Reservation Expiry Workflow

When a `PENDING` Order's confirmation deadline elapses, a background scheduler job queries `orders WHERE status = 'pending' AND created_at < cutoff` in bounded batches.

For each expired Order, in a dedicated PostgreSQL transaction:
1. `SELECT * FROM orders WHERE id = $1 AND status = 'pending' FOR UPDATE`
2. `UPDATE orders SET status = 'cancelled', cancellation_reason = 'confirmation_timeout', aggregate_version = aggregate_version + 1`
3. Update linked reservations: `status = 'expired'`, `reserved_qty -= quantity` on inventory snapshots.
4. Record `inventory_movements` (`movement_type = 'reservation_expired'`, `quantity_delta = 0`).
5. Insert `order_timeline` (`actor_type = 'scheduler'`, `reason = 'confirmation_timeout'`).
6. Insert `outbox_events` (`commerce.order.status_changed`).

---

## 20. Outbox Multi-Publisher Lease & Claim Design

### 20.1 Schema Extension to `outbox_events`

Migration adds the following claim columns to `outbox_events`:

| Column | Type | Default / Nullability |
|---|---|---|
| `publish_claim_id` | `UUID` | NULL |
| `publish_claimed_at` | `TIMESTAMPTZ` | NULL |
| `publish_attempts` | `INTEGER` | NOT NULL DEFAULT 0 |
| `next_attempt_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

### 20.2 Bounded Claim Algorithm

Worker instances execute the claim loop:

```sql
BEGIN;

SELECT event_id FROM outbox_events
WHERE published_at IS NULL
  AND next_attempt_at <= now()
  AND (
    publish_claim_id IS NULL
    OR publish_claimed_at < (now() - interval '30 seconds') -- Stale lease cutoff
  )
ORDER BY created_at ASC, event_id ASC
LIMIT $batch_size
FOR UPDATE SKIP LOCKED;

UPDATE outbox_events
SET publish_claim_id = $worker_claim_uuid,
    publish_claimed_at = now()
WHERE event_id = ANY($claimed_ids);

COMMIT;
```

### 20.3 Publishing & Confirmation

Events are published to RabbitMQ **outside** the PostgreSQL transaction.

On RabbitMQ Publisher Confirm:
```sql
UPDATE outbox_events
SET published_at = now(),
    publish_claim_id = NULL,
    publish_claimed_at = NULL
WHERE event_id = $1 AND publish_claim_id = $worker_claim_uuid;
```

On Broker Failure:
```sql
UPDATE outbox_events
SET publish_attempts = publish_attempts + 1,
    publish_claim_id = NULL,
    publish_claimed_at = NULL,
    next_attempt_at = now() + (interval '1 second' * power(2, LEAST(publish_attempts, 6)))
WHERE event_id = $1 AND publish_claim_id = $worker_claim_uuid;
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
- Tenant Isolation: DB composite UNIQUE keys and FKs enforce Store boundary.
- Price Tampering Defense: Core re-reads authoritative listing prices inside checkout transaction.
- Internal Data Sanitation: Public DTOs omit internal IDs and costs.

---

## 26. Database Migration Plan

Phase 5 proposes migrations to `core`:
- Migration A: `fulfillment_locations` ownership extension (support Seller-owned locations).
- Migration B: `customers` and `customer_addresses` tables.
- Migration C: `carts` and `cart_items` tables.
- Migration D: `checkout_sessions` table (with `finalize_fingerprint`).
- Migration E: `store_order_sequences`, `orders`, `order_items`, `order_addresses`, `order_timeline`, `order_notes` tables.
- Migration F: `outbox_events` claim extension columns.

---

## 27. Testing Strategy

Must include deterministic unit and integration tests:

### 27.1 Concurrency & Race Tests
1. **Concurrent Cart mutation vs Finalize:** Verify parent Cart locking prevents race conditions, stale items, or post-checkout mutations.
2. **Last-unit stock race:** 20 concurrent finalizations for 1 available stock unit → exactly 1 succeeds.
3. **Multi-item rollback:** Item stock failure causes full rollback; zero reservations persist.
4. **Order State Machine transitions:** Table-driven validation of allowed vs forbidden transitions.

### 27.2 Outbox Publisher Lease Tests
5. **Concurrent publisher claims:** Two worker instances claiming batch concurrently → zero overlapping events claimed.
6. **Stale lease recovery:** Worker crash after claim → lease recovered after cutoff.
7. **Publish confirm:** `published_at` set only by matching claim owner.
8. **Stale ACK attempt:** Stale worker acknowledge after lease reassignment fails gracefully.
9. **Broker failure:** Claim released and `next_attempt_at` backoff scheduled.
10. **At-least-once duplicate delivery:** Publish confirm crash recovery generates duplicate event handled cleanly by Consumer Inbox.

---

## 28. Repository Impact Matrix

| Repository | Impact | Description |
|---|---|---|
| **core** (`matjeroapps/core`) | **High** | Schema migrations; Customer/Cart/Checkout/Order aggregates; Outbox multi-publisher claim worker; Core internal API routes. |
| **seller** (`matjeroapps/seller`) | **High** | `apps/storefront-api` (Cart/Checkout routes); `apps/seller-api` (Order routes); `web/storefront` (Cart/Checkout UI); `web/seller` (Order Management UI). |
| **seller-hub** (`matjeroapps/seller-hub`) | **None** | README-only repository. NO Phase 5 runtime changes. |
| **admin** (`matjeroapps/admin`) | **None** | No Phase 5 runtime changes. |
| **supplier** (`matjeroapps/supplier`) | **None** | No Phase 5 runtime changes. |
| **supplier-hub** (`matjeroapps/supplier-hub`) | **None** | README-only repository. |

---

## 29. Implementation Unit Breakdown

### P5.0 — Phase Specification *(this document)*
**Repository:** core · **Branch:** feature/p5-phase-spec

### P5.1 — Customer + Cart Core Domain
**Repository:** core · **Dependencies:** P5.0 merged
Backend work: Migrations A, B, C; Cart aggregate; Parent Cart row locking on all item mutations.

### P5.2 — Checkout Session + Server-Computed Fingerprint
**Repository:** core · **Dependencies:** P5.1 merged
Backend work: Migration D; CheckoutSession aggregate; `finalize_fingerprint` calculation; session locking & expiry.

### P5.3 — Order Aggregate + Sequences + State Machine
**Repository:** core · **Dependencies:** P5.2 merged (requires `checkout_sessions` table)
Backend work: Migration E; `store_order_sequences`; Order aggregate & non-circular FKs; Order State Machine.

### P5.4 — Inventory Reservation Lifecycle + Allocation + Expiry
**Repository:** core · **Dependencies:** P5.3 merged (requires `order_items` table)
Backend work: Reservation state machine (`held`, `consumed`, `released`, `expired`); Movement `quantity_delta` rules; Expiry scheduler job.

### P5.5 — Atomic Single-Transaction Checkout
**Repository:** core · **Dependencies:** P5.2 + P5.3 + P5.4 merged
Backend work: Full single-transaction checkout implementation; Price revalidation; Multi-item stock lock & reserve; Idempotent replay handling (10 concurrent finalizes → 1 Order).

### P5.6 — Outbox Multi-Publisher Claim & Delivery Reliability
**Repository:** core · **Dependencies:** P5.5 merged
Backend work: Migration F; Bounded claim loop with `FOR UPDATE SKIP LOCKED`; Publisher confirms & backoff logic; Domain event emission.

### P5.7 — Storefront API + Storefront Web Cart & Checkout
**Repository:** seller (`apps/storefront-api`, `web/storefront`) · **Dependencies:** P5.5 merged
Backend & Frontend work: Storefront cart & checkout routes; Bearer cart cookie management; Next.js checkout UI.

### P5.8 — Seller API + Seller Web Order Management
**Repository:** seller (`apps/seller-api`, `web/seller`) · **Dependencies:** P5.3 merged
Backend & Frontend work: Seller Order management routes & React dashboard UI.

### P5.9 — Concurrency & Security Hardening
**Repository:** core + seller · **Dependencies:** P5.7 + P5.8 merged
Execution of full concurrency & security audit test suites.

### P5.10 — Phase Completion
**Repository:** core · **Dependencies:** P5.9 merged
Phase 5 completion report & final verification.

---

## 30. Dependency Graph

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

## 31. Self-Review: Architecture Assertions

| Question | Answer | Enforcement |
|---|---|---|
| Can a crash during checkout leave Checkout Session stuck FINALIZING? | **NO** | Single PostgreSQL transaction; rollback leaves session OPEN; no persistent FINALIZING state. |
| Can two publisher instances routinely claim/publish the same outbox row? | **NO** | Exclusive bounded lease via `publish_claim_id` set within `FOR UPDATE SKIP LOCKED`. |
| Can a stale outbox claim recover? | **YES** | Re-claimable when `publish_claimed_at < cutoff`. |
| Can publish-confirm crash cause duplicate delivery? | **YES** | At-least-once behavior; handled by Consumer Inbox `processed_events`. |
| Can Checkout Session and Order FK references disagree? | **NO** | Single directional `orders.checkout_session_id` UNIQUE FK. |
| Can Reservation and Order Item FK references disagree? | **NO** | Single directional `order_items.inventory_reservation_id` UNIQUE FK. |
| Can Store A Customer be persisted into Store B Cart/Order? | **NO** | Database composite FKs enforce `(id, store_id)` matching across all aggregates. |
| Can a client choose its own idempotency fingerprint? | **NO** | Core computes `finalize_fingerprint` server-side while holding locks. |
| Can Cart mutate while checkout reads it? | **NO** | Parent Cart row is locked (`FOR UPDATE`) during finalize and during all mutations. |
| Can P5.3 run before checkout_sessions exists? | **NO** | P5.3 strictly depends on P5.2. |
| Can an implementation agent invent the order_number algorithm? | **NO** | Atomic allocation via `store_order_sequences` table is strictly specified. |
| Can Phase 5 accidentally add customer OIDC without a Customer IAM decision? | **NO** | Guest checkout is the operational target; Customer IAM integration is explicitly DEFERRED. |
| Are inventory_movement quantity_deltas consistent? | **YES** | `quantity_delta` represents `on_hand_qty` delta (0 for held/released/expired; -qty for consumed; +qty for restock). |
