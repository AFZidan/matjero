# Custom Domain Lifecycle Report — P4.8 Stage A Core Capabilities

## Architectural Overview

This document reports the implementation of **P4.8 Custom Domain Lifecycle (Stage A — Core Domain Lifecycle + DNS Verification + Moderation Capabilities)** in `/var/www/personal/matjero/core`.

Core owns the domain lifecycle state machine, PostgreSQL persistence, DNS TXT verification, domain validation, primary domain switching invariants, admin moderation capabilities, resource authorization, and internal runtime HTTP contracts. No changes were made to Seller or Admin repositories, adhering strictly to the permanent **Repository Independence** constraint.

---

## 1. Domain Lifecycle State Machine

The store domain state model is governed by explicit lifecycle transitions:

```
                  [Seller Request]
                         │
                         ▼
                     (pending)
                         │
        ┌────────────────┴────────────────┐
   [DNS Verified]                  [DNS Failed]
        │                                 │
        ▼                                 ▼
    (verified) ◄─────[Retry]─────── (failed)
        │
 [Seller Activate]
        │
        ▼
     (active)
 (is_primary=true)
```

### Transition Invariants
- **New Custom Domain**: Starts in `custom` type, `pending` status, `is_primary = false`, with a cryptographically secure 32-byte verification token.
- **Ownership Verification**:
  - Exact TXT record match: `pending` or `failed` ➔ `verified` (`verified_at` and `last_checked_at` populated).
  - TXT mismatch / NXDOMAIN / missing record: `pending` or `failed` ➔ `failed` (`last_checked_at` updated).
- **Activation**:
  - `verified` ➔ `active` (`is_primary = true`).
  - Atomically demotes the store's previous primary domain to `is_primary = false` while maintaining its `active` status.
- **Admin Disable**:
  - `pending` / `verified` / `failed` / `active` ➔ `disabled` (`is_primary = false`).
  - If the disabled domain was the store's `primary`, Core automatically promotes an eligible `active` `platform` domain for the same store to `is_primary = true`. If no active platform domain exists, the store is left without a primary domain.
- **Admin Re-enable**:
  - `custom` domain: returns to `verified` (if previously verified, i.e. `verified_at != nil`) or `pending` (if unverified) with `is_primary = false`. It does **not** bypass Seller activation.
  - `platform` domain: returns to `active`. If the store currently has no primary domain, it becomes `is_primary = true`.

### Disallowed Transitions
- `pending` ➔ `active` (forbidden)
- `failed` ➔ `active` (forbidden)
- `disabled custom` ➔ `active` directly (forbidden; must re-enable to `verified` then Seller activates)
- Seller mutating platform domain lifecycle (forbidden)

---

## 2. DNS Verification Contract & Resolver Abstraction

### TXT Record Specification
- **Record Name**: `_matjero-verification.<custom-domain>`
- **Record Value**: `matjero-verification=<token>`
- **Token Generation**: 32 cryptographically random bytes (`crypto/rand`), hex-encoded (64 hex characters).

### Resolver Abstraction & Bounded Lookup
- **Interface**:
  ```go
  type TXTResolver interface {
      LookupTXT(ctx context.Context, name string) ([]string, error)
  }
  ```
- **Default Implementation**: `DefaultTXTResolver` wrapping `net.Resolver`.
- **Bounded Timeout**: Every DNS lookup runs under a 5-second `context.WithTimeout`.
- **Error Classification**:
  - **Temporary Resolver Failure / Timeout**: Returns `ErrUnavailable` (503 Service Unavailable). Database lifecycle state remains **unchanged**.
  - **Authoritative No-Record / NXDOMAIN / TXT Mismatch**: Updates status to `failed` and sets `last_checked_at = now()`.

---

## 3. Custom Domain Validation Policy

Custom domain validation is enforced by `ValidateCustomDomain(domain, platformDomain string)` before persistence:
- **Canonical Normalization**: Uses `commerce.NormalizeDomain` (trims whitespace, lowercases, strips trailing dots, rejects ports/paths/credentials).
- **Prohibited Patterns**:
  - Empty hostnames, scheme prefixes (`http://`, `https://`).
  - Port (`:`), path (`/`), credentials (`@`), wildcards (`*`).
  - IP literals (IPv4 and IPv6 parsed via `net.ParseIP`).
  - Single-label hostnames and `localhost` (must contain at least 1 dot).
  - Total length > 253 characters.
  - Labels > 63 characters, starting/ending with `-`, containing underscores `_`, or containing non-ASCII characters.
- **Platform Domain Protection**: Rejects custom domain requests that match or end with `.` + `PlatformDomain` (e.g. `foo.matjero.com` when `PlatformDomain = matjero.com`).

---

## 4. Primary Domain & Storefront Integration

### Single Primary Domain Invariant
- PostgreSQL index `store_domains_one_primary_per_store ON store_domains (store_id) WHERE is_primary = true` enforces at most one primary domain per store.
- Domain activation (`ActivateCustomDomainTx`) runs within a single PostgreSQL transaction with `FOR UPDATE` row locks, ensuring atomic primary switching without violating the index.

### Secondary Active Domains
- Activating a custom domain demotes the existing platform domain to `is_primary = false` but leaves it `status = "active"`.
- Both the primary custom domain and secondary platform domain resolve in `StoreResolver`.

### Storefront Routing & Host Discovery
- **StoreResolver**: Resolves ONLY domains with `status == "active"`. `VERIFIED`, `PENDING`, `FAILED`, and `DISABLED` domains return `ErrDomainInactive` and do not resolve.
- **StorefrontHost Discovery**: `GET /internal/v1/stores/{storeID}/storefront-host` queries `GetActivePrimaryStoreDomain` and returns the active primary custom domain immediately after activation.
- **Cache Revisions**: Domain lifecycle transitions do **not** bump `storefront_revisions` because cache safety is handled by host resolution checks before serving cached generations.

---

## 5. Internal API & OpenAPI Contract

Core exposes internal capabilities under `/internal/v1`:

### Seller Internal Routes (`CallerSeller`)
- `GET /internal/v1/stores/{storeID}/domains` — List store domains with verification instructions DTO.
- `POST /internal/v1/stores/{storeID}/domains` — Request custom domain (`{"domain": "shop.example.com"}`).
- `POST /internal/v1/stores/{storeID}/domains/{domainID}/verify` — Trigger DNS TXT verification.
- `POST /internal/v1/stores/{storeID}/domains/{domainID}/activate` — Activate verified custom domain.

### Admin Moderation Routes (`CallerAdmin`)
- `GET /internal/v1/domains` — List domains across stores with pagination and filters (`store_id`, `seller_id`, `status`, `domain_type`, `search`). Excludes raw `verification_token` for privacy.
- `POST /internal/v1/domains/{domainID}/disable` — Disable domain.
- `POST /internal/v1/domains/{domainID}/enable` — Re-enable domain.

### OpenAPI Artifacts
- committed spec updated: `docs/api/internal/openapi.json`
- Router-to-spec drift guard verified green via `TestSpecMatchesRouter`.

---

## 6. Database Schema & Migration Verification

- Zero database migrations added.
- Leveraged pre-existing schema from `000005_store_domain_lifecycle.up.sql` and `000006_store_domain_integrity.up.sql`.

---

## 7. Security Posture

- **Verification Token Security**: Cryptographically random 32-byte entropy (`crypto/rand`). Never logged in raw form.
- **Authorization Enforcement**: Core verifies Store ownership using the forwarded authenticated Seller subject (`RequireSellerAccess`). Cross-seller access yields safe 404 responses.
- **Caller Separation**: Admin moderation routes strictly require `CallerAdmin`.

---

## 8. Known Deployment Responsibilities

- **DNS Ownership Proof vs Traffic Routing**: TXT record verification proves domain control. It does **not** configure DNS A/AAAA/CNAME records or Cloudflare/load-balancer routing.
- **TLS Certificate Provisioning**: Automatic ACME / TLS certificate issuance is handled by the edge proxy / infrastructure layer outside Core application boundaries.

---

## 9. Verification & Testing Summary

1. `GOWORK=off go test -v ./pkg/commerce/...`:
   - `TestValidateCustomDomain`: Green
   - `TestDNSFormatting` & `TestFakeTXTResolver`: Green
   - `TestStoreDomainLifecycleIntegration`: Green (All 7 subtests: Seller isolation, custom domain validation, DNS state machine, activation & primary switching, admin disable/enable fallback, platform enable fallback, concurrent activation safety).
2. `GOWORK=off go test -v ./internal/coreapi/...`:
   - `TestSpecMatchesRouter`: Green
3. `GOWORK=off go run ./cmd/openapi-gen`: Green (`docs/api/internal/openapi.json` generated and verified).
