# Matjero — Phase 5: Cart, Checkout, Orders and Inventory Transactions

**Status:** Specification — DO NOT implement until this document is merged.
**Base SHA:** `4ebd38a496bf0d58ce3c6f286cca08436460da55`
**Canonical branch:** `feature/p5-phase-spec`

---

## 1. Executive Summary

Phase 5 introduces the complete transactional commerce loop for Matjero. It
implements Cart management, the Checkout Session, atomic Order creation, and
authoritative Inventory Reservations. This phase transforms the platform from a
catalog-browsing service into a transactional marketplace while preserving every
existing architectural invariant: PostgreSQL authority, RabbitMQ async backbone,
repository independence (ADR-017), market isolation, and strict multi-tenant
boundaries.

The checkout critical path is:

```
Validate Store → Validate Market → Validate Listings → Validate Prices
→ Reserve Inventory → Create Order → Create Order Items
→ Create Order Timeline entry → Create Outbox Events
→ Finalize Checkout Session → COMMIT → Return Order
```

All steps are one atomic PostgreSQL transaction. Any failure rolls back
everything.

---

## 2. Goals

- Implement the `customers`, `customer_addresses`, `carts`, `cart_items`,
  `checkout_sessions`, `orders`, `order_items`, `order_addresses`,
  `order_timeline`, and `order_notes` aggregates.
- Extend `fulfillment_locations` to support Seller-owned locations (Store-scoped
  inventory).
- Define a strictly consistent, oversell-proof atomic checkout transaction.
- Activate the Transactional Outbox (ADR-006) for commerce events.
- Define the Consumer Inbox (ADR-007) semantics for Phase 5 consumers.
- Establish the Order State Machine with explicit transition authority.
- Activate reservation expiry and Order cancellation compensation workflows.
- Provide API surface in `storefront-api` (customer path) and `seller-api`
  (dashboard path), both calling `core-api` over authenticated internal HTTP.

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
- **Supplier runtime work** — Phase 5 adds no Supplier API surfaces unless the
  Master Plan explicitly requires them for an operational function.
- **Admin runtime work** — Phase 5 adds no Admin API surfaces.
- **SHIPPED / OUT_FOR_DELIVERY / DELIVERED state activation** — These states are
  defined in the schema and state machine but cannot be reached without Phase 6
  shipping authority. No fake endpoints.
- **RETURNED state activation** — Defined but owned by a future returns workflow.

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
- `inventory_reservations` (`reservation_token`, `status`, `quantity`,
  `expires_at`). Existing status values in code: **`held`** (with `released`,
  `expired`, `consumed` as the intended lifecycle — see Section 14).
- `inventory_movements` — append-only ledger tracking `quantity_delta`,
  `on_hand_qty`, `reserved_qty` per snapshot.
- `outbox_events` and `processed_events` schemas present but no commerce events
  yet published.
- Core acts as the sole PostgreSQL authority. Actor services communicate with Core
  exclusively via authenticated HTTP (`/internal/v1/...`, ADR-017).
- Service callers: `CallerSeller`, `CallerAdmin`, `CallerSupplier`.

**Key gap this phase closes:** no customer, cart, checkout, or order domain
exists yet. The Outbox is wired but has never published a commerce event.

---

## 5. Business Decisions

The following decisions were resolved for Phase 5:

| Topic | Decision |
|---|---|
| Customer identity | Core maintains a lightweight `customers` profile per Store. Passwords are never stored; authenticated customers are linked via external provider `(identity_provider, identity_subject)`. Guest checkout is supported; identity fields are NULL for guests. |
| Cart identity | Carts are identified by a high-entropy bearer token (Section 8). For authenticated customers the Cart is also linked to `(customer_id, store_id)`. |
| Authenticated cart uniqueness | One `ACTIVE` cart per `(customer_id, store_id)`. Guest cart is merged transactionally on authentication if one already exists. |
| Order state on checkout | Checkout creates a `PENDING` Order and a `held` reservation. |
| Seller confirmation | Seller confirmation transitions `PENDING → CONFIRMED` and `held → consumed` (reservation consumed; inventory decremented). |
| Cancellation — customer | Customer may cancel only `PENDING` Orders. |
| Cancellation — seller | Seller may cancel `PENDING`, `CONFIRMED`, or `PROCESSING`. `READY_FOR_SHIPPING` onward is forbidden. |
| Confirmation deadline | A bounded confirmation deadline is enforced. When it expires, the PENDING Order transitions to `CANCELLED` (reason: `confirmation_timeout`) and the `held` reservation transitions to `expired`. |
| Price change at checkout | Checkout rejects with `price_changed` if authoritative price ≠ cart snapshot price. |
| Seller-owned inventory | Supported: `fulfillment_locations` is extended to support a Store-scoped ownership branch alongside Supplier-owned branch. |
| Order total Phase 5 | `total_minor = subtotal_minor = sum(line_total_minor)`. No shipping, tax, or discount components. |
| Checkout idempotency | Canonicalized around Checkout Session finalization (Section 13). |
| Phase 5 allocations | ONE Order Item uses ONE Fulfillment Location. No split-shipment allocations. |
| Guest Order lookup | Guest post-checkout Order lookup is **deferred** to a future phase. It requires a secure order-access token design beyond Phase 5 scope. |

---

## 6. Repository Boundaries

Phase 5 obeys ADR-017. No cross-repository Go imports. All inter-service
collaboration is network-based.

```
                         Customer Browser
                               │
                               ▼
               ┌───────────────────────────────┐
               │   web/storefront (Next.js)     │  ← seller-hub/storefront
               │   (seller-hub repo)            │
               └────────────────┬──────────────┘
                                │ HTTP
                                ▼
               ┌───────────────────────────────┐
               │  apps/storefront-api (Go)      │  ← seller repo
               │  /v1/storefront/...            │
               │  (anonymous public routes)     │
               └────────────────┬──────────────┘
                                │ authenticated HTTP
                                │ X-Matjero-Service: seller
                                │ X-Matjero-Storefront-Host: <trusted host>
                                ▼
               ┌───────────────────────────────┐
               │  apps/core-api (Go)            │  ← core repo
               │  /internal/v1/...              │
               │  PostgreSQL authority          │
               └───────────────────────────────┘

                       Seller Dashboard Browser
                               │
                               ▼
               ┌───────────────────────────────┐
               │   web/seller (React)           │  ← seller-hub/seller
               │   (seller-hub repo)            │
               └────────────────┬──────────────┘
                                │ HTTP + OIDC JWT
                                ▼
               ┌───────────────────────────────┐
               │  apps/seller-api (Go)          │  ← seller repo
               │  /v1/seller/...                │
               │  (authenticated seller routes) │
               └────────────────┬──────────────┘
                                │ authenticated HTTP
                                │ X-Matjero-Service: seller
                                ▼
               ┌───────────────────────────────┐
               │  apps/core-api (Go)            │  ← core repo
               │  /internal/v1/...              │
               └───────────────────────────────┘
```

**Key invariants:**
- `storefront-api` is a separate Go binary from `seller-api`. Both live in the
  `seller` repository under `apps/`.
- `storefront-api` is anonymous (no OIDC/ZITADEL). Tenant identity comes solely
  from the trusted `X-Matjero-Storefront-Host` header forwarded to Core.
- `seller-api` requires ZITADEL OIDC JWT. The seller principal is derived from
  the validated JWT subject.
- Core trusts `X-Matjero-Service: seller` for both binaries; the distinction
  between storefront and dashboard is that the storefront path additionally
  carries the trusted host and the dashboard path carries the validated seller
  principal.
- No actor may access Core's PostgreSQL directly.
- No actor-to-actor service calls for Core commerce workflows.

---

## 7. Customer Model

### 7.1 Purpose

Core maintains a lightweight commerce profile for each customer per Store. This
is distinct from ZITADEL, which manages platform actors (Sellers, Admins,
Suppliers). Customers are end-users of Seller storefronts.

### 7.2 Identity

Customers may be:

1. **Guest** — No identity provider linkage. Identified during a session by a
   `cart_token`. Email captured at checkout.
2. **Authenticated** — Linked to an external identity provider via
   `(store_id, identity_provider, identity_subject)`. The provider and subject
   are opaque references; Core never validates the token — that is the
   storefront-api's responsibility.

No passwords are ever stored in Core.

### 7.3 `customers` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | FK → `stores(id)` ON DELETE RESTRICT |
| `market_code` | `CHAR(2)` | FK → `markets(code)`; composite FK → `stores(id, market_code)` |
| `identity_provider` | `TEXT` | Nullable; NOT NULL when authenticated |
| `identity_subject` | `TEXT` | Nullable; NOT NULL when authenticated |
| `email` | `TEXT` | Nullable; captured at checkout for guests or authenticated |
| `display_name` | `TEXT` | Nullable |
| `status` | `TEXT` | NOT NULL; CHECK IN (`active`, `blocked`) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique constraints:**
- `(store_id, identity_provider, identity_subject)` — enforced only when
  `identity_provider IS NOT NULL AND identity_subject IS NOT NULL` (partial
  unique index).

**Delete policy:** `ON DELETE RESTRICT` from `stores` — customer history must
not cascade-delete because a Store is deactivated.

### 7.4 `customer_addresses` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `customer_id` | `UUID` | FK → `customers(id)` ON DELETE CASCADE |
| `label` | `TEXT` | Nullable display label (e.g. "Home") |
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

---

## 8. Cart Aggregate

### 8.1 Identity and Security

A Cart is identified by a **high-entropy bearer token** (`cart_token`) generated
by Core on Cart creation. This token is:

- A cryptographically random value (e.g. 32 bytes encoded as a URL-safe base64
  string, or a UUID v4). Not the Cart's database `id`.
- Delivered to the browser through a `Set-Cookie: cart_token=<value>; HttpOnly;
  Secure; SameSite=Strict` response (or an equivalent secure mechanism). Never
  in the URL path.
- **Never logged** — not in access logs, application logs, analytics, or metrics.
- **Never used as store authority** — tenant is derived exclusively from the
  trusted host, not the cart_token.
- If persisted, stored as a one-way digest (e.g. `SHA-256`) in the `carts` table
  so that a database breach does not expose bearer secrets.

The `carts.id` (UUID PK) is the internal database key. The cart_token is a
separate bearer secret used only to look up `carts.id` in the same Store context.

### 8.2 Scope Invariants

- One Cart belongs to exactly ONE `store_id` and `market_code`.
- Store identity comes only from the trusted `X-Matjero-Storefront-Host`
  header; it is never derived from client-supplied IDs.
- A cart_token is valid only within the resolved Store. A token from Store A
  cannot address a Cart in Store B.

### 8.3 Authenticated Cart Uniqueness and Merge

- There is at most ONE Cart in status `active` per `(customer_id, store_id)`.
- When a guest authenticates, the guest Cart may be merged into the authenticated
  Customer's existing Cart (or the guest Cart becomes the authenticated Cart if
  none exists). The merge is transactional.
- Merge line identity: `(seller_listing_id, sku_id)`.
- Merged quantities must remain positive, bounded by a configured maximum, and
  within valid stock at checkout (not at merge time).

### 8.4 `carts` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `store_id` | `UUID` | NOT NULL; composite FK → `stores(id, market_code)` |
| `market_code` | `CHAR(2)` | NOT NULL FK → `markets(code)` |
| `customer_id` | `UUID` | Nullable FK → `customers(id)` ON DELETE SET NULL |
| `cart_token_digest` | `TEXT` | NOT NULL UNIQUE; SHA-256 of bearer token |
| `status` | `TEXT` | NOT NULL CHECK IN (`active`, `checked_out`, `abandoned`, `expired`) |
| `expires_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique constraint:** partial `UNIQUE (customer_id, store_id) WHERE status = 'active' AND customer_id IS NOT NULL`.

**Indexes:** `(store_id, status)`, `(cart_token_digest)`.

**Delete policy:** Carts may be purged (hard-deleted) after sufficient retention
period because they are not historical business records. Checked-out Carts are
retained until linked Orders are settled, then eligible for archival/deletion.

### 8.5 `cart_items` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `cart_id` | `UUID` | NOT NULL FK → `carts(id)` ON DELETE CASCADE |
| `seller_listing_id` | `UUID` | NOT NULL FK → `seller_listings(id)` ON DELETE RESTRICT |
| `sku_id` | `UUID` | NOT NULL FK → `skus(id)` ON DELETE RESTRICT |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0 AND quantity <= 10000) |
| `expected_unit_price_minor` | `BIGINT` | Nullable; display-only snapshot captured from the Storefront response at add-to-cart time. NOT authoritative. |
| `expected_currency_code` | `CHAR(3)` | Nullable FK → `currencies(code)` |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique constraint:** `UNIQUE (cart_id, seller_listing_id, sku_id)`.

**SKU/Listing invariant (enforced at application layer):** The SKU's Variant's
Product must match `seller_listing.product_id`. Core validates this during
add-to-cart and again at checkout.

---

## 9. SKU / Listing / Source Semantics

### 9.1 Relationship Map

```
Customer selects:
  Seller Listing (product-level context, price authority)
      │
      ├── product_id ──→ Product → Variant → SKU  (customer selects SKU)
      │
      └── supplier_offer_id (nullable)
              │ if NOT NULL (Supplier-sourced)
              └── Supplier Offer → Supplier Product → product_id
                      │
                      └── must equal Listing.product_id ✓

Cart Item stores:
  seller_listing_id + sku_id

Checkout validates:
  SKU → Variant → Product == Listing.product_id   ✓
  Listing.status == active                          ✓
  Listing.market_code == Store.market_code          ✓

  If Supplier-sourced:
    supplier_offer.status == active                 ✓
    supplier_offer.market_code == Store.market_code ✓
    supplier_offer.supplier_product.product_id == Listing.product_id ✓
    Fulfillment location belongs to the Supplier behind supplier_offer ✓

  If Seller-owned (supplier_offer_id IS NULL):
    Fulfillment location belongs to the Store                         ✓
```

### 9.2 Current Offer Granularity

`supplier_offers` are currently at **Product level**, not SKU level. Therefore
the checkout rule for a Supplier-sourced Listing is:

> The Inventory Snapshot selected for reservation must have `sku_id` equal to the
> customer's chosen SKU, and the Fulfillment Location must belong to the Supplier
> who owns the Supplier Offer linked to the Listing.

This means a single Supplier Offer may source multiple SKUs (different sizes,
colors) as long as those SKUs all share the same Product, and as long as the
Supplier has inventory snapshots for each SKU. The checkout step verifies that
the specific chosen SKU has sufficient stock at a location owned by that Supplier.

**No `supplier_offer_sku` mapping table is introduced in Phase 5.** The
Product-level offer + SKU + inventory-location ownership rule is sufficient.

### 9.3 What the Storefront Must Never Receive

The public Storefront DTOs **must never** expose:
- `supplier_id`
- `supplier_offer_id`
- `supplier_market_id`
- Wholesale cost / supplier margin
- Fulfillment location identity
- `reservation_token`
- Internal service identities

---

## 10. Checkout Session

### 10.1 Purpose

A Checkout Session isolates temporary checkout state from the durable Cart and
final Order. It represents **the intent to validate and place an Order** (not
payment intent).

### 10.2 Lifecycle

```
OPEN ──── (validation failure) ──→ OPEN  (customer may retry)
   │
   └─── (finalize) ──→ FINALIZING (lock, validate, transact)
                              │
                 ┌────────────┴────────────┐
                 ▼                         ▼
           FINALIZED                   FAILED
         (order_id set)        (no order, retry permitted)
```

A `FINALIZED` session has exactly one `order_id`. A `FAILED` session may be
retried (new finalize call overwrites the failure) until `expires_at`.

### 10.3 `checkout_sessions` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK; the finalization idempotency identity |
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `cart_id` | `UUID` | NOT NULL FK → `carts(id)` ON DELETE RESTRICT |
| `customer_id` | `UUID` | Nullable FK → `customers(id)` ON DELETE SET NULL |
| `status` | `TEXT` | NOT NULL CHECK IN (`open`, `finalizing`, `finalized`, `failed`, `expired`) |
| `expires_at` | `TIMESTAMPTZ` | NOT NULL |
| `shipping_address_snapshot` | `JSONB` | Nullable; deep copy of customer's selected delivery address at session creation time |
| `contact_email` | `TEXT` | Nullable; filled in at checkout by guest |
| `finalize_fingerprint` | `TEXT` | Nullable; a stable hash of the finalize request payload (cart items + prices) |
| `order_id` | `UUID` | Nullable; set only when `status = 'finalized'` |
| `finalized_at` | `TIMESTAMPTZ` | Nullable |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique constraint:** `UNIQUE (order_id) WHERE order_id IS NOT NULL` — one
Checkout Session produces at most one Order.

**Indexes:** `(store_id, status)`, `(cart_id)`, `(expires_at) WHERE status IN ('open', 'finalizing')`.

---

## 11. Price Validation

### 11.1 Cart Price Semantics

`cart_items.expected_unit_price_minor` is a **display-only snapshot** captured
from the authoritative Storefront price response at the moment the item is added
to the cart. It is:

- Shown to the customer in the cart review screen.
- **Never used** as the authoritative purchase price.
- Used at checkout solely for comparison to trigger a `price_changed` error.

### 11.2 Checkout Price Revalidation

Inside the atomic checkout transaction, Core:

1. Reads the current `seller_listing_prices.amount_minor` and `currency_code`
   for each Cart Item's `seller_listing_id`.
2. Compares to the Cart Item's `expected_unit_price_minor` (if present).
3. If the current price ≠ the expected snapshot price: **ROLLBACK** and return
   `price_changed` with the current safe price for the customer to review.
4. Uses the current authoritative listing price as `unit_price_minor` in the
   Order Item and Checkout total calculation.

**Browser-submitted totals are never trusted.**

### 11.3 Total Calculation (Phase 5)

```
line_total_minor       = unit_price_minor * quantity        (overflow-checked)
order.subtotal_minor   = SUM(line_total_minor)               (overflow-checked)
order.total_minor      = order.subtotal_minor                (no shipping/tax/discount)
```

All values are signed 64-bit integers representing minor currency units. Floating
point is never used. Currency is always explicit.

---

## 12. Durable Checkout Idempotency

### 12.1 Model

Idempotency is canonicalized around the **Checkout Session `id`**. The Session ID
is provided by the caller (storefront-api) before the finalize call, allowing
safe retries.

```
POST /internal/v1/checkout-sessions/{session_id}/finalize
  Body: { cart_id, fingerprint, ... }
```

### 12.2 Finalization Protocol

```
BEGIN
  SELECT checkout_session WHERE id = $1 FOR UPDATE

  CASE status:
    'finalized':
      IF fingerprint matches → COMMIT, return existing order
      IF fingerprint differs → ROLLBACK, return idempotency_conflict
    'expired':
      → ROLLBACK, return checkout_expired
    'finalizing':
      → ROLLBACK, return conflict (concurrent finalize in progress)
    'open' or 'failed':
      SET status = 'finalizing'
      COMMIT  ← fence other concurrent requests

BEGIN (main checkout transaction)
  ... validate, lock inventory, create Order ...
  SET checkout_session.status = 'finalized'
  SET checkout_session.order_id = <new order id>
  SET checkout_session.finalized_at = now()
  SET checkout_session.finalize_fingerprint = $fingerprint
  INSERT outbox_events (OrderCreated)
COMMIT
```

### 12.3 Database Enforcement

- `UNIQUE (order_id) WHERE order_id IS NOT NULL` on `checkout_sessions` — prevents
  two sessions pointing to the same Order.
- `UNIQUE (id)` on `orders` — prevents duplicate Order rows.
- The `status = 'finalizing'` intermediate state acts as a database-level
  optimistic fence against concurrent finalization for the same session.

### 12.4 Invariant

```
Same session_id + same fingerprint     → same Order (idempotent success)
Same session_id + different fingerprint → idempotency_conflict
Concurrent finalizes for same session   → exactly one Order
```

---

## 13. Inventory Model (Reuse)

Phase 5 reuses the existing `inventory_snapshots` and `inventory_reservations`
tables without schema changes to these tables (except adding linkage columns to
reservations — see Section 14.4).

```
available_qty = on_hand_qty - reserved_qty
```

The CHECK constraint `reserved_qty <= on_hand_qty` on `inventory_snapshots` is
the final database-level oversell guard.

---

## 14. Inventory Reservation State Machine

### 14.1 Persisted Status Values

The existing codebase uses `"held"` for an active reservation. Phase 5 canonicalizes the full lifecycle:

| Status | Meaning |
|---|---|
| `held` | Reservation holds stock. `reserved_qty` is incremented. Has `expires_at`. |
| `consumed` | Order confirmed. `reserved_qty -= quantity` and `on_hand_qty -= quantity`. Terminal. |
| `released` | Order cancelled before confirmation. `reserved_qty -= quantity`. Terminal. |
| `expired` | Confirmation deadline elapsed. `reserved_qty -= quantity`. Terminal. |

**Terminal states do not transition again.** No `released → consumed`. No
`consumed → released`. No `expired → consumed`.

### 14.2 State Transitions

```
(checkout succeeds)
      ↓
   [ held ]
      │
      ├─── Seller confirms Order
      │         ↓
      │    [ consumed ]  ← terminal (on_hand_qty decremented)
      │
      ├─── Customer or Seller cancels PENDING Order
      │         ↓
      │    [ released ]  ← terminal (reserved_qty decremented, on_hand_qty unchanged)
      │
      └─── Expiry worker fires (confirmation_timeout)
                ↓
          [ expired ]   ← terminal (reserved_qty decremented, on_hand_qty unchanged)
```

### 14.3 Idempotency

Every transition is idempotent:
- If a reservation is already in a terminal state, a repeat transition attempt
  returns success without modifying inventory.
- Concurrent cancel + expiry: both attempt `UPDATE inventory_reservations SET status = $terminal WHERE id = $1 AND status = 'held'`. Exactly one wins; the other sees `rows affected = 0` and exits cleanly.

### 14.4 Reservation / Order Linkage

`inventory_reservations` must link to the Order Item that owns it. Phase 5
migration adds:

| Column | Type | Notes |
|---|---|---|
| `checkout_session_id` | `UUID` | Nullable FK → `checkout_sessions(id)` ON DELETE SET NULL; set at reservation creation |
| `order_item_id` | `UUID` | Nullable FK → `order_items(id)` ON DELETE SET NULL; set when Order Item is created in the same transaction |

This allows every Order Item to determine its SKU, fulfillment location,
reservation, and source Supplier Offer lineage without public exposure.

### 14.5 `inventory_movements` Integration

Phase 5 must record an `inventory_movements` row for every stock-changing
inventory event. The existing table (`movement_type`, `quantity_delta`,
`on_hand_qty`, `reserved_qty`, `reason`, `principal_subject`, `correlation_id`,
`causation_id`) supports all required events. No new columns are needed.

| Event | `movement_type` | `quantity_delta` | `on_hand_qty` changes? | `reserved_qty` changes? |
|---|---|---|---|---|
| Reservation `held` | `reservation_held` | `+quantity` | No | Yes (`+quantity`) |
| Reservation `released` (cancel PENDING) | `reservation_released` | `-quantity` | No | Yes (`-quantity`) |
| Reservation `expired` | `reservation_expired` | `-quantity` | No | Yes (`-quantity`) |
| Reservation `consumed` (confirm) | `reservation_consumed` | `-quantity` | Yes (`-quantity`) | Yes (`-quantity`) |
| Order cancellation restock (cancel CONFIRMED/PROCESSING) | `order_cancellation_restock` | `+quantity` | Yes (`+quantity`) | No |

---

## 15. Concurrency Strategy

### 15.1 PostgreSQL is the Sole Authority

Redis locks **must not** be used for correctness guarantees on inventory or order
creation. No distributed lock replaces PostgreSQL row-level serialization.

### 15.2 Lock Acquisition Order

To avoid deadlocks, all concurrent transactions must acquire locks in the same
deterministic order:

1. `checkout_sessions` row (by `id`) — fences concurrent finalization.
2. `inventory_snapshots` rows — sorted by `id ASC` (stable total order; not SKU
   order because the same SKU may exist at multiple locations).
3. `orders` rows — only after creation, for status transitions.

### 15.3 Inventory Lock Strategy

Phase 5 uses **pessimistic row locking** for multi-item checkout:

```sql
SELECT * FROM inventory_snapshots
WHERE id = ANY($snapshot_ids)
ORDER BY id ASC
FOR UPDATE
```

After acquiring locks:

```sql
UPDATE inventory_snapshots
SET reserved_qty = reserved_qty + $quantity,
    version = version + 1,
    updated_at = now()
WHERE id = $snapshot_id
  AND on_hand_qty - reserved_qty >= $quantity
```

If `rows affected = 0` → `insufficient_inventory` → ROLLBACK.

The `CHECK (reserved_qty <= on_hand_qty)` constraint acts as the final database
guard.

### 15.4 Multi-Item Atomicity

All Cart Items are one atomic checkout. If Item A allocates successfully and Item
B fails, the entire transaction rolls back: no Order, no Items, no reservations,
no Outbox events, no Checkout Session finalization. The caller receives
`insufficient_inventory` with the failing item identified.

---

## 16. Fulfillment Location Ownership

### 16.1 Dual Ownership Model

`fulfillment_locations` supports two exclusive ownership branches:

**Supplier-owned (existing):**
- `supplier_id NOT NULL`
- `supplier_market_id NOT NULL`
- `store_id NULL`

**Seller-owned (new in Phase 5):**
- `supplier_id NULL`
- `supplier_market_id NULL`
- `store_id NOT NULL`
- Seller identity is derived through `stores.seller_id`.

### 16.2 Schema Changes

Phase 5 migration alters `fulfillment_locations`:

- Make `supplier_id` and `supplier_market_id` **nullable** (currently NOT NULL).
- Add `store_id UUID NULLABLE FK → stores(id) ON DELETE RESTRICT`.
- Add `CHECK` constraint: exactly one ownership branch:
  ```sql
  CHECK (
    (supplier_id IS NOT NULL AND supplier_market_id IS NOT NULL AND store_id IS NULL)
    OR
    (supplier_id IS NULL AND supplier_market_id IS NULL AND store_id IS NOT NULL)
  )
  ```
- Adjust Supplier-branch FK: the existing composite FK on `(supplier_market_id, supplier_id, market_code)` remains.
- Add Seller-branch FK: `(store_id, market_code) REFERENCES stores(id, market_code)`.
- **Unique location code** must be branch-scoped:
  - Supplier: existing `UNIQUE (supplier_market_id, code) WHERE supplier_market_id IS NOT NULL`
  - Seller: new `UNIQUE (store_id, code) WHERE store_id IS NOT NULL`

### 16.3 Cross-Store / Cross-Market Prohibition

- Seller-owned fulfillment locations are Store-scoped in Phase 5.
- Shared inventory locations across multiple Seller Stores are **NOT implemented
  in Phase 5**.
- Cross-market inventory is prohibited by the market FK on `fulfillment_locations`.

### 16.4 Allocation Rule

Phase 5 checkout selects ONE Fulfillment Location per Order Item:

```
Supplier-sourced listing:
  Candidate locations WHERE:
    fulfillment_location.supplier_id == supplier_offer.supplier_id
    AND fulfillment_location.market_code == store.market_code
    AND fulfillment_location.status == 'active'
    AND inventory_snapshot.sku_id == selected_sku_id
    AND (inventory_snapshot.on_hand_qty - inventory_snapshot.reserved_qty) >= qty

Seller-owned listing (supplier_offer_id IS NULL):
  Candidate locations WHERE:
    fulfillment_location.store_id == cart.store_id
    AND fulfillment_location.market_code == store.market_code
    AND fulfillment_location.status == 'active'
    AND inventory_snapshot.sku_id == selected_sku_id
    AND (inventory_snapshot.on_hand_qty - inventory_snapshot.reserved_qty) >= qty
```

**Deterministic selection:** if multiple candidate locations satisfy the
constraints, select the one with the **lowest `inventory_snapshot.id` ASC**
(stable, does not require secondary ordering heuristics in Phase 5).

If no single location satisfies the full quantity for the line: `insufficient_inventory`.

Phase 6 may introduce shipping-aware location selection. Phase 5 keeps it
deliberately minimal.

---

## 17. Checkout Transaction Boundary

The full atomic transaction sequence:

```
BEGIN

  1. SELECT checkout_session FOR UPDATE
     ASSERT status IN ('open', 'failed')
     SET status = 'finalizing'
     (early commit to fence concurrent requests)

BEGIN  ← main checkout transaction

  2. Validate Store status == 'active'
  3. Validate Store.market_code matches Cart.market_code
  4. Validate Cart status == 'active', not expired
  5. FOR EACH Cart Item:
       a. Validate seller_listing.status == 'active'
       b. Validate seller_listing.store_id == store.id
       c. Validate seller_listing.market_code == store.market_code
       d. Validate sku → variant → product == listing.product_id
       e. If Supplier-sourced: validate supplier_offer active, market match,
          product match
       f. Read current seller_listing_prices.amount_minor
       g. Compare to cart_item.expected_unit_price_minor
          → price_changed if mismatch
       h. Determine candidate fulfillment_location (Section 16.4)
       i. SELECT inventory_snapshot FOR UPDATE (ordered by id ASC)
       j. Check available_qty >= quantity → insufficient_inventory if not

  6. FOR EACH Cart Item (after all locks acquired):
       a. UPDATE inventory_snapshot: reserved_qty += quantity, version++
       b. INSERT inventory_reservation (status='held', expires_at=now()+interval)
       c. INSERT inventory_movement (movement_type='reservation_held')

  7. INSERT orders (status='PENDING', aggregate_version=1)
  8. INSERT order_items (one per Cart Item, immutable snapshot)
  9. UPDATE inventory_reservations SET order_item_id = <new item id>
  10. INSERT order_timeline (from=NULL, to='PENDING', actor='checkout')
  11. INSERT order_addresses (snapshot of shipping_address_snapshot from session)
  12. INSERT outbox_events (event_type='commerce.order.created', schema_version=1)
  13. UPDATE checkout_sessions SET status='finalized', order_id=<id>, finalized_at=now()
  14. UPDATE carts SET status='checked_out'

COMMIT

```

Any step failure → full ROLLBACK. The caller receives a deterministic error code.

---

## 18. Order Aggregate

### 18.1 `orders` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_number` | `TEXT` | NOT NULL UNIQUE per store; human-readable (e.g. `#1001`) |
| `store_id` | `UUID` | NOT NULL FK → `stores(id)` ON DELETE RESTRICT |
| `market_code` | `CHAR(2)` | NOT NULL FK → `markets(code)` |
| `customer_id` | `UUID` | Nullable FK → `customers(id)` ON DELETE SET NULL |
| `checkout_session_id` | `UUID` | Nullable FK → `checkout_sessions(id)` ON DELETE SET NULL |
| `status` | `TEXT` | NOT NULL CHECK IN (`pending`, `confirmed`, `processing`, `ready_for_shipping`, `shipped`, `out_for_delivery`, `delivered`, `cancelled`, `returned`) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `subtotal_minor` | `BIGINT` | NOT NULL CHECK (subtotal_minor >= 0) |
| `total_minor` | `BIGINT` | NOT NULL CHECK (total_minor >= 0) |
| `cancellation_reason` | `TEXT` | Nullable; set on CANCELLED |
| `aggregate_version` | `BIGINT` | NOT NULL DEFAULT 1; increments on every meaningful state change |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Unique constraint:** `UNIQUE (store_id, order_number)`.
**Indexes:** `(store_id, status)`, `(customer_id, store_id)`, `(status, created_at)` for expiry queries.

**Delete policy:** `ON DELETE RESTRICT` from `stores`, `markets`, `currencies`.
Orders are historical business records and must never be cascade-deleted.

### 18.2 `order_number` Generation

A monotonic per-Store counter or a formatted timestamp+sequence produces
human-readable order numbers. The exact algorithm is defined by the implementing
unit (P5.4), but uniqueness is enforced by `UNIQUE (store_id, order_number)`.

---

## 19. Order Item Snapshot

Order Items are immutable purchase records. Catalog changes after the Order is
placed must not alter historical receipts.

### 19.1 `order_items` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `seller_listing_id` | `UUID` | Nullable FK → `seller_listings(id)` ON DELETE SET NULL |
| `product_id` | `UUID` | Nullable FK → `products(id)` ON DELETE SET NULL |
| `variant_id` | `UUID` | Nullable FK → `variants(id)` ON DELETE SET NULL |
| `sku_id` | `UUID` | Nullable FK → `skus(id)` ON DELETE SET NULL |
| `supplier_offer_id` | `UUID` | Nullable FK → `supplier_offers(id)` ON DELETE SET NULL; internal only |
| `fulfillment_location_id` | `UUID` | Nullable FK → `fulfillment_locations(id)` ON DELETE SET NULL; internal only |
| `inventory_reservation_id` | `UUID` | Nullable FK → `inventory_reservations(id)` ON DELETE SET NULL |
| `product_title_snapshot` | `TEXT` | NOT NULL; captured at checkout |
| `sku_code_snapshot` | `TEXT` | NOT NULL; captured at checkout |
| `unit_price_minor` | `BIGINT` | NOT NULL CHECK (unit_price_minor >= 0) |
| `currency_code` | `CHAR(3)` | NOT NULL FK → `currencies(code)` |
| `quantity` | `BIGINT` | NOT NULL CHECK (quantity > 0) |
| `line_total_minor` | `BIGINT` | NOT NULL CHECK (line_total_minor >= 0) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**All FK references to live catalog entities use `ON DELETE SET NULL`.** If a
Product or SKU is later deleted, the Order Item retains its snapshot data and the
reference becomes NULL — it does not cascade-delete the Order.

**Internal fields** (`supplier_offer_id`, `fulfillment_location_id`,
`inventory_reservation_id`) are NEVER exposed in Customer-facing DTOs.

---

## 20. Customer / Address Snapshot

`order_addresses` is a deep copy of the delivery address captured at Checkout
Session creation. It is not a FK to `customer_addresses`.

### 20.1 `order_addresses` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL UNIQUE FK → `orders(id)` ON DELETE RESTRICT |
| `address_type` | `TEXT` | NOT NULL CHECK IN (`shipping`) |
| `recipient_name` | `TEXT` | NOT NULL |
| `phone` | `TEXT` | Nullable |
| `address_line_1` | `TEXT` | NOT NULL |
| `address_line_2` | `TEXT` | Nullable |
| `city` | `TEXT` | NOT NULL |
| `region` | `TEXT` | Nullable |
| `postal_code` | `TEXT` | Nullable |
| `country_code` | `CHAR(2)` | NOT NULL |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Delete policy:** `ON DELETE RESTRICT` — order history must not disappear.

---

## 21. Order State Machine

### 21.1 Full Transition Matrix

| From | To | Authority | Precondition | Inventory Effect | Timeline Entry | Outbox Event |
|---|---|---|---|---|---|---|
| — | `pending` | Checkout | Validation passed | `held` (reserved_qty += qty) | ✓ | `commerce.order.created` |
| `pending` | `confirmed` | Seller | — | `consumed` (on_hand_qty -= qty, reserved_qty -= qty) | ✓ | `commerce.order.status_changed` |
| `pending` | `cancelled` | Customer OR Seller | — | `released` (reserved_qty -= qty) | ✓ | `commerce.order.status_changed` |
| `pending` | `cancelled` | Scheduler (expiry) | Confirmation deadline elapsed | `expired` (reserved_qty -= qty) | ✓ (reason=`confirmation_timeout`) | `commerce.order.status_changed` |
| `confirmed` | `processing` | Seller | — | None | ✓ | `commerce.order.status_changed` |
| `confirmed` | `cancelled` | Seller | — | Restock: `on_hand_qty += qty` (reservation remains `consumed`; new `inventory_movement` of type `order_cancellation_restock`) | ✓ | `commerce.order.status_changed` |
| `processing` | `ready_for_shipping` | Seller | — | None | ✓ | `commerce.order.status_changed` |
| `processing` | `cancelled` | Seller | — | Restock (same as `confirmed → cancelled`) | ✓ | `commerce.order.status_changed` |
| `ready_for_shipping` | `shipped` | **Phase 6** | Shipment created | Phase 6 defines | ✓ | Phase 6 defines |
| `shipped` | `out_for_delivery` | **Phase 6** | Provider event | Phase 6 defines | ✓ | Phase 6 defines |
| `out_for_delivery` | `delivered` | **Phase 6** | Provider event | Phase 6 defines | ✓ | Phase 6 defines |
| `delivered` | `returned` | **Future returns workflow** | Return request | Future defines | ✓ | Future defines |
| `cancelled` | — | — | Terminal | — | — | — |
| `returned` | — | — | Terminal | — | — | — |
| `delivered` | — | — | Terminal for normal flow | — | — | — |

**Phase 5 exposes NO endpoint to drive `ready_for_shipping → shipped` or
subsequent transitions.** These transitions are defined in the state model for
schema completeness and Phase 6 readiness, but require shipping provider authority
that does not exist until Phase 6.

**RETURNED** — owned by a future returns workflow. Phase 5 includes the status
in the schema CHECK constraint and transition table but activates no return
endpoints.

### 21.2 Invalid Transitions (must be deterministically rejected)

Any transition not listed in 21.1 above is forbidden. Examples:

- `delivered → pending`
- `cancelled → processing`
- `returned → shipped`
- `pending → processing` (skips confirmation)
- `confirmed → delivered` (skips intermediate states)

All state changes must increment `aggregate_version` transactionally.

---

## 22. Order Timeline

`order_timeline` is an append-only audit log. Mutation of existing rows is
forbidden (no `UPDATE`, no `DELETE`).

### 22.1 `order_timeline` Table Blueprint

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `from_status` | `TEXT` | Nullable (NULL for initial PENDING creation) |
| `to_status` | `TEXT` | NOT NULL |
| `actor_type` | `TEXT` | NOT NULL CHECK IN (`checkout`, `customer`, `seller`, `admin`, `scheduler`, `system`) |
| `actor_subject` | `TEXT` | Nullable; the authenticated principal subject if applicable; service identity for scheduler/system. Never tokens. |
| `reason` | `TEXT` | Nullable; machine-readable reason code (e.g. `confirmation_timeout`, `customer_requested`, `seller_cancelled`) |
| `metadata` | `JSONB` | Nullable; non-sensitive operational context |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

**Indexes:** `(order_id, created_at ASC)`.

---

## 23. Order Notes

`order_notes` supports Seller-internal notes only in Phase 5.

| Column | Type | Constraints |
|---|---|---|
| `id` | `UUID` | PK |
| `order_id` | `UUID` | NOT NULL FK → `orders(id)` ON DELETE RESTRICT |
| `author_subject` | `TEXT` | NOT NULL; the Seller member's ZITADEL subject |
| `visibility` | `TEXT` | NOT NULL DEFAULT `internal` CHECK IN (`internal`) |
| `body` | `TEXT` | NOT NULL |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT now() |

Customer-visible or Admin notes are deferred. Phase 5 only implements `internal`
visibility. Expanding visibility levels requires an explicit design pass.

---

## 24. Reservation Expiry Workflow

### 24.1 Trigger

When a `PENDING` Order's confirmation deadline elapses, a background scheduler
job must:

1. Query `orders WHERE status = 'pending' AND <confirmation deadline elapsed>`
   in bounded batches.
2. For each Order, atomically:

```
BEGIN
  SELECT orders WHERE id = $1 AND status = 'pending' FOR UPDATE
  SELECT inventory_reservations WHERE order_id related AND status = 'held' FOR UPDATE
  SELECT inventory_snapshots FOR UPDATE (ordered by id ASC)

  UPDATE orders SET status='cancelled', cancellation_reason='confirmation_timeout',
    aggregate_version = aggregate_version + 1, updated_at=now()

  FOR EACH reservation (status='held'):
    UPDATE inventory_reservations SET status='expired', updated_at=now()
    UPDATE inventory_snapshots SET reserved_qty -= quantity, updated_at=now()
    INSERT inventory_movements (movement_type='reservation_expired')

  INSERT order_timeline (actor_type='scheduler', reason='confirmation_timeout')
  INSERT outbox_events (commerce.order.status_changed, CANCELLED)
COMMIT
```

### 24.2 Idempotency

The `WHERE status = 'pending'` guard ensures exactly one worker wins a race.
`FOR UPDATE` serializes concurrent expiry + cancel attempts. If the Order is
already `cancelled` (by customer or seller), the scheduler finds no matching row
and exits cleanly.

### 24.3 Scheduler Implementation

Uses the existing Core scheduler foundation with:
- `FOR UPDATE SKIP LOCKED` for parallel scheduler safety.
- Bounded batch sizes (not unlimited queries).
- Transaction per individual Order expiry (not one transaction for the entire
  batch).
- Observable failure (metric count, structured log with correlation_id).
- No GoThrottle in the PostgreSQL transaction path.

---

## 25. Transactional Outbox

### 25.1 Invariant (ADR-006)

Business state and outbox event commit in the same PostgreSQL transaction. An
`OrderCreated` event that does not commit with the Order is never persisted. An
Order that commits always has a corresponding unpublished `outbox_events` row.

### 25.2 No New Outbox Table

Reuses the existing `outbox_events` table from migration `000001`.

### 25.3 Phase 5 Outbox Publisher Design (P5.7)

The publisher is a background worker loop:

```
LOOP:
  BEGIN
    SELECT * FROM outbox_events
    WHERE published_at IS NULL
    ORDER BY created_at ASC
    LIMIT $batch_size
    FOR UPDATE SKIP LOCKED
  COMMIT  ← release lock quickly

  FOR EACH event:
    Publish to RabbitMQ exchange with publisher confirms

    IF confirm received:
      UPDATE outbox_events SET published_at = now() WHERE event_id = $1

    IF broker error:
      log structured warning with event_id, correlation_id
      continue (will be retried on next poll)

SLEEP $poll_interval
```

**Critical crash scenario:**

If the process crashes after a successful RabbitMQ publish but before
`published_at` is written, the event will be re-published on the next poll cycle.
This is **expected at-least-once delivery**. Consumers must be idempotent. The
spec explicitly does NOT attempt fake exactly-once delivery.

**Publisher safety for multiple instances:**

`FOR UPDATE SKIP LOCKED` ensures no two publisher instances process the same
unpublished event simultaneously. Each instance claims its own subset.

**PostgreSQL locks are not held across the RabbitMQ network call.** The lock is
released (commit) before publishing. This means two instances may both read the
same unpublished event in theory if timing permits, but one of the resulting
`UPDATE … SET published_at` will be a no-op (already set). RabbitMQ will receive
a duplicate, which consumers handle via the inbox.

**No additional outbox columns required** for Phase 5. The existing
`published_at` NULL-means-unpublished sentinel is sufficient.

**Observability:**
- Metric: `outbox_unpublished_count` (gauge) — count of events WHERE published_at IS NULL.
- Metric: `outbox_oldest_unpublished_age_seconds` — age of the oldest unpublished event.
- Metric: `outbox_publish_failures_total` — broker publish failures.

---

## 26. Event Catalog

Phase 5 introduces the following Domain Events. All are emitted via the
Transactional Outbox.

### 26.1 `commerce.order.created` (schema_version: 1)

Emitted when a PENDING Order is created.

```json
{
  "order_id": "<uuid>",
  "order_number": "<string>",
  "store_id": "<uuid>",
  "market_code": "<string>",
  "customer_id": "<uuid|null>",
  "status": "pending",
  "currency_code": "<string>",
  "subtotal_minor": <integer>,
  "total_minor": <integer>,
  "item_count": <integer>,
  "occurred_at": "<rfc3339>"
}
```

### 26.2 `commerce.order.status_changed` (schema_version: 1)

Emitted on every allowed Order status transition.

```json
{
  "order_id": "<uuid>",
  "order_number": "<string>",
  "store_id": "<uuid>",
  "from_status": "<string|null>",
  "to_status": "<string>",
  "aggregate_version": <integer>,
  "reason": "<string|null>",
  "actor_type": "<string>",
  "occurred_at": "<rfc3339>"
}
```

### 26.3 Payload Rules

- Payloads are **privacy-safe**: no customer email, phone, address, or PII.
- No `supplier_id`, `supplier_offer_id`, or wholesale cost.
- No internal reservation or fulfillment location IDs.
- `event_type` uses the form `commerce.order.created`, `schema_version: 1`
  (not `commerce.order.created.v1` — the version is the `schema_version` field,
  not part of the event type string).

### 26.4 Events NOT introduced in Phase 5

- No `commerce.inventory.reservation_expired` as a public event (the expiry is
  handled internally by the scheduler; if downstream notification is needed,
  the `commerce.order.status_changed` event with `to_status: cancelled` and
  `reason: confirmation_timeout` carries that information).
- No notification or fulfillment events (no consuming service exists in Phase 5).

---

## 27. RabbitMQ Topology

### 27.1 Exchange

| Name | Type | Durable | Purpose |
|---|---|---|---|
| `commerce.events` | `topic` | Yes | All Phase 5 domain events |

### 27.2 Routing Keys

| Event Type | Routing Key |
|---|---|
| `commerce.order.created` | `order.created` |
| `commerce.order.status_changed` | `order.status_changed` |

### 27.3 Phase 5 Queues

Phase 5 initially publishes events to the exchange. No consuming queue is created
in Phase 5 unless a specific consuming service and business workflow is identified.

**DO NOT pre-create placeholder queues with no defined consumer.** Future
consumers bind their own queues to the exchange. The exchange is declared durable
so bindings can be added without restarting the publisher.

### 27.4 Reliability

- Durable exchange (persists across broker restarts).
- Publisher confirms required before `published_at` is written.
- Message persistence: `delivery_mode = 2` (persistent).
- No RabbitMQ RPC.
- No Kafka.

---

## 28. Consumer Inbox / Idempotency

### 28.1 Identity (ADR-007)

```
(consumer_name, event_id)
```

### 28.2 Atomic Processing Pattern

When a consumer performs a PostgreSQL side effect:

```
BEGIN
  INSERT INTO processed_events (consumer_name, event_id, processed_at)
  VALUES ($consumer, $event_id, now())
  ON CONFLICT (consumer_name, event_id) DO NOTHING

  IF rows_affected = 0:
    COMMIT  ← already processed; safe ACK
    ACK RabbitMQ
    return

  <perform business side effect within same transaction>
COMMIT
ACK RabbitMQ
```

The side effect and the processed-event marker are committed atomically. The
anti-pattern of "check processed → commit → perform side effect later" in two
separate transactions is forbidden.

### 28.3 Ordering Semantics

`processed_events` provides **duplicate suppression**, not **causal ordering**.

- `EventEnvelope` carries `aggregate_id` and `aggregate_version`.
- `orders.aggregate_version` monotonically increases on every state transition.
- Phase 5 does **not** implement global causal ordering enforcement.
- A consumer that requires ordered projection must own its own per-aggregate
  version state and define gap-handling logic. No generic ordering infrastructure
  is added until an actual consuming service requires it.
- The spec does not claim `processed_events` enforces ordering.

---

## 29. API Boundaries

### 29.1 Naming Conventions

- Seller Storefront public routes: `/v1/storefront/...` (served by
  `storefront-api`, anonymous).
- Seller Dashboard authenticated routes: `/v1/seller/...` (served by
  `seller-api`, requires OIDC JWT).
- Core internal routes: `/internal/v1/...` (authenticated by service token,
  `X-Matjero-Service: seller`).

### 29.2 Proposed Storefront API Routes (`storefront-api`)

```
POST   /v1/storefront/carts
         → Create guest Cart (returns cart_token cookie)

POST   /v1/storefront/carts/items
         → Add SKU to Cart (cart_token from cookie)

PATCH  /v1/storefront/carts/items/{item_id}
         → Update quantity

DELETE /v1/storefront/carts/items/{item_id}
         → Remove item

GET    /v1/storefront/carts
         → View Cart (cart_token from cookie)

POST   /v1/storefront/checkout/sessions
         → Create Checkout Session (validate Cart, snapshot address)

POST   /v1/storefront/checkout/sessions/{session_id}/finalize
         → Atomic Order creation
         → Body: { fingerprint, customer details for guest }
         → Returns: Order summary (no internal supplier data)
```

Cart identity comes from `cart_token` in an `HttpOnly` cookie.
Store identity comes from `X-Matjero-Storefront-Host`.
No `store_id` or `seller_id` is accepted from the request body.

### 29.3 Proposed Seller Dashboard API Routes (`seller-api`)

```
GET    /v1/seller/stores/{store_id}/orders
         → List Orders with filtering

GET    /v1/seller/stores/{store_id}/orders/{order_id}
         → Order detail with Order Items (includes internal source data)

POST   /v1/seller/stores/{store_id}/orders/{order_id}/confirm
         → Transition PENDING → CONFIRMED

POST   /v1/seller/stores/{store_id}/orders/{order_id}/process
         → Transition CONFIRMED → PROCESSING

POST   /v1/seller/stores/{store_id}/orders/{order_id}/ready
         → Transition PROCESSING → READY_FOR_SHIPPING

POST   /v1/seller/stores/{store_id}/orders/{order_id}/cancel
         → Cancel (PENDING / CONFIRMED / PROCESSING only)

POST   /v1/seller/stores/{store_id}/orders/{order_id}/notes
         → Add internal note
```

### 29.4 Core Internal Routes (`core-api`)

All under `/internal/v1/`, authenticated by `X-Matjero-Service: seller` bearer
token. Host-derived store context validated by Core.

```
POST   /internal/v1/carts
POST   /internal/v1/carts/items
PATCH  /internal/v1/carts/items/{item_id}
DELETE /internal/v1/carts/items/{item_id}
GET    /internal/v1/carts                    (by cart_token_digest + store from host)

POST   /internal/v1/checkout-sessions
POST   /internal/v1/checkout-sessions/{session_id}/finalize

GET    /internal/v1/stores/{store_id}/orders
GET    /internal/v1/stores/{store_id}/orders/{order_id}
POST   /internal/v1/stores/{store_id}/orders/{order_id}/confirm
POST   /internal/v1/stores/{store_id}/orders/{order_id}/process
POST   /internal/v1/stores/{store_id}/orders/{order_id}/ready
POST   /internal/v1/stores/{store_id}/orders/{order_id}/cancel
POST   /internal/v1/stores/{store_id}/orders/{order_id}/notes
```

Core validates `store_id` against the resolved Store (from `X-Matjero-Storefront-Host` for storefront paths, or from the seller principal for dashboard paths).

---

## 30. Error Contract

| Code | HTTP Status | Meaning |
|---|---|---|
| `not_found` | 404 | Resource does not exist or is not accessible to this caller |
| `unauthorized` | 401 | Missing or invalid authentication |
| `forbidden` | 403 | Authenticated but not authorized for this resource |
| `invalid_argument` | 400 | Malformed request, missing required field |
| `validation_error` | 422 | Structurally valid but semantically invalid |
| `market_mismatch` | 422 | Store/Listing/SKU market codes do not match |
| `insufficient_inventory` | 409 | Not enough available stock |
| `price_changed` | 409 | Listing price changed since Cart snapshot; includes current price |
| `listing_unavailable` | 409 | Listing is not active at checkout time |
| `cart_expired` | 409 | Cart or Checkout Session has expired |
| `checkout_expired` | 409 | Checkout Session has expired |
| `idempotency_conflict` | 409 | Same idempotency identity with a different payload |
| `invalid_order_transition` | 409 | Requested state transition is not permitted |
| `conflict` | 409 | General conflict (e.g., concurrent finalization) |
| `internal_error` | 500 | Unexpected server error |

**Internal IDs, supplier identities, reservation tokens, and wholesale costs must
never appear in error messages returned to the Storefront.**

---

## 31. Security and Privacy

### 31.1 Threat Mitigations

| Threat | Mitigation |
|---|---|
| Cross-Store Cart IDOR | Store derived from trusted host only; cart_token validated within that Store |
| Cross-Store Order IDOR | `store_id` from authenticated principal; Core validates ownership |
| Cart token guessing | High-entropy token (32 random bytes); stored as one-way digest |
| Cart token in logs | Logging middleware must mask cookie values |
| Checkout Session token guessing | UUID v4 (session id); no sequential IDs |
| Order enumeration | UUIDs; no sequential global order IDs exposed to customers |
| Idempotency-key abuse | Scoped to `(session_id, store_id)`; fingerprint mismatch returns conflict |
| Quantity overflow | `CHECK (quantity > 0 AND quantity <= 10000)`; overflow check before multiply |
| Money overflow | All arithmetic checked for int64 overflow before committing |
| Price tampering | Browser totals never trusted; Core re-reads authoritative prices |
| store_id/seller_id tampering | Client-supplied identifiers stripped; host/JWT authority only |
| SKU/Listing mismatch | Core validates SKU → Variant → Product == Listing.product_id |
| Source Supplier spoofing | `supplier_offer_id` never accepted from client; derived from Listing |
| Reservation replay | Reservation `held → consumed/released/expired` uses `WHERE status = 'held'`; terminal states cannot retransition |
| Double checkout | `FOR UPDATE` on Checkout Session + `status = 'finalizing'` fence |
| Race-to-last-stock | `SELECT FOR UPDATE` + `CHECK (reserved_qty <= on_hand_qty)` |
| Invalid Order transitions | State machine enforced in application code + `status` CHECK constraint |
| Internal Supplier data leakage | Storefront DTOs actively exclude `supplier_offer_id`, `fulfillment_location_id`, wholesale costs |
| Customer PII in logs | Structured logging middleware must mask email, phone, address fields |
| Negative quantities | `CHECK (quantity > 0)` on `cart_items`, `order_items`, `inventory_reservations` |

### 31.2 Guest Order Access

Guest Customers cannot retrieve their Order history by guessing a UUID. Post-checkout
Order lookup for guests is **deferred** to a future phase, which must design a
secure access-token mechanism separate from the cart_token.

---

## 32. Observability

### 32.1 Metrics

| Metric | Type | Labels |
|---|---|---|
| `checkout_attempts_total` | Counter | `store_id`, `result` (success/failure) |
| `checkout_failures_total` | Counter | `store_id`, `reason` (price_changed/insufficient_inventory/...) |
| `order_created_total` | Counter | `store_id`, `market_code` |
| `inventory_reservation_conflicts_total` | Counter | `store_id`, `sku_id` |
| `reservation_expiry_processed_total` | Counter | `result` (expired/already_terminal) |
| `order_status_transition_total` | Counter | `from_status`, `to_status`, `actor_type` |
| `invalid_order_transition_total` | Counter | `from_status`, `attempted_to` |
| `outbox_unpublished_count` | Gauge | — |
| `outbox_oldest_unpublished_age_seconds` | Gauge | — |
| `outbox_publish_failures_total` | Counter | `event_type` |
| `consumer_duplicate_events_total` | Counter | `consumer_name` |
| `cart_created_total` | Counter | `store_id` |
| `cart_item_add_total` | Counter | `store_id`, `result` |

### 32.2 Logging Rules

- Structured JSON format.
- Every request carries `correlation_id` (propagated from `X-Request-ID`).
- Order operations include `order_id` and `store_id` (internal identifiers are
  acceptable in logs; they are UUIDs, not customer PII).
- **Never log:** `cart_token`, `cart_token_digest`, reservation tokens, customer
  email, phone, address, or service tokens.

### 32.3 Correlation ID Propagation

- HTTP request → `correlation_id` header (generated or forwarded from `X-Request-ID`).
- Checkout transaction → `outbox_events.correlation_id` carries the request correlation ID.
- Derived async events → `causation_id` points to the triggering `event_id`.

---

## 33. Database Migration Plan

Current highest migration: `000009`. Phase 5 proposes migrations `000010`–`000015`.
Exact numbering is assigned during implementation; this section defines the
**content and sequence** of each migration, not the numbers.

### Migration A — Fulfillment Location Ownership Extension

Alter `fulfillment_locations`:
- Make `supplier_id` and `supplier_market_id` nullable.
- Add `store_id UUID NULLABLE REFERENCES stores(id) ON DELETE RESTRICT`.
- Add ownership CHECK constraint (Section 16.2).
- Add Seller-branch FK `(store_id, market_code) REFERENCES stores(id, market_code)`.
- Add partial unique indexes for location code by branch.

### Migration B — Customer and Address Aggregates

Create:
- `customers` (Section 7.3)
- `customer_addresses` (Section 7.4)

### Migration C — Cart Aggregates

Create:
- `carts` (Section 8.4)
- `cart_items` (Section 8.5)

### Migration D — Checkout Session

Create:
- `checkout_sessions` (Section 10.3)

### Migration E — Order Aggregates

Create:
- `orders` (Section 18.1)
- `order_items` (Section 19.1)
- `order_addresses` (Section 20.1)
- `order_timeline` (Section 22.1)
- `order_notes` (Section 23)

### Migration F — Inventory Reservation Linkage

Alter `inventory_reservations`:
- Add `checkout_session_id UUID NULLABLE REFERENCES checkout_sessions(id) ON DELETE SET NULL`.
- Add `order_item_id UUID NULLABLE REFERENCES order_items(id) ON DELETE SET NULL`.

---

## 34. OpenAPI Plan

Phase 5 spec changes no existing generated OpenAPI documents. Implementation
units will generate updated specs during their own PRs:

| Document | Location | Updated By |
|---|---|---|
| Core internal API | `core/docs/api/internal/openapi.json` | P5.3, P5.4, P5.5, P5.6 implementing units |
| Seller Storefront API | `seller/docs/api/storefront/openapi.json` | P5.8 implementing unit |
| Seller Dashboard API | `seller/docs/api/seller/openapi.json` | P5.9 implementing unit |

Each repository's CI already verifies that the generated OpenAPI document matches
what is committed, preventing stale specs from merging.

---

## 35. Rollout / Feature Flag Strategy

Phase 5 capability gates should be **capability-named**, not phase-number named.
Runtime feature flags live in environment configuration, not in code as permanent
constants.

| Flag | Controls |
|---|---|
| `STOREFRONT_CHECKOUT_ENABLED` | Enables cart/checkout/order customer routes in `storefront-api` |
| `SELLER_ORDER_MANAGEMENT_ENABLED` | Enables order management routes in `seller-api` |

Until `STOREFRONT_CHECKOUT_ENABLED` is enabled, the storefront-api must return
`404` or `503` for all cart and checkout routes. Partial schema/API work must
never be user-visible unintentionally.

Core internal routes may be enabled progressively (Core has no end-user flag
requirement). The gate is applied at the actor boundary.

---

## 36. Testing Strategy

### 36.1 Inventory Concurrency Tests (deterministic, no sleep-based races)

1. **Last unit:** 20+ concurrent goroutines attempt to checkout 1 unit of the
   same SKU. Exactly one `201 Created`; all others receive `insufficient_inventory`.
2. **Two units:** N concurrent checkouts for a 2-unit stock; exactly 2 succeed.
3. **Multi-item rollback:** Item A has stock; Item B does not. All goroutines
   receive `insufficient_inventory`; zero reservations persist after rollback.
4. **Same SKU at multiple locations:** Ensure location selection is deterministic
   and total allocations never exceed combined stock.
5. **Supplier source isolation:** A Seller-owned listing cannot draw inventory
   from a Supplier-owned fulfillment location, and vice versa.
6. **Market isolation:** Attempt cross-market inventory allocation; assert
   rejection.

### 36.2 Idempotency Tests

7. **Concurrent identical finalize:** 10 concurrent finalizations of the same
   Checkout Session. Exactly 1 Order created.
8. **Retry after success:** Same `session_id` + same `fingerprint`. Same Order
   returned.
9. **Same session, different fingerprint:** `idempotency_conflict`.

### 36.3 Race Condition Tests

10. **Confirm vs expiry:** Concurrent Seller confirm + scheduler expiry for the
    same PENDING Order. Exactly one wins; inventory is affected exactly once.
11. **Cancel vs expiry:** Concurrent customer cancel + scheduler expiry. Exactly
    one wins.
12. **Confirm vs cancel:** Concurrent. Exactly one wins.
13. **Cancellation retry:** Cancel of an already-CANCELLED Order is idempotent
    (no double inventory release).
14. **Expiry retry:** Expiry attempted on an already-expired reservation. No
    double `reserved_qty` decrement.
15. **Concurrent cancel + expiry:** Both attempt `WHERE status = 'held'`.
    Exactly one succeeds; `reserved_qty` adjusted exactly once.

### 36.4 Post-confirm Cancellation / Restock

16. Cancel a CONFIRMED Order; assert `on_hand_qty` restores and reservation
    remains `consumed` (no status change to reservation).
17. Cancel a PROCESSING Order; same assertion.
18. Attempt cancel from `READY_FOR_SHIPPING`; assert `invalid_order_transition`.

### 36.5 History Immutability Tests

19. Change Seller Listing price after Order. Assert `order_item.unit_price_minor`
    unchanged.
20. Rename Product after Order. Assert `order_item.product_title_snapshot`
    unchanged.
21. Change SKU code after Order. Assert `order_item.sku_code_snapshot` unchanged.
22. Customer edits address after Order. Assert `order_addresses` snapshot
    unchanged.
23. Supplier changes wholesale price after Order. Assert no customer Order field
    changes.

### 36.6 Outbox / Inbox Event Tests

24. Business commit + outbox event are atomic: asserting both exist or neither
    exists after commit/rollback.
25. Business rollback → no `outbox_events` row persisted.
26. Publisher confirm → `published_at` set.
27. Broker failure → event remains with `published_at = NULL`; retried on next
    poll.
28. Publish succeeds then process crashes before `published_at` → duplicate
    publish possible → consumer's `processed_events` check prevents double
    side-effect.
29. Duplicate event delivery (same `event_id`) to same consumer → side effect
    executes exactly once.
30. Same `event_id` delivered to two different consumer names → each processes
    once independently.

### 36.7 Order State Machine Tests

Table-driven tests covering:

- Every **allowed** transition (Section 21.1) → assert success + timeline entry +
  outbox event.
- Every **invalid** transition → assert `invalid_order_transition` error + no
  state change + no timeline entry.
- Every transition increments `aggregate_version`.

Specific invalid transition assertions:
- `delivered → pending` → rejected
- `cancelled → processing` → rejected
- `returned → shipped` → rejected
- `pending → processing` (skips confirm) → rejected
- `confirmed → delivered` (skips intermediate) → rejected

---

## 37. Repository Impact Matrix

| Repository | Impact Level | Phase 5 Work |
|---|---|---|
| **core** (`matjeroapps/core`) | **High** | All schema migrations; Customer/Cart/Checkout/Order domain; Inventory reservation lifecycle; Atomic checkout transaction; Outbox wiring; Scheduler expiry job; Core internal API routes |
| **seller** (`matjeroapps/seller`) | **High** | `storefront-api`: Cart and Checkout routes; `seller-api`: Order management routes; `coreclient` extended for new Core capabilities |
| **seller-hub** (`matjeroapps/seller-hub`) | **Medium** | `storefront/`: Cart, Checkout UI flow in Next.js; `seller/`: Order list, detail, confirm, cancel UI in React |
| **admin** (`matjeroapps/admin`) | **None** | No Phase 5 runtime work |
| **supplier** (`matjeroapps/supplier`) | **None** | No Phase 5 runtime work |
| **supplier-hub** (`matjeroapps/supplier-hub`) | **None** | README-only, no Phase 5 work |

---

## 38. Implementation Unit Breakdown

### P5.0 — Phase Specification *(this document)*

**Repository:** core · **Branch:** feature/p5-phase-spec

---

### P5.1 — Customer + Cart Core Domain

**Repository:** core
**Dependencies:** P5.0 merged
**Branch:** feature/p5.1-customer-cart-domain

**Backend Work:**
- Migrations A, B, C (fulfillment_locations ownership, customers, carts, cart_items)
- Domain types: `Customer`, `Cart`, `CartItem`
- Repository: create/get/update cart, add/remove/update cart items
- Service: Cart validation (SKU→Listing invariant, market check)
- Core internal HTTP routes: `POST /carts`, `POST /carts/items`, etc.
- Storefront host security: `X-Matjero-Storefront-Host` for all cart routes

**Events:** None (cart operations are not event-sourced in Phase 5)
**Tests:** Unit + integration; cart IDOR tests; SKU/Listing invariant tests

---

### P5.2 — Checkout Session + Durable Idempotency

**Repository:** core
**Dependencies:** P5.1 merged
**Branch:** feature/p5.2-checkout-session

**Backend Work:**
- Migration D (checkout_sessions)
- Domain types: `CheckoutSession`
- Repository: create/get/finalize checkout session
- Idempotency: `SELECT FOR UPDATE` finalization fence
- Price snapshot comparison logic
- Core internal HTTP routes: `POST /checkout-sessions`, `POST /checkout-sessions/{id}/finalize` (validation only at this stage, no Order creation)

**Tests:** Idempotency unit tests; concurrent finalize tests

---

### P5.3 — Order Aggregate + Immutable Snapshots + State Machine

**Repository:** core
**Dependencies:** P5.1 merged (customers, checkout_sessions tables exist)
**Branch:** feature/p5.3-order-aggregate

**Backend Work:**
- Migration E (orders, order_items, order_addresses, order_timeline, order_notes)
- Domain types: `Order`, `OrderItem`, `OrderAddress`, `OrderTimeline`, `OrderNote`
- State machine: all transitions, invalid transition rejection
- Repository: create/get/transition order, append timeline, add note
- `aggregate_version` increment logic
- Core internal HTTP routes: `GET /stores/{store_id}/orders`, `GET .../orders/{id}`, status transition routes
- Seller order management business logic (confirm, process, ready, cancel)

**Tests:** State machine table-driven tests; history immutability tests

---

### P5.4 — Inventory Reservation Lifecycle + Allocation + Expiry

**Repository:** core
**Dependencies:** P5.3 merged (order_items table exists for linkage), P5.1 (fulfillment_location changes)
**Branch:** feature/p5.4-inventory-lifecycle

**Backend Work:**
- Migration F (inventory_reservations linkage columns)
- Reservation state machine: `held → consumed/released/expired`
- Fulfillment location allocation algorithm (Section 16.4)
- Inventory movement recording for all reservation transitions
- Cancellation restock logic (Section 21.1)
- Expiry scheduler job (Section 24)
- Idempotent reservation transitions using `WHERE status = 'held'`

**Tests:** Concurrency tests (last-unit, two-units, multi-item rollback, supplier/seller source isolation); reservation idempotency tests; expiry + cancel race tests

---

### P5.5 — Atomic Checkout Transaction + Transactional Outbox Enqueue

**Repository:** core
**Dependencies:** P5.2 + P5.3 + P5.4 all merged
**Branch:** feature/p5.5-atomic-checkout

**Backend Work:**
- Full atomic checkout transaction (Section 17)
- Outbox event enqueue for `commerce.order.created` inside checkout transaction
- Price revalidation and `price_changed` response
- Multi-item rollback enforcement
- Integration of lock ordering (Section 15.2)
- End-to-end `POST /checkout-sessions/{id}/finalize` completing with Order creation

**Tests:** All Phase 5 concurrency tests; price_changed test; full transaction atomicity tests

---

### P5.6 — Outbox Publisher + Event Delivery Reliability

**Repository:** core
**Dependencies:** P5.5 merged
**Branch:** feature/p5.6-outbox-publisher

**Backend Work:**
- Outbox publisher worker with `FOR UPDATE SKIP LOCKED`, bounded batch, poll interval
- Publisher confirms → `published_at`
- `commerce.order.status_changed` outbox enqueue on all order transitions
- Exchange declaration: `commerce.events` (topic, durable)
- Routing key mapping (Section 27.2)
- Observability metrics (outbox gauge, publish failures)

**Tests:** Outbox/inbox event tests (Section 36.6); publisher crash-recovery test

---

### P5.7 — Storefront API + Storefront Web Cart / Checkout

**Repository:** seller (API) + seller-hub (web/storefront)
**Dependencies:** P5.5 merged (Core checkout routes ready)
**Branch:** feature/p5.7-storefront-cart-checkout

**Backend Work (`seller` repo):**
- Extend `coreclient` for Cart and Checkout capabilities
- `storefront-api`: Cart routes under `/v1/storefront/carts`
- `storefront-api`: Checkout Session routes under `/v1/storefront/checkout`
- `cart_token` cookie management (HttpOnly, Secure, SameSite=Strict)
- Storefront OpenAPI update (`docs/api/storefront/openapi.json`)

**Frontend Work (`seller-hub/storefront`):**
- Add-to-cart UI
- Cart sidebar/drawer
- Checkout flow (address entry, order review, finalize)
- Price-changed error handling and review UX
- Post-checkout Order confirmation page

**Tests:** Stub Core server tests (no real DB); IDOR test (cross-store cart token)

---

### P5.8 — Seller API + Seller Web Order Management

**Repository:** seller (API) + seller-hub (web/seller)
**Dependencies:** P5.3 merged (Core Order routes ready)
**Branch:** feature/p5.8-seller-order-management

**Backend Work (`seller` repo):**
- Extend `coreclient` for Order management capabilities
- `seller-api`: Order routes under `/v1/seller/stores/{store_id}/orders`
- Seller OpenAPI update (`docs/api/seller/openapi.json`)

**Frontend Work (`seller-hub/seller`):**
- Order list with filtering
- Order detail page
- Confirm / Process / Cancel actions
- Internal note capability

**Tests:** Stub Core server tests; seller IDOR tests (cannot access another seller's orders)

---

### P5.9 — Concurrency / Security / Multi-Tenant E2E Hardening

**Repository:** core (primarily) + seller
**Dependencies:** P5.7 + P5.8 merged
**Branch:** feature/p5.9-e2e-hardening

**Work:**
- Full concurrency test suite execution (Section 36.1-36.5)
- Security audit: all threat mitigations verified (Section 31.1)
- Multi-tenant invariant tests (cross-store IDOR, market mismatch)
- Numeric overflow edge case tests
- Cart merge race tests
- Seller storefront flag (`STOREFRONT_CHECKOUT_ENABLED`) integration

---

### P5.10 — Phase Completion

**Repository:** core
**Dependencies:** P5.9 merged; all CI green
**Branch:** feature/p5.10-phase-completion

**Work:**
- Phase 5 completion report
- Final cross-repository audit
- Definition of Done verification (Section 39)
- Documentation updates (master-plan cross-reference)

---

## 39. Dependency Graph

```
P5.0 (merged)
  │
  ├── P5.1 (Customer + Cart Domain)
  │     │
  │     ├── P5.2 (Checkout Session + Idempotency)
  │     │     │
  │     │     └── [awaits P5.3]
  │     │
  │     └── P5.3 (Order Aggregate + State Machine)
  │           │
  │           ├── P5.4 (Inventory Lifecycle + Expiry)
  │           │     │
  │           │     └── P5.5 (Atomic Checkout) ──────┐
  │           │                                       │
  │           └── [awaits P5.4]                       │
  │                                                   │
  └── [P5.2 + P5.3 + P5.4 merge gate]                │
                                                      │
                                               P5.5 merged
                                                      │
                                               P5.6 (Outbox Publisher)
                                                      │
                                         P5.6 merged gate
                                                    / \
                                                   /   \
                                           P5.7        P5.8
                                     (Storefront)   (Seller Mgmt)
                                           │              │
                                           └──────┬───────┘
                                                  │
                                            P5.9 (Hardening)
                                                  │
                                            P5.10 (Completion)
```

**Critical merge gate rule (ADR-017):** A seller-repo PR must not merge against
Core contracts that exist only on an unmerged Core branch. The Core contract must
merge first.

---

## 40. Known Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Overselling under high concurrency | High | `SELECT FOR UPDATE` + `CHECK (reserved_qty <= on_hand_qty)` |
| Duplicate Orders on retry | High | Checkout Session finalization fence + `UNIQUE (order_id)` constraint |
| Stale prices charged | High | Authoritative price re-read + `price_changed` rejection inside transaction |
| Cross-tenant IDOR (Cart/Order) | High | Host-derived tenant; UUID PKs; `store_id` validation on all routes |
| Source Supplier drift post-Order | High | `supplier_offer_id` immutably captured in `order_items` snapshot |
| Reservation leakage (stranded held reservations) | High | Confirmation deadline expiry job (Section 24) |
| Reservation deadlocks | Medium | Deterministic `id ASC` lock order; bounded transaction scope |
| Outbox backlog (broker unavailable) | Medium | `outbox_unpublished_count` metric + alerting; at-least-once recovery |
| Duplicate RabbitMQ delivery | Medium | Consumer Inbox idempotency (`processed_events`) |
| Event ordering violations | Low (initial) | `aggregate_version` in envelope; ordering enforcement deferred until an actual consumer requires it |
| Partial checkout commits (crash between fence and main tx) | Medium | Fencing tx is minimal; main tx is a single block; crash leaves session in `'finalizing'` which is detectable |
| Order history mutability | High | `ON DELETE RESTRICT` on Order FK chains; SET NULL on catalog FKs |
| Large Cart / numeric overflow | Medium | Quantity bounds; int64 overflow checks before multiply |
| Customer PII exposure | High | Storefront DTOs sanitized; logging masks PII fields |
| Guest token security | High | HttpOnly + Secure + SameSite=Strict cookie; never in URL or logs |
| Future Shipping/Payment compatibility | Low | Order state machine includes all states; Phase 6/7 transitions defined but not activated |
| `'finalizing'` stuck sessions (crash between fence commit and main tx) | Medium | Checkout Session expiry; `finalizing` sessions older than a threshold are reset to `'failed'` by a cleanup job |

---

## 41. Deferred Work

The following are explicitly out of Phase 5 scope:

- **Phase 6:** Shipping provider integration, shipment creation, label generation,
  tracking webhooks, split shipment, `SHIPPED / OUT_FOR_DELIVERY / DELIVERED`
  state activation.
- **Phase 7:** Payment gateway integration, payment authorization, capture,
  refund, COD flow, `PaymentCaptured` event.
- **Phase 8+:** Financial ledger, seller settlements, payouts.
- **Guest Order lookup** — requires secure order-access token design.
- **Cart merge UI** — design for merged cart items on login.
- **Customer account portal** — order history for authenticated customers.
- **Promotions, coupons, loyalty** — no discount engine.
- **Tax calculation** — no tax engine.
- **Supplier-facing order notifications** — no Supplier API surface in Phase 5.
- **Shared Seller inventory across multiple Stores** — deferred.
- **Returns workflow** — `RETURNED` state defined but not activated.
- **`SupplierOfferSKU` mapping** — if the Product-level offer rule proves
  insufficient for a future business requirement, a new ADR and mapping table are
  required. Phase 5 proceeds with the Product-level rule (Section 9.2).

---

## 42. Phase 5 Definition of Done

- [ ] P5.0 specification merged to Core main.
- [ ] P5.1–P5.10 all implementation units merged.
- [ ] All concurrency tests passing in CI (Section 36.1–36.3).
- [ ] All history immutability tests passing (Section 36.5).
- [ ] All outbox/inbox tests passing (Section 36.6).
- [ ] All Order state machine tests passing (Section 36.7).
- [ ] Storefront can add an item to a Cart, complete Checkout, and receive a
  PENDING Order confirmation.
- [ ] Seller Dashboard can list Orders, confirm an Order, and cancel a PENDING
  Order.
- [ ] Inventory `reserved_qty` is correctly incremented on checkout and
  decremented on confirmation or cancellation.
- [ ] `outbox_events` rows are published to `commerce.events` exchange.
- [ ] Zero cross-repository compile-time dependencies introduced.
- [ ] `STOREFRONT_CHECKOUT_ENABLED` and `SELLER_ORDER_MANAGEMENT_ENABLED` flags
  control live traffic exposure.
- [ ] Security audit: all threat mitigations in Section 31.1 verified.
- [ ] Phase 5 completion report published.

---

## 43. Self-Review: Architecture Correctness Assertions

| Question | Answer | Enforcement |
|---|---|---|
| Can a PENDING Order remain after reservation expiry? | **NO** | Scheduler job: expiry → Order CANCELLED + reservation expired atomically |
| Can a consumed reservation be released? | **NO** | Terminal state; `WHERE status = 'held'` guard |
| Can cancel-after-confirm restore stock twice? | **NO** | Cancel-CONFIRMED performs restock; reservation stays `consumed`; restock is idempotent via `inventory_movements` record check |
| Can concurrent finalization create two Orders? | **NO** | `status = 'finalizing'` fence + `UNIQUE (order_id)` |
| Can a Supplier Offer source an unrelated SKU? | **NO** | Checkout validates SKU → Product == Listing.product_id == SupplierOffer.product_id |
| Can Seller-owned inventory cross Store/Market? | **NO** | `fulfillment_locations.store_id` FK + market FK |
| Can a customer checkout bypass storefront-api? | **NO** | No public Core routes exist; Core is internal-only |
| Can shipping/payment functionality enter Phase 5? | **NO** | Non-Goal; no related schema, routes, or events added |
| Can duplicate RabbitMQ delivery duplicate a DB side effect? | **NO** | Consumer Inbox idempotency: `processed_events` + atomic side-effect commit |
| Can the Outbox lose an OrderCreated event after Order commit? | **NO** | ADR-006: outbox event and Order commit in same PostgreSQL transaction |
