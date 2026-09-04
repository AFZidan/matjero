# Matjero — Phase 5: Cart, Checkout, Orders and Inventory Transactions

## 1. Executive Summary
Phase 5 introduces the core transactional commerce loop for the Matjero platform. It implements Cart management, the Checkout process, Order creation, and authoritative Inventory Reservations. This phase transitions the system from a catalog browsing platform to a transactional marketplace, maintaining strict consistency, data integrity, and strict boundaries across multiple tenants (Sellers, Stores, and Suppliers).

## 2. Goals
- Implement the Cart and Checkout Session aggregates.
- Define a strictly consistent atomic checkout transaction boundary.
- Integrate existing Inventory Snapshots with durable Inventory Reservations to prevent overselling.
- Establish the Order aggregate and Order State Machine.
- Activate the Transactional Outbox and Consumer Inbox patterns for event delivery.
- Provide APIs for customer storefront interactions and seller order management.
- Update the schema to support Seller-owned fulfillment locations alongside Supplier-owned ones.

## 3. Non-Goals
- **Shipping Provider Integration:** (Phase 6 scope) Rate calculation, label generation, tracking.
- **Payment Gateway Integration:** (Phase 7 scope) Credit card processing, refunds, provider webhooks.
- **Complex Promotions:** Coupons, loyalty points, tax calculation engines.
- **Multi-Store Checkout:** Carts remain isolated to a single Store.
- **Cross-Repository Code Sharing:** Seller and Core must remain strictly independent at compile-time.
- **New Messaging Backbone:** No Kafka; RabbitMQ remains the sole asynchronous broker.

## 4. Existing Phase 4 / Commerce Baseline
Phase 4 delivered the multi-tenant catalog, market isolation, and Supplier Retail Capability.
- `inventory_snapshots` tracks `on_hand_qty` and `reserved_qty` per SKU + Fulfillment Location.
- `inventory_reservations` exists to block allocations for a `reservation_token` and `quantity`.
- `seller_listings` bridges Stores, Products, and optional `supplier_offer_id` source points.
- Core acts as the sole PostgreSQL authority; Actors (Seller, Admin, Supplier) use HTTP APIs to communicate with Core.
- RabbitMQ is established as the async backbone (ADR-018), and the Transactional Outbox/Consumer Inbox schema is present but unused.

## 5. Business Decisions
- **Customer Identity:** Core maintains a lightweight `customers` table to track guest and authenticated shoppers per Store. It stores an external identity reference (e.g., OIDC subject) for authenticated users but handles NO password hashes.
- **Cart Identity:** Carts are identified by a secure guest token (UUID). If a user is authenticated, the Cart is also linked to `(customer_id, store_id)`.
- **Order Lifecycle:** Checkout creates a `PENDING` Order. The reservation is held `ACTIVE` with a bounded deadline. Seller confirmation shifts the Order to `CONFIRMED` and `CONSUMES` the reservation.
- **Order Cancellation:** Customers may cancel only `PENDING` Orders. Sellers may cancel `PENDING`, `CONFIRMED`, or optionally `PROCESSING` (if fulfillment hasn't begun). Cancellation idempotently releases the inventory reservation.
- **Seller-Owned Inventory:** The `fulfillment_locations` schema will be updated in Phase 5 to support `seller_id` and `store_id`, enabling Seller-owned physical stock.

## 6. Repository Boundaries
- **Core (core-api):** Owns the checkout transaction, order invariants, inventory reservations, the PostgreSQL database, and event publishing.
- **Seller (seller-api):** Acts as the storefront gateway for customer API calls (validating store context) and provides the Dashboard UI to manage Orders.
- **Supplier (supplier-api):** Will eventually receive wholesale fulfillment events, though Phase 5 requires minimal changes here.
- **No Compile-Time Coupling:** As per ADR-017, Seller continues to interact with Core via authenticated HTTP calls (`/internal/v1/...`).

## 7. Customer Model
Phase 5 introduces `customers` to track the purchasing entity.
- Scope: Isolated per Store. `(id, store_id)`
- Authenticated Users: Linked via `identity_subject` to an external Auth provider (not ZITADEL, which remains for platform actors).
- Guest Users: Captured via email during checkout.
- Customer Addresses: Saved directly to the customer profile for reuse, but Orders take an immutable snapshot.

## 8. Cart Aggregate
A Cart is a temporary collection of intended purchases.
- **Identity:** `cart_token` (UUID).
- **Scope:** Bound strictly to a single `store_id` and `market_code`.
- **Items:** `cart_items` referencing `sku_id` and `seller_listing_id`.
- **Lifecycle:** Expirable, merged upon login, and deactivated upon successful checkout.
- Carts DO NOT calculate final prices internally; they rely on Core authoritative logic at checkout.

## 9. SKU / Listing / Source Semantics
Customers browse Listings (Product level) but select Variants/SKUs (Inventory level).
- **Cart Item → SKU:** The explicit SKU selected by the customer.
- **Cart Item → Seller Listing:** Defines the selling price.
- **Seller Listing → Supplier Offer (optional):** Defines the source of the goods if dropshipped.
- **Fulfillment Location:** Determined at checkout based on the SKU and source. The resulting Order Item locks an immutable snapshot of this source lineage.

## 10. Checkout Session
Isolates temporary checkout progression from the durable Cart and Order.
- Represents the intent to pay and finalize.
- Stores selected shipping address, contact details, and chosen rates (stubbed for P5).
- Serves as the primary idempotency token for order creation.
- If checkout fails (e.g., inventory shortage), the Checkout Session records the validation failure, allowing the frontend to gracefully guide the user.

## 11. Price Validation
The price displayed in the Cart is a cached snapshot or client estimation.
- During checkout, Core re-reads the authoritative `seller_listing_prices`.
- If the current listing price differs from the expected snapshot, the checkout **REJECTS** the transaction with a `price_changed` error, forcing the customer to review the new total.
- Wholesale supplier costs are strictly internal and never exposed to the storefront DTO.

## 12. Durable Idempotency
- **Identity:** `idempotency_key` (typically the Checkout Session ID or a unique request UUID) scoped to the `store_id`.
- **Mechanism:** A unique constraint in PostgreSQL linking the `idempotency_key` to the resulting `order_id`.
- **Behavior:** Concurrent identical requests yield exactly one Order. Subsequent identical requests return the already-created Order. Differing payloads with the same key yield a `conflict` error.

## 13. Inventory Model
Reuses the existing `inventory_snapshots` and `inventory_reservations`.
- `available_qty = on_hand_qty - reserved_qty`.
- A reservation atomically increases `reserved_qty`.
- The `fulfillment_locations` table will be altered to support Seller-owned locations.

## 14. Inventory Reservation State Machine
- **ACTIVE:** Reservation holds stock. `reserved_qty` is incremented. Has an `expires_at`.
- **CONSUMED:** Order confirmed. `on_hand_qty` and `reserved_qty` are both decremented.
- **RELEASED:** Order cancelled or checkout abandoned. `reserved_qty` is decremented.
- **EXPIRED:** Background worker releases abandoned ACTIVE reservations. `reserved_qty` is decremented.
All transitions are idempotent.

## 15. Concurrency Strategy
PostgreSQL is the sole authority for oversale prevention; Redis locks are insufficient.
- Strategy: **Pessimistic Row Locking** (`SELECT ... FOR UPDATE` on `inventory_snapshots`) ordered consistently (e.g., by `sku_id` ASC) to avoid deadlocks.
- Or **Atomic Update:** `UPDATE inventory_snapshots SET reserved_qty = reserved_qty + $1 WHERE id = $2 AND on_hand_qty - reserved_qty >= $1`.
- Phase 5 will utilize Atomic Updates combined with `SELECT ... FOR UPDATE` for multi-row validation to ensure absolute safety.

## 16. Checkout Transaction Boundary
The creation of an Order must be entirely atomic.
```sql
BEGIN;
  -- 1. Validate Store, Market, Listing statuses
  -- 2. Validate current Listing Prices
  -- 3. Lock Inventory Snapshots (SELECT FOR UPDATE)
  -- 4. Check available stock
  -- 5. Create Inventory Reservations (Adjust reserved_qty)
  -- 6. Insert Order (PENDING)
  -- 7. Insert Order Items (Immutable Snapshots)
  -- 8. Insert Order Timeline Entry
  -- 9. Insert Outbox Event (OrderCreated)
  -- 10. Mark Checkout Session finalized
COMMIT;
```
If any step fails, the entire transaction rolls back, leaving no stranded reservations or ghost orders.

## 17. Order Aggregate
The `orders` table holds the authoritative financial and lifecycle state.
- `id`, `order_number` (human readable, unique per store)
- `store_id`, `customer_id`, `market_code`, `currency_code`
- `status`
- `subtotal_minor`, `total_minor` (signed integers)
- `timestamps`, `aggregate_version` (strictly monotonic)

## 18. Order Item Snapshot
Order items are immutable records of the transaction. Future catalog changes must not alter historical receipts.
- **Snapshot Fields:** `product_title`, `sku_code`, `unit_price_minor`, `line_total_minor`.
- **References:** `seller_listing_id`, `sku_id`, `supplier_offer_id` (internal).
- The API will never expose internal supplier identities or wholesale costs to the storefront.

## 19. Customer / Address Snapshot
- `order_addresses` stores a deep copy of the customer's delivery information at the moment of checkout.
- Normalizing to a live address ID is forbidden, as customers may update their home address later.

## 20. Order State Machine
| From | To | Actor | Precondition | Side Effect |
|------|----|-------|--------------|-------------|
| - | PENDING | Checkout | Valid payment intent | Reservation ACTIVE |
| PENDING | CONFIRMED | Seller | - | Reservation CONSUMED |
| CONFIRMED | PROCESSING | Seller | - | - |
| PROCESSING | READY_FOR_SHIPPING| Seller | Packed | - |
| PENDING | CANCELLED | Cust/Seller| - | Reservation RELEASED |
| CONFIRMED/PRO | CANCELLED | Seller | Not shipped | Return stock |

## 21. Order Timeline / Notes
- `order_timeline`: Append-only audit log tracking every state change.
  - Fields: `order_id`, `from_status`, `to_status`, `actor_type`, `actor_subject`, `reason`.
- `order_notes`: Allows Sellers to add internal notes to an order. Visibility is strictly `internal` (Seller Dashboard only).

## 22. Transactional Outbox
Follows ADR-006. 
- Business logic (Order creation) and Event emission (`outbox_events`) commit atomically.
- Schema: `event_id`, `aggregate_type`, `aggregate_id`, `aggregate_version`, `payload`, `published_at`.

## 23. Event Catalog
Phase 5 introduces these foundational Domain Events:
- `commerce.order.created.v1`
- `commerce.order.status_changed.v1` (captures CONFIRMED, CANCELLED, etc.)
- `commerce.inventory.reservation_expired.v1`

## 24. RabbitMQ Topology
- **Exchanges:** Topic exchange `commerce.events`.
- **Routing Keys:** `order.created`, `order.status_changed`.
- **Queues:** Bound independently per consumer (e.g., `queue.core.order_notifications`).
- **DLQ:** Configured for every consumer queue with bounded retry TTL.

## 25. Consumer Inbox / Idempotency
Follows ADR-007.
- Consumers of domain events insert `(consumer_name, event_id)` into `processed_events`.
- If the insert violates the unique constraint, the event is treated as a duplicate and safely ACKed without side effects.

## 26. Event Ordering
- `aggregate_version` on the Order monotonically increases with every state change (PENDING -> CONFIRMED).
- Consumers use this version to discard strictly older out-of-order events if they have already processed a newer version for the same `aggregate_id`.

## 27. API Boundaries
### Customer Storefront (via `seller-api` -> `core-api`)
- `POST /storefront/carts`: Create guest cart.
- `POST /storefront/carts/{id}/items`: Add SKU to cart.
- `POST /storefront/checkout`: Initialize session.
- `POST /storefront/checkout/{id}/finalize`: Atomic order creation.

### Seller Dashboard (via `seller-api` -> `core-api`)
- `GET /seller/orders`: List store orders.
- `POST /seller/orders/{id}/confirm`: Transition to CONFIRMED.
- `POST /seller/orders/{id}/cancel`: Cancel order.

## 28. Error Contract
Utilizes standard HTTP mappings and stable string codes:
- `insufficient_inventory` (409 Conflict)
- `price_changed` (409 Conflict)
- `market_mismatch` (400 Bad Request)
- `cart_expired` (400 Bad Request)
- `idempotency_conflict` (409 Conflict)

## 29. Security / Privacy
- **IDOR Protection:** All Storefront Cart and Order routes validate the request's `store_id` derived securely from the `X-Matjero-Storefront-Host` boundary.
- **Supplier Privacy:** Order Item responses to the Storefront actively omit `supplier_id`, `supplier_offer_id`, and wholesale margins.
- **Integer Limits:** All money and quantity inputs check for signed 64-bit integer overflow.

## 30. Observability
- **Metrics:** `checkout_success_total`, `checkout_failure_total` (labeled by reason), `inventory_oversell_prevented`.
- **Tracing:** Correlation IDs (`X-Request-ID`) pass from the HTTP request into the Outbox `correlation_id` field.
- **Logs:** Structured JSON logging; strictly masking customer PII (emails, addresses).

## 31. Database Migration Plan
1. Alter `fulfillment_locations` to allow nullable `supplier_id` and `supplier_market_id`, add nullable `seller_id` and `store_id`. Add check constraint ensuring exactly one owner type exists.
2. Create `customers` and `customer_addresses` tables.
3. Create `carts` and `cart_items` tables.
4. Create `checkout_sessions` table.
5. Create `orders`, `order_items`, `order_addresses`, `order_timeline`, and `order_notes` tables.

## 32. OpenAPI Plan
- Update `docs/api/core-internal.yaml` with the new Cart, Checkout, and Order endpoints under `/internal/v1`.
- Update `docs/api/seller-api.yaml` with corresponding Storefront and Dashboard endpoints.

## 33. Testing Strategy
- **Concurrency:** Run 50 goroutines attempting to purchase the last 1 unit of stock simultaneously. Assert exactly one success and 49 `insufficient_inventory` errors.
- **Price Drift:** Change a listing price mid-checkout. Assert the checkout is rejected.
- **Idempotency:** Fire 10 identical checkout finalized requests. Assert exactly 1 order and 9 identical success responses.
- **Outbox:** Force a crash immediately after the PostgreSQL commit. Assert the poller recovers and publishes the event on restart.

## 34. Repository Impact Matrix
| Repository | Impact | Description |
|------------|--------|-------------|
| **Core** | High | Implements entire domain, transactions, and internal APIs. |
| **Seller** | Medium | Implements Storefront bridge routes and Seller Dashboard endpoints. |
| **Supplier** | Low | Minor DTO updates if supplier-facing order notifications are pre-built. |
| **Admin** | Low | Read-only platform analytics (optional for MVP). |
| **Hubs** | None | No compile-time dependencies. |

## 35. Implementation Unit Breakdown
- **P5.1:** Customer & Cart Domain Schema + Core Foundation.
- **P5.2:** Fulfillment Location Schema Update (Seller-Owned).
- **P5.3:** Inventory Reservation State Machine & Expiry Scheduler.
- **P5.4:** Checkout Session & Durable Idempotency.
- **P5.5:** Atomic Checkout Transaction & Concurrency Enforcement.
- **P5.6:** Order Aggregate, Snapshotting, and Timeline.
- **P5.7:** Outbox Publisher & Event Catalog Wiring.
- **P5.8:** Seller-API Storefront Bridge (Cart/Checkout).
- **P5.9:** Seller-API Dashboard Bridge (Order Management).
- **P5.10:** E2E Hardening & Security Audits.
- **P5.11:** Phase 5 Completion.

## 36. Dependency Graph
P5.1 -> P5.4 -> P5.5 -> P5.6
P5.2 -> P5.3 -> P5.5
P5.5 -> P5.7
P5.6 -> P5.8 & P5.9
All -> P5.10 -> P5.11

## 37. Rollout / Feature Flag Strategy
- Phase 5 schemas and Core internal APIs will merge progressively.
- Storefront and Seller Dashboard API routes will remain protected behind a `FEATURE_PHASE_5_COMMERCE` flag until P5.10 is complete, ensuring partial checkout flows are never accessible to live traffic.

## 38. Known Risks
- **Overselling:** Mitigated by pessimistic PostgreSQL row locking on `inventory_snapshots`.
- **Double Event Publish:** Expected under at-least-once delivery; mitigated by mandatory `processed_events` Consumer Inbox.
- **Stale Cart Prices:** Mitigated by strict price re-verification inside the Checkout atomic transaction.
- **Cross-Store IDOR:** Mitigated by host-based tenant resolution stripping any client-supplied `store_id`.

## 39. Deferred Work
- **Shipping Rates & Tracking (Phase 6)**
- **Payment Providers & Refunds (Phase 7)**
- **Financial Ledger & Settlement (Phase 8)**
- **Tax Calculation Engine**
- **Promotions & Coupons**

## 40. Phase 5 Definition of Done
- Specification merged.
- All 11 implementation units completed.
- Concurrency and Idempotency tests passing in CI.
- Storefront can add an item to a cart and check out successfully.
- Seller can view and confirm the order.
- Inventory is reliably reserved and consumed.
- Zero cross-repository compile-time dependencies introduced.
