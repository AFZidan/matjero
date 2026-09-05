# Matjero Phase 5.4 — Inventory Reservation Lifecycle + Allocation + Expiry Implementation Report

**Repository:** `matjeroapps/core`  
**Base SHA:** `030a1450a6d3f2fcc8332ab880cea9416e45a4e2`  
**Branch:** `feature/p5-4-inventory-reservation-lifecycle`  

---

## Executive Summary

Phase 5.4 implements the complete **Inventory Reservation Lifecycle**, deterministic candidate allocation primitives, Order transition transactions with strict lock ordering, PostgreSQL-derived `decision_now` timestamps, a two-stage confirmation-timeout expiry scheduler, and transactional status-changed Outbox event emission.

A corrective patch has been applied to address 10 merge-blocking defects, ensuring zero inventory bypass, strict transaction-only lifecycle boundaries, scheduler-only expiry routing, complete reservation-set fail-closed validation, guarded row counts, and deterministic sleep-free concurrency tests.

All requirements specified in `docs/implementation/phase-05-cart-checkout-orders-inventory.md` have been fulfilled. Full test suites pass with zero regressions.

---

## 1. Corrective Patch Improvements & Safety Boundaries

### 1. AdvanceOrderStatus Whitelist (Blocker 1)
- Implemented `isInventoryNeutralAdvance(from, to, authority)`.
- Restricted `AdvanceOrderStatus` to ONLY permit:
  - `confirmed -> processing` (seller only)
  - `processing -> ready_for_shipping` (seller only)
- Any attempt to invoke `AdvanceOrderStatus` for inventory-affecting transitions (`pending -> confirmed`, `pending -> cancelled`, `confirmed -> cancelled`, `processing -> cancelled`) immediately returns `ErrInvalidTransition` prior to state mutation.

### 2. Strict Transaction Scope Enforcement (Blocker 2)
- Implemented `requireTx(exec DBExecutor) (pgx.Tx, error)`.
- All lifecycle mutation commands (`ConfirmOrder`, `CancelPendingOrder`, `CancelConfirmedOrder`, `ExpirePendingOrder`, `AdvanceOrderStatus`, `HoldReservation`) reject non-transactional executors (such as DB pools) BEFORE executing any mutation SQL.
- If `exec == nil`, commands open their own transaction via `r.withTx(...)`. If a non-nil executor is passed, it MUST be a valid `pgx.Tx`.

### 3. Scheduler Cancellation Isolation (Blocker 3)
- `CancelPendingOrder` enforces authority validation: permits ONLY `AuthorityCustomer` and `AuthoritySeller`, and returns `ErrInvalidTransition` if called with `AuthorityScheduler`.
- Scheduler confirmation timeout cancellations execute exclusively via `ExpirePendingOrder`, which updates reservation statuses to `expired` with `reservation_expired` movement type.

### 4. Fail-Closed Complete Reservation Set Invariants (Blocker 4)
- All Order lifecycle operations collect every required `inventory_reservation_id` from `order_items`.
- Require `len(locked_reservations) == len(expected_reservation_ids)`.
- Validate that **every linked reservation** matches the required status (`held` for confirm/pending-cancel/expire; `consumed` for post-confirm cancel) and deadline lineage (`expires_at == confirmation_deadline_at`).
- Any mixed or corrupted reservation state immediately fails closed with `ErrInternalError` and rolls back with zero partial side effects.

### 5. Deterministic No-Sleep Concurrency Tests (Blocker 5)
- Replaced wall-clock `time.Sleep` in concurrency tests with PostgreSQL transaction synchronization via `pg_stat_activity` and `clock_timestamp()` polling.
- Proved deterministic race outcomes for Confirm vs Expiry, Confirm vs Cancel, and Cancel vs Expiry.
- Zero `time.Sleep` calls remain in P5.4 concurrency tests.

### 6. RowsAffected Optimistic Guards (Blocker 6)
- Checked `cmdTag.RowsAffected()` on all `UPDATE inventory_snapshots` optimistic version queries and bulk `UPDATE inventory_reservations` statements.
- Any mismatch returns `ErrConflict` and rolls back the transaction.

### 7. PostgreSQL decision_now Semantics & HoldReservation Contract (Blockers 7 & 9)
- All Order transitions capture `decision_now = clock_timestamp()` inside the locked transaction after `orders` row lock `FOR UPDATE`.
- `HoldReservation` strictly requires `pgx.Tx`, verifies `ConfirmationDeadlineAt > DecisionNow`, checks overflow bounds `reserved_qty + qty <= on_hand_qty`, checks snapshot update row count, sets `expires_at = ConfirmationDeadlineAt`, and emits `reservation_held` movement with `quantity_delta = 0`.

### 8. Outbox Event Privacy & Atomicity (Blocker 10)
- `commerce.order.status_changed.v1` events are stored transactionally in `outbox_events` within the order transition transaction.
- Verified zero leakage of supplier costs, source IDs, fulfillment location IDs, reservation IDs, or raw capability digests.

---

## 2. Mandatory Regression Tests Added

- `TestAdvanceOrderStatusCannotBypassInventoryLifecycle`: Verifies `AdvanceOrderStatus` cannot execute inventory-affecting transitions.
- `TestLifecycleCommandRejectsNonTransactionalExecutor`: Verifies rejection when passing non-tx DB pool to lifecycle commands.
- `TestHoldReservationRequiresTransaction`: Verifies `HoldReservation` fails when passed a non-tx executor.
- `TestSchedulerCannotUseCancelPendingOrder`: Verifies `AuthorityScheduler` is rejected by `CancelPendingOrder`.
- `TestExpiryRejectsMixedReservationStates`: Verifies fail-closed rollback when expiry encounters mixed reservation states.
- `TestPendingCancelRejectsMixedReservationStates`: Verifies fail-closed rollback on pending cancel with mixed reservation states.
- `TestPostConfirmCancelRejectsMixedReservationStates`: Verifies fail-closed rollback on post-confirm cancel with mixed reservation states.
- `TestReservationDeadlineMismatchFailsClosed`: Verifies rejection when reservation deadline does not match order confirmation deadline.
- `TestConfirmLockLinearizedDeadlineWithoutSleep`: Deterministic post-lock deadline test using PostgreSQL clock polling.
- `TestConfirmVsExpirySellerWinsDeterministically`: Deterministic race where seller acquires lock before deadline.
- `TestConfirmVsExpirySchedulerWinsDeterministically`: Deterministic race where scheduler acquires lock at/after deadline.
- `TestConfirmVsCancelExactlyOneWins`: Linearized race test between seller confirmation and customer cancellation.
- `TestCancelVsExpiryExactlyOneWins`: Linearized race test between customer cancellation and scheduler expiry.
- `TestFullAtomicRollbackOnFailure`: Verifies complete rollback across orders, snapshots, reservations, movements, timeline, and outbox.

---

## 3. Scope Boundary Assertions

- `P5.5 started`: **NO**
- `P5.6 started`: **NO**
- `P5.7/P5.8 started`: **NO**

---

## 4. Verification Results

- `gofmt`: Passed (clean formatting)
- `go vet ./...`: Passed
- `go list -m all`: Passed
- `go test ./...`: Passed
- Internal OpenAPI stale-spec check: Passed (zero drift)
- Migration validation: Passed
- Docker compose config: Passed
- `apps/core-api` build: Passed
- `apps/workers/general-worker` build: Passed
- `apps/workers/scheduler` build: Passed
- Gitleaks security scan: Passed (no leaks found)
- `time.Sleep` check: ZERO in P5.4 concurrency tests
