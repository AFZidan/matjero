# Implementation Report — Matjero Phase 5.5: Atomic Single-Transaction Checkout

**Base SHA:** `5ce704c8f649c65300472e37b12f5390ee1c49dd`  
**Branch:** `feature/p5-5-atomic-single-transaction-checkout`  
**PR Target:** `main`  
**Migration Changes:** NONE  
**P5.6 Started:** NO  
**P5.7 Started:** NO  
**P5.8 Started:** NO  

---

## 1. Executive Overview

Phase 5.5 implements the authoritative atomic single-transaction checkout command (`FinalizeCheckout`) in `matjeroapps/core`. It transforms an eligible Guest Checkout Session and active Cart into exactly one confirmed/pending Order within **ONE PostgreSQL transaction**.

The implementation respects all platform architectural invariants:
- **PostgreSQL Authority**: Single transaction boundary using `pgx.Tx`.
- **Lock Linearity**: Deterministic lock hierarchy preventing deadlocks (`Session -> Cart -> Snapshots ASC -> Sequence -> Create Order`).
- **Timestamp Precision**: Separate lock-linearized `$session_decision_now` and `$order_created_at` captured via `clock_timestamp()`.
- **Authoritative Commercial Revalidation**: Final revalidation of Store, Market, Listing, Retail Price, Supplier Offer, Supplier Price, Supplier Availability, Product/Variant/SKU status, and locked Inventory Snapshots immediately before acceptance.
- **Oversell-Proof Allocation**: One-location-per-line deterministic allocation (`AllocateLineSnapshot`) with cumulative multi-line demand tracking. No split fulfillment.
- **Checked 64-Bit Arithmetic**: Multiplications and additions enforce signed int64 overflow protection, rolling back on overflow (`ErrInvalidInput`).
- **Event Privacy**: Privacy-safe `commerce.order.created.v1` event envelope containing zero supplier cost, supplier lineage, location IDs, reservation tokens, capability digests, or unapproved address PII.

---

## 2. Lock Ordering & Transaction Flow

The transaction executes in exact sequential order:

1. **Lock Checkout Session**: `SELECT ... FROM checkout_sessions WHERE id = $1 AND store_id = $2 FOR UPDATE`.
2. **Lock Parent Cart**: `SELECT ... FROM carts WHERE id = $1 AND store_id = $2 FOR UPDATE`.
3. **Capture Session Decision Timestamp**: `$session_decision_now = clock_timestamp()`.
4. **Session / Cart State Rules**:
   - `session.status == 'finalized'` requires `cart.status == 'checked_out'` (else `ErrCheckoutCartInvariant`).
   - `session.status == 'open'` requires `cart.status == 'active'` (else `ErrConflict`).
   - `expires_at <= $session_decision_now` or `status == 'expired'` returns `ErrCheckoutExpired`.
5. **Load Cart Items**: Ordered by `seller_listing_id ASC, sku_id ASC`.
6. **Compute Server Fingerprint**: `ComputeFinalizeFingerprint(session, cart, request)`.
7. **Finalized Replay**: If `session.status == 'finalized'`, compare fingerprints. If matching, return existing Order via `GetOrderByID` with ZERO side effects. If mismatch, return `ErrIdempotencyConflict`.
8. **Guest Payload & Capability Digest Validation**: Validate guest shipping address, email, and require 32-byte `guest_order_access_token_digest`.
9. **Candidate Inventory Discovery & Snapshot Locking**: Deduplicate candidate snapshot IDs across lines, sort globally `id ASC`, and lock via `SELECT ... FROM inventory_snapshots WHERE id = ANY($1) ORDER BY id ASC FOR UPDATE`.
10. **Allocate Store Order Sequence**: Call `AllocateOrderNumber` inside the transaction.
11. **Final Commercial Revalidation**: Re-read and revalidate Store, Market, Seller Listing, Retail Price Amount & Currency, Product, Variant, SKU, Supplier Offer, Supplier Price, and Supplier Availability.
12. **Checked Money Arithmetic & Allocation**: Allocate snapshots line-by-line (`AllocateLineSnapshot`) using transaction-local cumulative demand map.
13. **Capture Order Acceptance Timestamp**: `$order_created_at = clock_timestamp()` captured after final revalidation. Freeze `$confirmation_deadline_at = $order_created_at + 15m`.
14. **Hold Reservations & Movements**: Call `HoldReservation` per line (`reserved_qty += qty`, `reservation_held` movement with `quantity_delta = 0`).
15. **Create Order, Items, Address, Timeline**: Insert Order (copying `guest_order_access_token_digest`), Order Items (capturing immutable Supplier cost snapshots or NULLs for Seller-owned lines), Order Address, and Order Timeline (`from_status = NULL`, `to_status = 'pending'`, `actor_type = 'checkout'`).
16. **OrderCreated Event & Outbox**: Enqueue `commerce.order.created.v1` in `outbox_events`.
17. **Finalize Session & Cart**: Update `checkout_sessions` (`status = 'finalized'`, `finalize_fingerprint`, `shipping_address_snapshot`, `contact_email`, `finalized_at = $order_created_at`) and `carts` (`status = 'checked_out'`). Assert `RowsAffected == 1`.

---

## 3. Implemented Matrix Cases

The following Phase 5 matrix cases are fully implemented and verified by unit/integration tests:
- **Matrix Cases**: `1–24`, `57` (creation aggregate version = 1), `63–65`, `67–70`, `82–86`, `91–96`, `98–100`, `101–107`, `112–114`, `118–137`.
- **P5.6 Cases Excluded**: `49–56`, `78–81`, `97` (Outbox publisher claim/lease reliability remains P5.6 / NOT STARTED).

---

## 4. Concurrency & Integration Test Suite

All concurrency integration tests use deterministic channels, barriers, database locks, and clock polling with **ZERO `time.Sleep()`**:

1. `TestFinalizeCheckoutCreatesOrderAtomically`
2. `TestFinalizeCheckoutTenConcurrentIdenticalRequestsOneOrder`
3. `TestFinalizeCheckoutReplayReturnsExactSameOrder`
4. `TestFinalizeCheckoutChangedSemanticReplayConflicts`
5. `TestFinalizeCheckoutTwoSessionsOneCartExactlyOneOrder`
6. `TestFinalizeCheckoutLastUnitContention`
7. `TestFinalizeCheckoutLateFailureRollsBackEverything`
8. `TestFinalizeCheckoutMoneyMultiplicationOverflow`
9. `TestFinalizeCheckoutPriceChanged`
10. `TestFinalizeCheckoutCurrencyChanged`
11. `TestFinalizeCheckoutInactiveProduct`
12. `TestFinalizeCheckoutInactiveVariant`
13. `TestFinalizeCheckoutInactiveSKU`
14. `TestFinalizeCheckoutSellerOwnedSupplierFieldsNull`
15. `TestFinalizeCheckoutCopiesSessionCapabilityDigest`
16. `TestFinalizeCheckoutMissingGuestDigestRejected`
17. `TestFinalizeCheckoutNoSplitFulfillment`
18. `TestFinalizeCheckoutCumulativeSnapshotDemand`
19. `TestOrderCreatedEnvelopeCompleteAndPrivate`

---

## 5. Summary of Verification Results

- `gofmt`: Clean
- `go list ./...`: Clean
- `go list -m all`: Clean
- `go vet ./...`: Clean
- `go test ./...`: PASS (All packages)
- `go run ./cmd/openapi-gen`: Specs updated in `docs/api/internal/openapi.json`
- `docker compose config`: Valid
- Binary builds (`core-api`, `general-worker`, `scheduler`): Clean
- `pkg/` imports in active code: ZERO
- `time.Sleep()` in `modules/commerce` / `internal/coreapi`: ZERO
