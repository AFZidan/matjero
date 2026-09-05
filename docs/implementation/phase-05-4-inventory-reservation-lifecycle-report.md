# Matjero Phase 5.4 — Inventory Reservation Lifecycle + Allocation + Expiry Implementation Report

**Repository:** `matjeroapps/core`  
**Base SHA:** `030a1450a6d3f2fcc8332ab880cea9416e45a4e2`  
**Branch:** `feature/p5-4-inventory-reservation-lifecycle`  

---

## Executive Summary

Phase 5.4 implements the complete **Inventory Reservation Lifecycle**, deterministic candidate allocation primitives, Order transition transactions with strict lock ordering, PostgreSQL-derived `decision_now` timestamps, a two-stage confirmation-timeout expiry scheduler, and transactional status-changed Outbox event emission.

A final test-hardening patch has been applied to prove post-mutation transaction rollback and real PostgreSQL lock contention without wall-clock sleeps.

All requirements specified in `docs/implementation/phase-05-cart-checkout-orders-inventory.md` have been fulfilled. Full test suites pass with zero regressions.

---

## 1. Corrective & Test-Hardening Improvements

### 1. AdvanceOrderStatus Whitelist
- Implemented `isInventoryNeutralAdvance(from, to, authority)`.
- Restricted `AdvanceOrderStatus` to ONLY permit:
  - `confirmed -> processing` (seller only)
  - `processing -> ready_for_shipping` (seller only)
- Any attempt to invoke `AdvanceOrderStatus` for inventory-affecting transitions immediately returns `ErrInvalidTransition` prior to state mutation.

### 2. Strict Transaction Scope Enforcement
- Implemented `requireTx(exec DBExecutor) (pgx.Tx, error)`.
- All lifecycle mutation commands (`ConfirmOrder`, `CancelPendingOrder`, `CancelConfirmedOrder`, `ExpirePendingOrder`, `AdvanceOrderStatus`, `HoldReservation`) reject non-transactional executors (such as DB pools) BEFORE executing any mutation SQL.

### 3. Scheduler Cancellation Isolation
- `CancelPendingOrder` enforces authority validation: permits ONLY `AuthorityCustomer` and `AuthoritySeller`, and returns `ErrInvalidTransition` if called with `AuthorityScheduler`.
- Scheduler confirmation timeout cancellations execute exclusively via `ExpirePendingOrder`.

### 4. Fail-Closed Complete Reservation Set Invariants
- All Order lifecycle operations collect every required `inventory_reservation_id` from `order_items`.
- Require `len(locked_reservations) == len(expected_reservation_ids)` and exact deadline lineage match (`expires_at == confirmation_deadline_at`).
- Any mixed or corrupted reservation state immediately fails closed with `ErrInternalError` and rolls back with zero partial side effects.

### 5. Real Post-Mutation Transactional Rollback Test
- Implemented `TestLifecycleTransactionRollsBackAfterDownstreamFailure`.
- Injected downstream failure at outbox enqueue after snapshot, reservation, movement, order, and timeline mutations executed.
- Verified complete PostgreSQL transaction rollback across all 6 tables with zero persisted side effects.

### 6. Real Lock Contention Concurrency Tests (Zero Sleep)
- Implemented `TestConfirmVsExpirySellerWinsUnderLockContention` and `TestConfirmVsExpirySchedulerWinsUnderLockContention`.
- Updated waiter detection in `TestConfirmLockLinearizedDeadlineWithoutSleep` to query `pg_locks.granted = false` and `pg_stat_activity.wait_event_type = 'Lock'`.
- Proved real PostgreSQL lock wait queue serialization between competing seller and scheduler transactions.

### 7. RowsAffected Optimistic Guards & Outbox Privacy
- Checked `cmdTag.RowsAffected()` on all `UPDATE inventory_snapshots` optimistic version queries and bulk `UPDATE inventory_reservations` statements.
- `commerce.order.status_changed.v1` events are stored transactionally with zero privacy leakage.

---

## 2. Mandatory Regression Tests

- `TestAdvanceOrderStatusCannotBypassInventoryLifecycle`: Verifies `AdvanceOrderStatus` cannot execute inventory-affecting transitions.
- `TestLifecycleCommandRejectsNonTransactionalExecutor`: Verifies rejection when passing non-tx DB pool to lifecycle commands.
- `TestHoldReservationRequiresTransaction`: Verifies `HoldReservation` fails when passed a non-tx executor.
- `TestSchedulerCannotUseCancelPendingOrder`: Verifies `AuthorityScheduler` is rejected by `CancelPendingOrder`.
- `TestExpiryRejectsMixedReservationStates`: Verifies fail-closed rollback when expiry encounters mixed reservation states.
- `TestPendingCancelRejectsMixedReservationStates`: Verifies fail-closed rollback on pending cancel with mixed reservation states.
- `TestPostConfirmCancelRejectsMixedReservationStates`: Verifies fail-closed rollback on post-confirm cancel with mixed reservation states.
- `TestReservationDeadlineMismatchFailsClosed`: Verifies rejection when reservation deadline does not match order confirmation deadline.
- `TestConfirmLockLinearizedDeadlineWithoutSleep`: Deterministic post-lock deadline waiter test using PostgreSQL `pg_locks` wait detection.
- `TestConfirmVsExpirySellerWinsUnderLockContention`: Deterministic lock contention race where seller holds lock before deadline and expiry blocks on lock.
- `TestConfirmVsExpirySchedulerWinsUnderLockContention`: Deterministic lock contention race where expiry holds lock at deadline and seller blocks on lock.
- `TestLifecycleTransactionRollsBackAfterDownstreamFailure`: Proves complete rollback after business mutations have executed upon downstream failure.

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
