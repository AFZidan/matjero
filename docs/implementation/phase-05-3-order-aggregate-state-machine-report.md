# Phase 5.3 Completion Report — Order Aggregate + Sequences + State Machine Foundation

## 1. Executive Summary

Phase 5.3 implements the Order Aggregate, Store Order Sequence Allocator, Order State Machine, and DB schema persistence layer in `matjeroapps/core`. It establishes PostgreSQL database-enforced integrity for Order currency, Supplier commercial snapshots, tenant isolation, and operational source lineage while remaining decoupled from Phase 5.4 inventory reservation mutations and Phase 5.5 atomic single-transaction checkout.

## 2. Implementation Metadata

- **Base SHA:** `261adb57798704ef48f5c8ee93085bc3dc4eff3b`
- **Branch:** `feature/p5-3-order-aggregate-state-machine`
- **Migration E:** `000012_order_aggregate_schema.up.sql` / `000012_order_aggregate_schema.down.sql`
- **Tables Added:**
  - `store_order_sequences`
  - `orders`
  - `order_items`
  - `order_addresses`
  - `order_timeline`
  - `order_notes`

## 3. Core Architecture & Components

### 3.1 Store Order Sequence Allocator
- **Location:** `pkg/commerce/order_repository.go` (`AllocateOrderNumber`)
- **Query:**
  ```sql
  INSERT INTO store_order_sequences (store_id, next_value)
  VALUES ($store_id, 100002)
  ON CONFLICT (store_id) DO UPDATE
  SET next_value = store_order_sequences.next_value + 1
  RETURNING next_value - 1;
  ```
- **Semantics:** Monotonic per-Store integer starting at `100001`, formatted as `#100001`, `#100002`. The database row acts as the sole concurrency authority. Concurrency tests prove zero duplicates across simultaneous allocations and independent sequence counters per Store.

### 3.2 Order State Machine & Authority Rules
- **Location:** `pkg/commerce/orders.go` (`ValidateOrderTransition`)
- **Enabled Phase 5 Transitions:**
  - `<creation> -> pending` (Authority: `checkout`)
  - `pending -> confirmed` (Authority: `seller`, Precondition: `decision_now < confirmation_deadline_at`)
  - `pending -> cancelled` (Authority: `customer`, `seller`, or `scheduler`)
  - `confirmed -> processing` (Authority: `seller`)
  - `confirmed -> cancelled` (Authority: `seller`)
  - `processing -> ready_for_shipping` (Authority: `seller`)
  - `processing -> cancelled` (Authority: `seller`)
- **Deadline Enforcement:** Seller confirmation at or after deadline (`decision_now >= confirmation_deadline_at`) returns `ErrInvalidTransition` (`invalid_order_transition`).
- **Aggregate Version:** Starts at 1 upon order creation and increments transactionally on every valid state mutation. Invalid transitions leave version and status unchanged.

### 3.3 Database Constraints & Currency Integrity
- **Currency Isolation:** `orders` defines `UNIQUE (id, currency_code)`. `order_items` enforces `FOREIGN KEY (order_id, currency_code) REFERENCES orders(id, currency_code) ON DELETE RESTRICT`. PostgreSQL directly rejects any Order Item whose currency differs from its parent Order.
- **Supplier Commercial Snapshot Invariants:** DB CHECK constraint `order_items_supplier_snapshot_check` requires all four Supplier fields (`supplier_offer_id`, `source_supplier_id`, `supplier_cost_minor`, `supplier_cost_currency_code`) to be NULL for Seller-owned inventory, or all four NOT NULL for Supplier-backed inventory.
- **Lineage Delete Policies:** Operational source references (`supplier_offer_id`, `source_supplier_id`, `fulfillment_location_id`, `inventory_reservation_id`) use `ON DELETE RESTRICT`. Catalog display references (`seller_listing_id`, `product_id`, `variant_id`, `sku_id`) use `ON DELETE SET NULL` with immutable text snapshots (`product_title_snapshot`, `sku_code_snapshot`, `unit_price_minor`, `currency_code`).

## 4. Security & Privacy

- Public DTO `ToPublic()` in `pkg/commerce/orders.go` sanitizes output by excluding internal operational fields (`source_supplier_id`, `supplier_offer_id`, `supplier_cost_minor`, `supplier_cost_currency_code`, `fulfillment_location_id`, `inventory_reservation_id`, `guest_order_access_token_digest`).
- Tenant isolation is strictly enforced via DB composite UNIQUE keys and composite FKs (`store_id, market_code`, `customer_id, store_id`, `checkout_session_id, store_id`).

## 5. Verification Results

- **Unit & Integration Tests:**
  - `go test -v ./pkg/commerce` -> PASS (All tests including sequence allocation concurrency, constraint validation, lineage restrictions, state machine matrix, and migration up/down checks passed).
  - `go test ./...` -> PASS across all packages in `matjeroapps/core`.
- **Code Quality:**
  - `go vet ./...` -> PASS (exit code 0).
- **Configuration & Builds:**
  - `docker compose config` -> PASS (exit code 0).
  - `go build ./apps/core-api` -> PASS
  - `go build ./apps/workers/general-worker` -> PASS
  - `go build ./apps/workers/scheduler` -> PASS
  - `go run ./cmd/openapi-gen` -> PASS (no spec drift).

## 6. Scope Boundaries & Deferred Work

- `P5.4 started: NO`
- Inventory reservation state transitions, allocation, and two-stage scheduler expiry are deferred to P5.4.
- Atomic single-transaction checkout finalization is deferred to P5.5.
- Outbox claim lease worker is deferred to P5.6.

