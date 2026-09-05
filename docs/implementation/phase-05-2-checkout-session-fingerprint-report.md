# P5.2 Checkout Session + Server-Computed Fingerprint Report

## Delivery

- Base SHA: `172f7b2ccddba87efec3bf0f60e86d7639414be9`
- Branch: `feature/p5-2-checkout-session-fingerprint`
- Final head SHA: reported after the final delivery commit
- Scope: P5.2 Checkout Session and server-computed fingerprint only

## Migration and domain

Migration `000011_checkout_sessions` creates `checkout_sessions` with the
approved `open`, `finalized`, and `expired` states, Store/Cart/Customer
composite tenant constraints, indexes, and a required 32-byte guest capability
digest. Its down migration drops only this table.

`modules/commerce/checkout_session.go` owns the Checkout Session model, creation,
lock-linearized evaluation, and fingerprint primitive. Session creation locks
the Cart, requires an active Cart, generates a dedicated 32-byte cryptographic
capability, stores only its SHA-256 digest, and returns the raw capability once.
It creates no Order, reservation, or Cart state transition. Multiple Sessions
for one Cart remain allowed.

Session lifetime is configuration-driven through `CHECKOUT_SESSION_LIFETIME`,
validated as a positive Go duration, with a 30-minute default.

## Locking and expiry

Evaluation locks the Checkout Session first, locks its parent Cart second, and
only then executes PostgreSQL `clock_timestamp()` as `session_decision_now`.
An open Session is eligible only when its Cart is active and
`expires_at > session_decision_now`; persisted `expired` status is not needed
for correctness. A finalized Session requires a checked-out Cart and may be
replayed after expiry. An open Session with an unavailable Cart returns the
normal conflict contract, while a finalized Session with a non-checked-out Cart
returns the invariant failure. No P5.3 Order lookup or side effect is created.

## Fingerprint

`ComputeFinalizeFingerprint` marshals an explicitly typed, field-ordered JSON
representation and hashes it with SHA-256. It includes Session ID, Cart ID,
Customer ID, the complete shipping address fields, contact email, and Cart
lines containing Listing ID, SKU ID, quantity, expected retail amount, and
expected currency. Lines are deterministically sorted by Listing ID and SKU
ID. Core does not accept a client fingerprint or normalize semantic values
implicitly.

## Internal API and security

The internal Core API adds host-scoped Checkout Session creation and semantic
finalization evaluation routes. Creation accepts the existing Cart bearer
capability and returns the raw guest Order capability only in that creation
response. The finalization request accepts shipping address and contact email;
it returns only the decision status and replay flag, never the digest or
fingerprint. No public browser route, Seller route, Order, reservation, or
Customer IAM integration was added.

## Tests

Added deterministic unit and database integration coverage for digest-only
capabilities, distinct multiple Sessions, no Order side effects, status and
tenant constraints, open/stale/expired Session decisions, finalized replay
after expiry, Cart-state invariants, changed semantic replay conflicts,
deterministic line-order-independent fingerprints, all Cart-line authority
fields, and lock-linearized expiry. The lock test uses a `FOR UPDATE NOWAIT`
barrier and database clock polling; it does not use sleep-based synchronization.
Coverage maps to mandatory Phase 5 cases 16–19, 61–62, 66–70, 82–85, 86, and
92–93. P5.3 case 137 and all later Order behavior remain deferred.

## Validation

The final handoff records the actual results for formatting, Go build/vet/test,
module listing, OpenAPI generation and drift, migration validation, Docker
Compose validation, the three CI binaries, security checks, and remote CI.
Local gitleaks availability is reported separately. P5.3 was not started.
