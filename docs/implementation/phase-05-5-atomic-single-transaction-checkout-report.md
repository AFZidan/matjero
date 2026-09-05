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

## 3. Verified P5.5 Test Suite & Matrix Mapping

All concurrency, allocation, validation, and immutability integration tests run deterministically with **ZERO `time.Sleep()`**:

| Test Name | Verified Matrix Case(s) | Description / Coverage |
| :--- | :--- | :--- |
| `TestIntegrationFinalizeCheckoutCorrelationIDPropagation` | `98` | HTTP/integration proof that generated or caller correlation ID propagates to outbox event |
| `TestFinalizeCheckoutConfiguredConfirmationDuration` | `95` | Configured duration respected (`confirmation_deadline_at - order_created_at == config.OrderConfirmationDuration`) |
| `TestFinalizeCheckoutSupplierOfferAvailabilityFailClosed` | `104` | Availability true/no-row allowed; availability false fails closed (`ErrListingUnavailable`); DB error rolls back |
| `TestFinalizeCheckoutCopiesSessionCapabilityDigest` | `63` | Exact byte-for-byte capability digest copy from Session to Order |
| `TestFinalizeCheckoutMissingGuestDigestRejected` | `67` | Invalid/missing capability digest rejected; zero side effects across all 11 tables |
| `TestFinalizeCheckoutMoneyMultiplicationOverflow` | `21`, `23` | Cart & listing price math.MaxInt64 overflow reaches `checkedMultiply`, returns `ErrInvalidInput`, 0 side effects |
| `TestFinalizeCheckoutMaximumValidMultiplication` | `20` | Max valid `int64` multiplication succeeds with exact line totals and subtotal |
| `TestFinalizeCheckoutSubtotalAdditionOverflow` | `22`, `23` | Multiple valid lines summing > MaxInt64 fails `checkedAdd`, returns `ErrInvalidInput`, complete rollback |
| `TestFinalizeCheckoutTwoOpenSessionsOneCartConcurrentContention` | `91`, `92` | Real concurrent contention on Cart lock; exactly 1 Order, 1 session finalized, loser returns `ErrConflict` |
| `TestFinalizeCheckoutLateFailureRollsBackEverything` | `3` | Failure before commit asserts exact equality for all 11 tables/states, including `store_order_sequences` |
| `TestFinalizeCheckoutCumulativeSnapshotDemand` | `113` | Multiple cart lines competing for same snapshot capacity enforce cumulative demand (`ErrInsufficientInventory`) |
| `TestFinalizeCheckoutDeterministicAllocationFallback` | `2`, `114` | ID-ASC candidate snapshots fall back to next snapshot when earlier snapshot capacity exhausted |
| `TestFinalizeCheckoutSessionExpiryAfterCartLockWait` | `17`, `18`, `86` | Lock wait past session expiry captures `$session_decision_now` post-lock and returns `checkout_expired` |
| `TestFinalizeCheckoutConfirmationWindowAfterInventoryWait` | `95` | Inventory lock wait captures `$order_created_at` post-revalidation and grants full configured deadline window |
| `TestFinalizeCheckoutSupplierBacked` | `4`, `103`, `107`, `118`, `119`, `121` | Active Supplier Offer finalization populates supplier cost/source snapshots, supplier location, rejects inactive/missing price |
| `TestFinalizeCheckoutSellerOwnedSourceIsolation` | `5`, `105`, `106` | Seller listing cannot allocate another Store or Supplier stock; inactive location rejected |
| `TestFinalizeCheckoutCommercialPreAcceptanceRace` | `126` | Price/status change committed before acceptance is observed and rejected/remapped |
| `TestFinalizeCheckoutCommercialPostAcceptanceImmutability` | `41`, `120`, `127` | Commercial changes post `$order_created_at` leave persisted Order totals, retail price, and supplier cost unchanged |
| `TestOrderCreatedEnvelopeCompleteAndPrivate` | `96`, `99`, `125` | Persisted outbox row envelope fields asserted; payload privacy excludes supplier cost, reservation tokens, digests |
| `TestFinalizeCheckoutCreatesOrderAtomically` | `16`, `57`, `68` | Single-transaction creation, initial aggregate version = 1, open session with active cart finalizes |
| `TestFinalizeCheckoutTenConcurrentIdenticalRequestsOneOrder` | `6`, `7`, `64` | 10 concurrent identical finalizes produce 1 Order; all return identical Order |
| `TestFinalizeCheckoutReplayReturnsExactSameOrder` | `19`, `65`, `69`, `82` | Finalized session replay returns exact same Order with zero side-effects and no capability rotation |
| `TestFinalizeCheckoutChangedSemanticReplayConflicts` | `8`, `83`, `84` | Finalized session with altered semantic request returns `idempotency_conflict` |
| `TestFinalizeCheckoutLastUnitContention` | `1`, `94` | 20+ concurrent finalization requests for last unit yield exactly 1 Order without overselling |
| `TestFinalizeCheckoutPriceChanged` | `9` | Retail price change before acceptance returns `price_changed` |
| `TestFinalizeCheckoutCurrencyChanged` | `10`, `122` | Listing currency mismatch returns `price_changed` / `market_mismatch` |
| `TestFinalizeCheckoutInactiveProduct` | `100` | Inactive product returns `listing_unavailable` |
| `TestFinalizeCheckoutInactiveVariant` | `101` | Inactive variant returns `listing_unavailable` |
| `TestFinalizeCheckoutInactiveSKU` | `102` | Inactive SKU returns `listing_unavailable` |
| `TestFinalizeCheckoutSellerOwnedSupplierFieldsNull` | `123` | Seller-owned order items contain NULL supplier cost and source fields |
| `TestFinalizeCheckoutNoSplitFulfillment` | `112` | Order line requiring 5 units cannot split across locations with 3 + 3 units |

---

## 4. Matrix Coverage Summary

- **Genuinely Verified P5.5 Matrix Cases**: `1–10`, `16–23`, `41`, `57`, `63–65`, `67–69`, `82–84`, `86`, `91–92`, `94–96`, `98–107`, `112–114`, `118–123`, `125–127`.
- **P5.4 Owned Lifecycle Matrix Cases**: `25–31`, `37–39`, `59–60`, `71–77`, `87–90`, `115–117` (Verified in Phase 5.4 suite `orders_integration_test.go` & `order_inventory_lifecycle_integration_test.go`).
- **P5.6 Outbox Publisher Reliability Matrix Cases**: `49–56`, `78–81`, `97` (NOT STARTED — strictly out-of-scope P5.6 outbox claim/lease publisher work).
- **Matrix Cases Not Owned / Out of Scope for P5.5 Engine**:
  - `32–36`, `40`, `61–62`, `66`, `124`: Guest Order Access Token HTTP/Storefront API authorization (P5.7 / Storefront API).
  - `47–48`, `108–111`, `128–137`: Storefront / Seller API catalog discovery & Add-to-Cart resolution (P5.7 & P5.8).
  - `NOT YET VERIFIED`: `11–15`, `24`, `42–46`, `70`, `85`, `93` (To be covered in dedicated Storefront/Seller API and DB schema constraint integration suites).

---

## 5. Summary of Verification Results

- `gofmt -w`: Clean
- `go list ./...`: Clean
- `go list -m all`: Clean
- `go vet ./...`: Clean
- `go test ./...`: PASS (All packages cleanly passing)
- `go test ./modules/commerce -run 'TestFinalizeCheckout|TestOrderCreated' -count=10`: PASS (Clean execution across 10 repeats)
- `go run ./cmd/openapi-gen`: Specs updated in `docs/api/internal/openapi.json` (Zero drift)
- `docker compose config`: Valid
- Binary builds (`core-api`, `general-worker`, `scheduler`): Clean
- `pkg/` imports in active code: ZERO
- `time.Sleep()` in P5.5 concurrency tests: ZERO (100% deterministic coordination via `pg_locks` & test hooks)
