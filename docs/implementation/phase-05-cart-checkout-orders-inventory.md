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
     - Freeze one transaction timestamp: $transaction_now
     - If status = 'expired': ROLLBACK → return checkout_expired
     - If status = 'open' AND expires_at <= $transaction_now:
       ROLLBACK → return checkout_expired
     - Only status = 'open' AND expires_at > $transaction_now may finalize

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
     - Validate the complete guest checkout payload before creating any Order or reservation
     - Revalidate Store, Listings, SKUs, and Prices (compare amount and currency → price_changed or market_mismatch)
     - Validate Supplier Offer → Supplier Product → Product lineage (for Supplier-sourced listings)
     - Reserve Inventory (insert held reservation with expires_at = $deadline + record movement)
     - Allocate Store Order Number (from store_order_sequences)
     - Insert Order (with confirmation_deadline_at = $deadline)
     - Insert Order Items (referencing inventory_reservation_id, fulfillment_location_id, supplier_offer_id)
     - Insert Order Address & Order Timeline entry
     - Insert Outbox Event (commerce.order.created)
     - Generate a high-entropy guest Order access token; persist only its digest
     - Update Checkout Session (status = 'finalized', finalize_fingerprint = $computed_fingerprint, finalized_at = $transaction_now)
     - Update Cart (status = 'checked_out')
COMMIT
```

All steps commit or roll back together. Any validation failure (e.g. `price_changed`, `insufficient_inventory`, `checkout_expired`, or money overflow) causes a transaction rollback. A timestamp-expired `open` session returns `checkout_expired` and creates no reservation, Order, timeline, or outbox event. A finalized session is the sole replay exception: it may return its already-created Order after `expires_at`, but it cannot create another Order or side effect.

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
| Price change handling | Authoritative price amount and currency re-read inside checkout transaction. Amount or currency mismatch with Cart snapshot rejects with `price_changed`; listing currency inconsistent with Store Market rejects with `market_mismatch` / invariant failure according to the current error contract. |
| Order currency | `orders.currency_code` is exactly `Store Market.currency_code`; every Order Item uses the same currency. Cross-currency arithmetic is forbidden. |
| Checkout expiry authority | `checkout_sessions.expires_at` is authoritative. Finalization requires `status = 'open' AND expires_at > $transaction_now`; cleanup may persist `expired` but correctness does not depend on cleanup. Finalized replay returns the existing Order even after expiry. |
| Money arithmetic | Monetary values are signed 64-bit minor units. Each multiplication and subtotal addition uses checked arithmetic; overflow returns `validation_error` and rolls back the entire checkout. |
| Guest Order access | Successful guest checkout creates a dedicated high-entropy bearer capability. Core persists only `guest_order_access_token_digest`; Storefront operations require both the resolved Store and the capability. |

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

## 11. Price and Currency Validation & Calculation

1. Inside the atomic checkout transaction, Core re-reads current `seller_listing_prices` (`amount_minor`, `currency_code`).
2. For every Cart Item, the current Seller Listing Price amount MUST equal `cart_item.expected_unit_price_minor`.
3. For every Cart Item, the current Seller Listing Price currency MUST equal `cart_item.expected_currency_code`.
4. For every Cart Item, the current Seller Listing Price currency MUST equal `Store Market.currency_code`.
5. If the expected amount OR expected currency changed, Core rolls back and returns `price_changed` with the current authoritative amount and currency.
6. If the current Seller Listing Price currency differs from the Store Market currency, checkout is rejected as `market_mismatch` / invariant failure according to the current error contract.
7. `orders.currency_code` is set to `Store Market.currency_code`. Every `order_items.currency_code` MUST equal `orders.currency_code`; no mixed-currency Order can commit.
8. Never perform cross-currency arithmetic. Order totals are calculated deterministically with signed 64-bit checked operations:
   ```
   line_total_minor = checkedMultiply(unit_price_minor, quantity)
   subtotal_minor   = checkedAdd(previous_subtotal_minor, line_total_minor)
   total_minor      = subtotal_minor
   ```
   Before persistence, require `unit_price_minor >= 0`, `quantity > 0`,
   `line_total_minor >= 0`, `subtotal_minor >= 0`, and `total_minor >= 0`.
   If multiplication or addition exceeds the supported `int64` range, return
   `validation_error`; never allow a wrapped value or PostgreSQL `BIGINT`
   overflow to surface as `internal_error`.
9. Client-submitted price totals are NEVER trusted.

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

-- 2. Freeze one transaction timestamp and enforce timestamp expiry
SELECT transaction_timestamp() AS transaction_now;

IF status = 'expired'
   OR (status = 'open' AND expires_at <= $transaction_now) THEN
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

-- 7. If status = 'open' AND expires_at > $transaction_now: proceed
--    a. Freeze single confirmation deadline: $deadline = $transaction_now + configured_confirmation_duration
--    b. Validate Store status = 'active' & market match
--    c. Validate the complete guest checkout payload (see section 12.4)
--    d. Validate Listings, SKUs, and reread listing prices (amount and currency)
--    e. Validate Seller Listing currency == Store Market currency
--    f. Validate Supplier Offer → Supplier Product → Product lineage (for Supplier listings)
--    g. Lock candidate inventory_snapshots (ordered by id ASC)
--    h. Reserve inventory (insert inventory_reservations status='held', expires_at = $deadline)
--    i. Allocate Order Number from store_order_sequences
--    j. Generate raw guest capability in memory; persist only its digest
--    k. Insert Order (currency_code = Store Market.currency_code, confirmation_deadline_at = $deadline, guest_order_access_token_digest = digest)
--    l. Insert Order Items (currency_code = orders.currency_code, referencing inventory_reservation_id, fulfillment_location_id, supplier_offer_id)
--    m. Insert Order Address & Order Timeline (to_status='pending', actor_type='checkout')
--    n. Insert Outbox Event (commerce.order.created)
--    o. Update Checkout Session: status = 'finalized', finalize_fingerprint = $computed_fingerprint, finalized_at = $transaction_now
--    p. Update Cart: status = 'checked_out'

COMMIT;
```

### 12.3 Concurrency Behavior & Failure Scenarios

- **Concurrent finalization requests:** The second request blocks on `SELECT ... FOR UPDATE` on `checkout_sessions`. When the first transaction commits, the second request wakes, loads cart lines, computes fingerprint, reads `status = 'finalized'`, verifies fingerprint match, and returns the existing Order.
- **Crash before COMMIT:** Entire transaction rolls back; session remains `open`; cart remains `active`.
- **Crash after COMMIT before HTTP response:** Retried request hits step 6, sees `status = 'finalized'`, verifies fingerprint, and returns the existing Order.
- **Validation failures:** Validation errors (e.g. `price_changed`, `insufficient_inventory`) cause ROLLBACK, leaving `checkout_sessions` in `open` state so the Customer may retry.
- **Timestamp expiry:** An `open` session is finalizable only when `expires_at > $transaction_now`. A stale `open` row cannot finalize even if lifecycle cleanup has not changed its status. The transaction creates no reservation, Order, timeline, or outbox event on expiry.
- **Finalized replay after expiry:** A `finalized` session may replay after its original `expires_at`; after fingerprint verification it returns the same existing Order only. It never creates a new Order, capability, reservation, timeline, or outbox event.
- **Money overflow:** Checked multiplication/addition returns `validation_error`; the transaction rolls back and leaves zero Order, reservation, and outbox rows.

### 12.4 Guest Finalization Payload Validation

`checkout_sessions` may remain incomplete while `status = 'open'`; therefore
`shipping_address_snapshot` and `contact_email` are nullable during session
editing. Finalization MUST validate the complete guest payload before any
inventory reservation or Order creation.

At minimum, the payload must contain the fields already required by
`order_addresses`: `recipient_name`, `address_line_1`, `city`, and
`country_code`, plus `contact_email`. Any other field required by
`order_addresses` is validated at the same boundary. Missing required data
returns `validation_error`; it must not surface as `internal_error` or a raw
database constraint error. The immutable Order Address is created only after
validation succeeds.

If this validation fails, the transaction rolls back: the session remains
`open`, and no reservation, Order, or Outbox row survives.

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

Commerce operations use two explicit lock families; there is no universal lock
order for every operation.

### 15.1 Checkout Creation Lock Family

Checkout creation acquires locks in this order:

1. `checkout_sessions` row (by `id`)
2. `carts` parent row (by `id`)
3. `inventory_snapshots` rows, ordered by `id ASC`
4. `store_order_sequences` row

The Order row is created after these locks and is not a pre-existing row that
must be locked.

### 15.2 Existing Order Transition Lock Family

Seller confirm, Seller cancel, Customer PENDING cancel, and Scheduler expiry
MUST all acquire locks in this same order:

1. `orders` row `FOR UPDATE`
2. linked `inventory_snapshots` rows `FOR UPDATE`, ordered by `id ASC`
3. linked `inventory_reservations` rows `FOR UPDATE`, ordered by `id ASC`

Only after these locks are acquired does the operation perform its guarded
state mutation and exactly one terminal inventory effect. This shared order is
mandatory to prevent confirm/cancel/expiry deadlocks.

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

Existing Supplier-owned location uniqueness remains:

```sql
UNIQUE (supplier_market_id, code)
```

Seller/Store-owned branches use a partial unique index:

```sql
CREATE UNIQUE INDEX fulfillment_locations_store_code_uidx
ON fulfillment_locations (store_id, code)
WHERE store_id IS NOT NULL;
```

The composite FK `(store_id, market_code) REFERENCES stores(id, market_code)`
remains mandatory, as does the exclusive ownership CHECK above. A location
code is unique within its owning Supplier Market or Store.

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
| `guest_order_access_token_digest` | `BYTEA` | Nullable only for deferred non-guest flows; required for Phase 5 guest Orders; SHA-256 digest of the dedicated high-entropy guest Order access capability |
| `subtotal_minor` | `BIGINT` | NOT NULL CHECK (subtotal_minor >= 0) |
| `total_minor` | `BIGINT` | NOT NULL CHECK (total_minor >= 0) |
| `confirmation_deadline_at` | `TIMESTAMPTZ` | NOT NULL (frozen Seller confirmation deadline) |
| `cancellation_reason` | `TEXT` | Nullable |
| `aggregate_version` | `BIGINT` | NOT NULL DEFAULT 1 |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique Constraints:** `UNIQUE (store_id, order_number)`, `UNIQUE (checkout_session_id)`.
**Supporting UNIQUE key:** `UNIQUE (id, store_id)`.
**Currency relation key:** `UNIQUE (id, currency_code)`.
**Index:** `CREATE INDEX orders_status_deadline_idx ON orders (status, confirmation_deadline_at);`.
**Store/Market Composite FK:** `FOREIGN KEY (store_id, market_code) REFERENCES stores(id, market_code) ON DELETE RESTRICT`.
**Composite FKs:**
- `FOREIGN KEY (customer_id, store_id) REFERENCES customers(id, store_id) ON DELETE RESTRICT`
- `FOREIGN KEY (checkout_session_id, store_id) REFERENCES checkout_sessions(id, store_id) ON DELETE RESTRICT`

#### `order_items` Table

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL; part of composite FK → `orders(id, currency_code)` ON DELETE RESTRICT |
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

**Currency relation:** `FOREIGN KEY (order_id, currency_code) REFERENCES orders(id, currency_code) ON DELETE RESTRICT`. The independent `currency_code → currencies(code)` FK may remain, but PostgreSQL rejects an Order Item whose currency differs from its parent Order.

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
`OUTBOX_CLAIM_RENEWAL_MARGIN` is a separate configuration and MUST satisfy:

```
0 < OUTBOX_CLAIM_RENEWAL_MARGIN
  < OUTBOX_CLAIM_LEASE_DURATION
```

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
   Define the lease timestamps explicitly:
   ```
   lease_expires_at = publish_claimed_at + OUTBOX_CLAIM_LEASE_DURATION
   remaining_lease  = lease_expires_at - now()
   ```
   `now() - publish_claimed_at` is elapsed lease age, not remaining lease
   duration. Before publishing each event in the claimed batch:
   - If `remaining_lease <= OUTBOX_CLAIM_RENEWAL_MARGIN`, atomically renew:
     ```sql
     UPDATE outbox_events
     SET publish_claimed_at = now()
     WHERE event_id = $event_id
       AND publish_claim_id = $current_claim
       AND published_at IS NULL
       AND publish_claimed_at >=
           now() - OUTBOX_CLAIM_LEASE_DURATION;
     ```
   - If renewal update affects 0 rows, **DO NOT publish** that event.

3. **Publisher Confirm Acknowledge:**
```sql
UPDATE outbox_events
SET published_at = now(),
    publish_claim_id = NULL,
    publish_claimed_at = NULL
WHERE event_id = $1
  AND publish_claim_id = $current_claim
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
  AND publish_claim_id = $current_claim;
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

## 23. Event Ordering

Duplicate suppression and event ordering are distinct concerns:

- Consumer Inbox identity is `(consumer_name, event_id)`.
- Event ordering metadata is `(aggregate_id, aggregate_version)`.
- Every `EventEnvelope` includes `aggregate_type`, `aggregate_id`, and
  `aggregate_version`.
- For Order events, `aggregate_version` starts at `1` and increments
  transactionally on every meaningful Order mutation that emits an Order event.

`processed_events` does not enforce aggregate ordering. In this initial Phase 5
rule, a consumer that does not require an ordered projection may process an
event normally after Inbox duplicate suppression. A consumer that requires an
ordered projection owns durable `last_applied_aggregate_version` state per
aggregate. Older or already-applied versions are ignored safely. A future
version with a gap must not silently advance the projection; that consumer must
retry/defer the event or resync the aggregate according to its durable policy.

Timestamps are not authoritative ordering. Phase 5 does not claim global
exactly-once delivery or global causal ordering, and no generic ordering table
is required until a real consumer requires one.

---

## 24. API Boundaries

- `storefront-api` (in `matjeroapps/seller`): Public anonymous routes `/v1/storefront/carts`, `/v1/storefront/checkout/sessions`, `GET /v1/storefront/orders/{order_id}`, and `POST /v1/storefront/orders/{order_id}/cancel` (exact path may follow the existing router convention).
- `seller-api` (in `matjeroapps/seller`): Authenticated routes `/v1/seller/stores/{store_id}/orders`.
- `core-api` (in `matjeroapps/core`): Internal authenticated routes `/internal/v1/...`.

### 24.1 Secure Guest Order Access

Phase 5 Guest Checkout uses a dedicated Order access capability; Customer IAM
is deferred. On successful checkout, Core generates a cryptographically secure,
high-entropy random token and persists only its digest in
`orders.guest_order_access_token_digest`. The raw token is returned only through
the secure Storefront session/cookie boundary (Secure, HttpOnly, and an
appropriate SameSite policy); it is never logged, included in ordinary
analytics, exposed in Seller/Supplier DTOs, or treated as Store authority.

Every guest Order read or cancellation requires both:

1. The trusted request Host resolves through `StoreResolver` to the Store.
2. The presented capability verifies against the digest for that Order in the
   resolved Store.

An Order UUID, order number, browser-supplied `store_id`, or email alone never
authorizes access. The public Order response never contains the raw capability
after its initial secure issuance. A valid guest capability may cancel only a
`pending` Order; `confirmed` and later states return `invalid_order_transition`.
Repeated cancellation of an already-cancelled Order is idempotent. Internal
Core calls remain under `/internal/v1/...` and never accept the guest capability
as a substitute for service authentication.

---

## 25. Error Contract

Standardized HTTP error codes: `not_found` (404), `unauthorized` (401), `forbidden` (403), `invalid_argument` (400), `validation_error` (422), `market_mismatch` (422), `insufficient_inventory` (409), `price_changed` (409), `listing_unavailable` (409), `cart_expired` (409), `checkout_expired` (409), `idempotency_conflict` (409), `invalid_order_transition` (409), `conflict` (409), `internal_error` (500).

---

## 26. Security & Privacy

- Bearer Cart Token Digest: Token held in HttpOnly cookie; SHA-256 stored in DB.
- Tenant Isolation: DB composite UNIQUE keys and Store/Market composite FKs enforce Store boundary.
- Price Tampering Defense: Core re-reads authoritative listing prices inside checkout transaction.
- Internal Data Sanitation: Public DTOs omit internal IDs, costs, supplier data,
  reservation tokens, and guest access token digests. Raw guest capabilities
  are never logged or placed in ordinary analytics.
- Guest Order authorization: Trusted Storefront Host resolution plus a valid
  dedicated Order capability are both mandatory for guest reads and
  PENDING-only cancellation.

---

## 27. Database Migration Plan

Phase 5 proposes migrations to `core`:
- Migration A: `fulfillment_locations` ownership extension (support Seller-owned locations with Store/Market composite FK).
- Migration B: `customers` & `customer_addresses` tables (with Store/Market composite FK).
- Migration C: `carts` & `cart_items` tables (with Store/Market composite FK & NOT NULL price snapshot fields).
- Migration D: `checkout_sessions` table (with `finalize_fingerprint`).
- Migration E: `store_order_sequences`, `orders` (with Store/Market composite FK, `confirmation_deadline_at`, `UNIQUE (id, currency_code)`, and nullable `guest_order_access_token_digest`), `order_items` (with `FOREIGN KEY (order_id, currency_code) REFERENCES orders(id, currency_code) ON DELETE RESTRICT` and ON DELETE RESTRICT operational lineage), `order_addresses`, `order_timeline`, `order_notes`.
- Migration F: `outbox_events` claim extension columns & partial index.

---

## 28. OpenAPI Ownership Plan

OpenAPI contracts are owned per-repository and validated against drift in CI:

- **Core Internal API:** `core/docs/api/internal/openapi.json` (updated by P5.1–P5.6).
- **Seller Storefront API:** `seller/docs/api/storefront/openapi.json` (in `matjeroapps/seller`, updated by P5.7).
- **Seller Dashboard API:** `seller/docs/api/seller/openapi.json` (in `matjeroapps/seller`, updated by P5.8).

P5.0 itself changes zero generated OpenAPI documents.

---

## 29. Capability Rollout Gates

Feature exposure is controlled at actor boundaries via capability-named environment flags:

- `STOREFRONT_CHECKOUT_ENABLED` — Controls cart, checkout, and order placement routes in `storefront-api`.
- `SELLER_ORDER_MANAGEMENT_ENABLED` — Controls order management routes in `seller-api`.

Partially implemented functionality must not be exposed to traffic until the entire vertical slice is ready.

---

## 30. Observability

Operational metrics tracked:
- `checkout_attempts_total` (counter) & `checkout_failures_total` (counter, labeled by reason)
- `inventory_reservation_conflicts_total` (counter)
- `reservation_expiry_processed_total` (counter)
- `order_status_transition_total` (counter) & `invalid_order_transition_total` (counter)
- `outbox_unpublished_count` (gauge) & `outbox_oldest_unpublished_age_seconds` (gauge)
- `outbox_publish_failures_total` (counter)
- `consumer_duplicate_events_total` (counter)

---

## 31. Known Risk Matrix

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

## 32. Phase 5 Definition of Done

- [ ] Core database migrations complete & verified.
- [ ] All database composite FK constraints (`store_id, market_code`) and index definitions verified.
- [ ] Concurrent last-unit stock checkout tests green.
- [ ] Durable checkout idempotency and HTTP-replay loss tests green.
- [ ] Checkout timestamp expiry and finalized-after-expiry replay tests green.
- [ ] Checked money boundary, multiplication overflow, subtotal overflow, and zero-side-effect rollback tests green.
- [ ] PostgreSQL rejects Order Item currency mismatch through the composite `(order_id, currency_code)` FK.
- [ ] Guest Order capability, Store binding, PENDING cancellation, idempotent retry, and response-sanitization tests green.
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

## 33. Testing Strategy & Mandatory Test Matrix

Must include deterministic unit and integration tests (zero sleep-based concurrency tests):

The mandatory matrix below contains 60 deterministic tests.

### 33.1 Concurrency & Stock Allocation Tests
1. **Two available units under N checkouts:** N concurrent checkouts for a 2-unit stock → exactly 2 succeed.
2. **Same SKU across multiple fulfillment locations:** Allocates from location with lowest `inventory_snapshot.id ASC` deterministically.
3. **Multi-item in-tx rollback:** Item A succeeds, Item B stock fails → full rollback; zero surviving reservations.
4. **Supplier Offer cross-location isolation:** Supplier Offer cannot allocate Seller-owned location or another Supplier's location.
5. **Seller-owned listing isolation:** Seller-owned listing (supplier_offer_id IS NULL) cannot allocate Supplier location.

### 33.2 Idempotency & Replay Tests
6. **10 concurrent identical finalizes:** Exactly 1 Order created; all 10 return exact same Order.
7. **HTTP response loss retry:** Client retries after network drop → returns same committed Order.
8. **Fingerprint mismatch:** Same finalized session + different semantic fingerprint → `idempotency_conflict`.
9. **Validation failure state:** Validation failure (e.g. `price_changed`) rolls back and leaves Checkout Session `open` for retry.
10. **Same amount, changed currency:** Same numeric listing amount with changed listing currency returns `price_changed`.
11. **Listing/Market currency mismatch:** Listing currency differing from Store Market currency is rejected as `market_mismatch` / invariant failure.
12. **Order currency uniformity:** Every Order Item uses the single `orders.currency_code`.
13. **Mixed-currency commit prevention:** An Order with mixed item currencies cannot commit and leaves no partial rows.
14. **Incomplete address finalization:** Missing required address returns `validation_error`, leaves the session `open`, and leaves no reservation, Order, or Outbox row.
15. **Missing guest email finalization:** Missing `contact_email` returns `validation_error`, leaves the session `open`, and leaves no reservation, Order, or Outbox row.
16. **Open session before expiry:** An `open` session with `expires_at > $transaction_now` may finalize.
17. **Open session after expiry:** An `open` session with `expires_at <= $transaction_now` returns `checkout_expired`.
18. **Stale OPEN row:** Timestamp-expired `open` status cannot finalize without lifecycle cleanup; no reservation, Order, timeline, or Outbox row is created.
19. **Finalized replay after expiry:** A finalized session replay after original expiry returns the same Order only.
20. **Maximum valid multiplication:** The largest supported non-negative `unit_price_minor * quantity` that fits in `int64` succeeds.
21. **Multiplication overflow:** Checked line multiplication returns `validation_error` and persists no wrapped value.
22. **Subtotal addition overflow:** Checked subtotal addition returns `validation_error` when the next line exceeds `int64`.
23. **Overflow rollback:** Any money overflow leaves zero Order, reservation, and Outbox rows.
24. **Database currency relation:** PostgreSQL rejects an Order Item whose currency differs from its parent Order.

### 33.3 Reservation & State Machine Race Tests
25. **Confirm vs expiry race:** Concurrent Seller confirm + scheduler expiry → exactly one wins; no double inventory release.
26. **Cancel vs expiry race:** Concurrent Customer cancel + scheduler expiry → exactly one wins.
27. **Confirm vs cancel race:** Concurrent Seller confirm + Customer cancel → exactly one wins.
28. **Expiry retry:** Expiry run on already-expired reservation → no-op (no double decrement).
29. **Cancellation retry:** Cancel on already-cancelled Order → no-op (no double restock).
30. **Seller confirm consumption:** Seller confirm transitions reservation `held → consumed` exactly once.
31. **Seller cancel restock:** Seller cancel after confirmation restocks `on_hand_qty` exactly once.

### 33.4 Historical Immutability Tests
32. **Correct Store and token:** Correct resolved Store plus correct guest Order capability allows guest Order read.
33. **Wrong Store Host:** Correct capability presented through a Host resolving to another Store is rejected.
34. **Wrong guest token:** Correct Store plus wrong capability is rejected.
35. **UUID-only access:** Order UUID without capability is rejected.
36. **Order-number-only access:** Order number without capability is rejected.
37. **Guest pending cancellation:** Valid guest capability cancels a `pending` Order.
38. **Guest confirmed cancellation:** Guest capability cannot cancel `confirmed` or later states.
39. **Guest cancellation retry:** Repeating cancellation is idempotent.
40. **Capability response sanitation:** After secure issuance, the raw guest capability never appears in a public Order response body.
41. **Listing price change:** Changing listing price after order → Order amount unchanged.
42. **Product title change:** Changing product title after order → Order title snapshot unchanged.
43. **SKU code change:** Changing SKU code after order → Order SKU code snapshot unchanged.
44. **Customer address change:** Customer editing address after order → Order address snapshot unchanged.
45. **Supplier wholesale price change:** Supplier changing wholesale price → Customer Order unchanged.
46. **Source record deletion block:** Attempting to delete `SupplierOffer` or `FulfillmentLocation` while referenced by Order lineage → rejected by `ON DELETE RESTRICT`.
47. **Supplier location code uniqueness:** Duplicate `(supplier_market_id, code)` is rejected.
48. **Store location code uniqueness:** Duplicate Store-owned `(store_id, code)` is rejected while the same code remains valid for another Store.

### 33.5 Outbox Publisher Lease Tests
49. **Batch lease renewal:** Long-running publish batch renews lease before expiration using `remaining_lease` and the configured renewal margin.
50. **Multi-publisher isolation:** Two worker instances claiming batch concurrently → zero overlapping events claimed.
51. **Stale lease recovery:** Worker crash after claim → lease recovered after `OUTBOX_CLAIM_LEASE_DURATION`.
52. **Stale ACK rejection:** Stale worker acknowledge after lease reassignment fails gracefully.
53. **Broker failure backoff:** Broker failure releases claim and schedules `next_attempt_at` backoff.
54. **Confirm-then-crash duplicate:** Publish confirm crash recovery generates duplicate event handled cleanly by Consumer Inbox.
55. **Consumer Inbox duplicate suppression:** Duplicate `event_id` delivery to same consumer → side-effect executes once.
56. **Multi-consumer event delivery:** Same `event_id` delivered to two different consumer names → each processes once independently.

### 33.6 Event Ordering Tests
57. **Order aggregate versions:** Order creation emits version `1`; each meaningful emitting mutation increments it transactionally.
58. **Consumer-owned gap handling:** An ordering-dependent consumer ignores older versions and defers/retries or resyncs on a future version gap without advancing durable state.

### 33.7 State Machine Matrix Tests
59. **All Phase 5 enabled transitions:** Table-driven test asserting success, timeline entry, and outbox event.
60. **All Phase 5 rejected transitions:** Table-driven test asserting `invalid_order_transition` error and zero side-effects.

---

## 34. Repository Impact Matrix

| Repository | Impact | Description |
|---|---|---|
| **core** (`matjeroapps/core`) | **High** | Schema migrations; Customer/Cart/Checkout/Order aggregates; Outbox multi-publisher claim worker; Core internal API routes. |
| **seller** (`matjeroapps/seller`) | **High** | `apps/storefront-api` (Cart/Checkout routes); `apps/seller-api` (Order routes); `web/storefront` (Cart/Checkout UI); `web/seller` (Order Management UI). |
| **seller-hub** (`matjeroapps/seller-hub`) | **None** | README-only repository. NO Phase 5 runtime changes. |
| **admin** (`matjeroapps/admin`) | **None** | No Phase 5 runtime changes. |
| **supplier** (`matjeroapps/supplier`) | **None** | No Phase 5 runtime changes. |
| **supplier-hub** (`matjeroapps/supplier-hub`) | **None** | README-only repository. |

---

## 35. Implementation Unit Breakdown

### P5.0 — Phase Specification *(this document)*
**Repository:** core · **Branch:** feature/p5-phase-spec

### P5.1 — Customer + Cart Core Domain
**Repository:** core · **Dependencies:** P5.0 merged
Backend work: Migrations A, B, C; Cart aggregate; Parent Cart row locking on all item mutations; Store/Market composite FKs; NOT NULL expected price fields.

### P5.2 — Checkout Session + Server-Computed Fingerprint
**Repository:** core · **Dependencies:** P5.1 merged
Backend work: Migration D; CheckoutSession aggregate (`open`, `finalized`, `expired`); Fingerprint computation logic; lock the session at the beginning of finalization, freeze `$transaction_now`, and enforce `expires_at > $transaction_now` independently of persisted cleanup status. Permit only finalized replay after expiry.

### P5.3 — Order Aggregate + Sequences + State Machine
**Repository:** core · **Dependencies:** P5.2 merged (requires `checkout_sessions` table)
Backend work: Migration E; `store_order_sequences`; Order aggregate with `confirmation_deadline_at`, `UNIQUE (id, currency_code)`, dedicated `guest_order_access_token_digest`, composite Order/Order Item currency FK, and operational lineage RESTRICT FKs; Order State Machine.

### P5.4 — Inventory Reservation Lifecycle + Allocation + Expiry
**Repository:** core · **Dependencies:** P5.3 merged (requires `order_items` table)
Backend work: Reservation state machine (`held`, `consumed`, `released`, `expired`); Movement `quantity_delta` rules; Two-stage expiry scheduler job based on `confirmation_deadline_at`.

### P5.5 — Atomic Single-Transaction Checkout
**Repository:** core · **Dependencies:** P5.2 + P5.3 + P5.4 merged
Backend work: Full single-transaction checkout implementation; Correct fingerprint ordering; Multi-item stock lock & reserve; checked signed-64-bit money multiplication/addition with `validation_error` rollback; atomic high-entropy guest capability generation and digest persistence; Idempotent replay handling (10 concurrent finalizes → 1 Order).

### P5.6 — Outbox Multi-Publisher Claim & Delivery Reliability
**Repository:** core · **Dependencies:** P5.5 merged
Backend work: Migration F; Bounded claim loop with `FOR UPDATE SKIP LOCKED`, partial index, and per-event lease renewal; Publisher confirms & backoff logic; Domain event emission.

### P5.7 — Storefront API + Storefront Web Cart & Checkout
**Repository:** seller (`apps/storefront-api`, `web/storefront`) · **Dependencies:** P5.6 merged (requires reliable transaction & event foundation)
Backend & Frontend work: Storefront cart & checkout routes; Bearer cart cookie management; Next.js checkout UI; resolved-Host plus guest-capability Order read and PENDING-only cancellation routes. No UUID/order-number-only, browser-supplied Store, or email-only authorization.

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

## 36. Dependency Graph

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

## 37. Self-Review: Architecture Assertions

| Question | Answer | Enforcement |
|---|---|---|
| Is Supplier Offer product lineage correctly validated? | **YES** | Verified via `SupplierOffer → SupplierProduct → product_id == SellerListing.product_id`. |
| Are Store and Market pair mismatches prevented in DB? | **YES** | Composite FK `(store_id, market_code) REFERENCES stores(id, market_code)` on `customers`, `carts`, `orders`. |
| Is confirmation deadline frozen once at checkout? | **YES** | Computed `$deadline` ONCE in tx and stored on `orders.confirmation_deadline_at` & `inventory_reservations.expires_at`. |
| Can an OPEN Checkout Session finalize after `expires_at`? | **NO** | Finalization locks the session, freezes `$transaction_now`, and requires `status = 'open' AND expires_at > $transaction_now`; cleanup is not authoritative. |
| Can a finalized Checkout Session replay after `expires_at`? | **YES** | Fingerprint-matching replay returns the same existing Order only; it creates no new side effects. |
| Can `line_total_minor` or `subtotal_minor` overflow `int64` silently? | **NO** | `checkedMultiply` and `checkedAdd` return `validation_error` and roll back before persistence. |
| Is reservation expiry locking safe? | **YES** | Stage 1 reads candidate Order IDs; Stage 2 locks Order & snapshots in dedicated transaction. |
| Can lease renewal run using elapsed time as if it were remaining time? | **NO** | `remaining_lease = publish_claimed_at + OUTBOX_CLAIM_LEASE_DURATION - now()`; elapsed age is `now() - publish_claimed_at`, and renewal uses `OUTBOX_CLAIM_RENEWAL_MARGIN`. |
| Does outbox worker renew lease during long batches? | **YES** | Per-event renewal check before publish when `remaining_lease <= OUTBOX_CLAIM_RENEWAL_MARGIN`, with matching-claim and unexpired-lease predicates. |
| Is operational source lineage preserved on Order Items? | **YES** | `supplier_offer_id`, `fulfillment_location_id`, `inventory_reservation_id` use `ON DELETE RESTRICT`. |
| Can a crash during checkout leave Checkout Session stuck FINALIZING? | **NO** | Single PostgreSQL transaction; rollback leaves session OPEN; no persistent FINALIZING or FAILED state in DB. |
| Can two publisher instances routinely claim/publish the same outbox row? | **NO** | Exclusive bounded lease via `publish_claim_id` set within `FOR UPDATE SKIP LOCKED`. |
| Can a stale outbox claim recover? | **YES** | Re-claimable when `publish_claimed_at < (now() - OUTBOX_CLAIM_LEASE_DURATION)`. |
| Can publish-confirm crash cause duplicate delivery? | **YES** | At-least-once behavior; handled by Consumer Inbox `processed_events`. |
| Can an Order mix currencies? | **NO** | `orders.currency_code` is the Store Market currency and every Order Item must match it. |
| Can an Order Item persist a different currency than its Order? | **NO** | `UNIQUE (orders.id, orders.currency_code)` plus `FOREIGN KEY (order_items.order_id, order_items.currency_code)` makes the relation database-enforced. |
| Can a Listing price currency differ from Store Market currency and still checkout? | **NO** | Checkout rejects the mismatch as `market_mismatch` / invariant failure. |
| Do confirm/cancel/expiry use the same lock ordering? | **YES** | They lock Order, inventory snapshots by id ASC, then inventory reservations by id ASC. |
| Can one Store create duplicate fulfillment location codes? | **NO** | Partial unique index `fulfillment_locations_store_code_uidx` enforces `(store_id, code)`. |
| Can a guest finalize without delivery/contact data? | **NO** | Finalization validates required address fields and `contact_email` before reservation or Order creation. |
| Can a guest read/cancel an Order with only its UUID or order number? | **NO** | Guest operations require resolved Store Host plus the dedicated high-entropy Order capability. |
| Can a guest securely cancel its own PENDING Order? | **YES** | A valid capability scoped to the resolved Store may perform the Customer-authorized PENDING → CANCELLED transition. |
| Can a guest cancel CONFIRMED or later? | **NO** | The state machine returns `invalid_order_transition`; cancellation retry is idempotent only for an already-cancelled Order. |
| Does `processed_events` claim to enforce aggregate ordering? | **NO** | It only suppresses duplicates by `(consumer_name, event_id)`. |
| Can an ordering-dependent consumer detect aggregate version gaps? | **YES** | Consumer-owned durable last-applied version state detects gaps and applies retry/defer/resync policy. |
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
