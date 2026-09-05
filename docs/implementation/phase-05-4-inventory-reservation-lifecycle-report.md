# Matjero Phase 5.4 — Inventory Reservation Lifecycle + Allocation + Expiry Implementation Report

**Repository:** `matjeroapps/core`  
**Base SHA:** `030a1450a6d3f2fcc8332ab880cea9416e45a4e2`  
**Branch:** `feature/p5-4-inventory-reservation-lifecycle`  

---

## Executive Summary

Phase 5.4 implements the complete **Inventory Reservation Lifecycle**, deterministic candidate allocation primitives, Order transition transactions with strict lock ordering, PostgreSQL-derived `decision_now` timestamps, a two-stage confirmation-timeout expiry scheduler, and transactional status-changed Outbox event emission.

All requirements specified in `docs/implementation/phase-05-cart-checkout-orders-inventory.md` have been fulfilled. Full test suites pass with zero regressions.

---

## 1. Scope and Implementation Breakdown

### Schema Changes
**NONE**. Reused existing PostgreSQL tables (`inventory_snapshots`, `inventory_reservations`, `inventory_movements`, `orders`, `order_items`, `order_timeline`, `outbox_events`).

### Reservation State-Machine Primitives
Implemented canonical states: `held`, `consumed`, `released`, `expired`.
- Terminal states (`consumed`, `released`, `expired`) cannot transition again.
- Retries on terminal states are idempotent and do not create secondary inventory effects or movements.

### Inventory Movement Semantics
`quantity_delta` in `inventory_movements` strictly represents `on_hand_qty` delta:
- `reservation_held`: `quantity_delta = 0`, `reserved_qty += quantity`, `on_hand_qty` unchanged.
- `reservation_released`: `quantity_delta = 0`, `reserved_qty -= quantity`, `on_hand_qty` unchanged.
- `reservation_expired`: `quantity_delta = 0`, `reserved_qty -= quantity`, `on_hand_qty` unchanged.
- `reservation_consumed`: `quantity_delta = -quantity`, `reserved_qty -= quantity`, `on_hand_qty -= quantity`.
- `order_cancellation_restock`: `quantity_delta = +quantity`, `reserved_qty` unchanged, `on_hand_qty += quantity`.

### Allocation Primitive
Implemented `AllocateLineSnapshot`:
- Orders candidate snapshots deterministically by `inventory_snapshot.id ASC`.
- Evaluates line quantity against `on_hand_qty - reserved_qty - cumulativeAllocated`.
- No split fulfillment: fails with `ErrInsufficientInventory` if no single snapshot satisfies the line.
- Respects cumulative intra-transaction demand.

### Lock Ordering & Concurrency Strategy
Strict mandatory lock order enforced across all Order transitions:
1. `orders` row `FOR UPDATE`
2. `decision_now = clock_timestamp()` captured directly from PostgreSQL
3. Linked `inventory_snapshots` rows `FOR UPDATE` (ordered by `id ASC`)
4. Linked `inventory_reservations` rows `FOR UPDATE` (ordered by `id ASC`)

### Refactoring `UpdateOrderStatus`
- Refactored `UpdateOrderStatus` and transition methods so externally callable business commands do NOT supply caller-controlled timestamps.
- Captured `decision_now = clock_timestamp()` inside the locked transaction after Order lock.
- Delegated all transition execution to explicit lifecycle commands (`ConfirmOrder`, `CancelPendingOrder`, `CancelConfirmedOrder`, `ExpirePendingOrder`, `AdvanceOrderStatus`).
- Eliminated any status-only bypass for inventory-affecting transitions.

### Seller Confirmation Transaction (`pending -> confirmed`)
- Locked Order `FOR UPDATE`, captured `decision_now = clock_timestamp()`.
- Validated `decision_now < confirmation_deadline_at`.
- Locked snapshots (`id ASC`) and reservations (`id ASC`).
- Revalidated reservation invariants: `status == held`, `expires_at > decision_now`, `expires_at == confirmation_deadline_at`.
- Aggregated quantities per snapshot (`SUM(quantity)`).
- Consumed inventory (`on_hand_qty -= Q`, `reserved_qty -= Q`), updated reservations (`consumed`), recorded `reservation_consumed` movement with `quantity_delta = -Q`.
- Updated Order status (`confirmed`), incremented `aggregate_version`, inserted timeline, and enqueued `commerce.order.status_changed.v1` in Outbox.

### Customer / Seller Cancellation Transactions
- **Pending Cancel (`pending -> cancelled`)**: Releases `reserved_qty -= Q`, marks reservations `released`, records `reservation_released` movement (`quantity_delta = 0`), appends timeline, enqueues status event.
- **Post-Confirm Restock (`confirmed -> cancelled` or `processing -> cancelled`)**: Restocks `on_hand_qty += Q`, leaves reservations `consumed` (terminal), records `order_cancellation_restock` movement (`quantity_delta = +Q`), appends timeline, enqueues status event.

### Two-Stage Expiry Scheduler
- **Stage 1**: Query bounded candidate Order IDs (`status = 'pending' AND confirmation_deadline_at <= clock_timestamp() ORDER BY confirmation_deadline_at ASC LIMIT $batch_size`).
- **Stage 2**: Dedicated per-order transaction per candidate Order ID (`ExpirePendingOrder`): locks Order `FOR UPDATE`, revalidates deadline against `clock_timestamp()`, locks snapshots (`id ASC`) and reservations (`id ASC`), expires held reservations (`reserved_qty -= Q`), sets `status = cancelled` with `cancellation_reason = confirmation_timeout`, appends scheduler timeline, enqueues status event.
- Integrated into `apps/workers/scheduler`.

### Status-Changed Event & Outbox
- Implemented `NewOrderStatusChangedEvent` producing `commerce.order.status_changed.v1`.
- Enqueued transactionally via `outbox.Store.Enqueue(ctx, tx, envelope)`.
- Verified event privacy: excludes Supplier costs, source supplier IDs, location IDs, reservation IDs, guest token digests, and raw capabilities.

---

## 2. Mandatory Matrix Coverage

- **Case 2**: Deterministic location selection (lowest `inventory_snapshot.id ASC`).
- **Case 25**: Confirm vs expiry race (exactly one wins).
- **Case 26**: Cancel vs expiry race (exactly one cancellation effect).
- **Case 27**: Confirm vs cancel race (exactly one transition commits).
- **Case 28**: Expiry retry idempotency (no double decrement).
- **Case 29**: Cancellation retry idempotency (no double release/restock).
- **Case 30**: Seller confirm consumption (`held -> consumed`).
- **Case 31**: Seller cancellation restock (`on_hand_qty += Q`).
- **Case 59**: All enabled Phase 5 state machine transitions.
- **Case 60**: All rejected transitions (zero side-effects).
- **Case 71–75**: Confirm before/at/after deadline boundaries, reservation/inventory safety on late confirm.
- **Case 76**: Confirm vs expiry boundary (exactly 1 terminal outcome).
- **Case 77**: Multi-reservation lock ordering (`id ASC`).
- **Case 87–90**: Order-lock serialization point & post-lock `decision_now` timestamp.
- **Case 112–117**: No split fulfillment, cumulative demand, allocation fallback, multi-item aggregate release/consumption/restock.
- **Section 40 Critical Regression Test**: Proving caller-controlled pre-lock timestamp cannot bypass deadline or override DB `clock_timestamp()`.

---

## 3. Scope Boundary Assertions

- `P5.5 started`: **NO**
- `P5.6 started`: **NO**
- `P5.7/P5.8 started`: **NO**

---

## 4. Verification Results

- `gofmt`: Passed (clean formatting)
- `go vet ./...`: Passed
- `go test ./...`: Passed
- `apps/core-api` build: Passed
- `apps/workers/general-worker` build: Passed
- `apps/workers/scheduler` build: Passed
