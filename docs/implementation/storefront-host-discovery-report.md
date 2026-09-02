# Core Storefront Host Discovery — Technical Report

## Overview
This document records the implementation of the Core authoritative Storefront host discovery capability (`GET /internal/v1/stores/{storeID}/storefront-host`).

This internal read-only capability provides the authoritative Storefront domain that actor applications (such as Seller management) require to construct Theme Preview URLs (`https://<storefront-host>/?theme_preview=<token>`) and open Storefront routes without relying on client-side domain synthesis (e.g., `store.code + ".matjero.com"`).

---

## 1. Objective & Service Boundaries
- **Route**: `GET /internal/v1/stores/{storeID}/storefront-host`
- **Audience**: Internal service callers (`Seller`, `Admin`). Excluded from public customer traffic and `Supplier` service actors.
- **Responsibility**: Core remains the single source of truth for `store_domains`, domain normalization, tenant resolution, primary-domain promotion, and custom domain lifecycle. Seller consumes the authoritative host returned by Core.

---

## 2. Authorization Policy
The endpoint strictly reuses Core's standard `authorizeStore(w, r)` security boundary:

```
Service Auth (Bearer token + Caller Header)
  ↓
Forwarded Authenticated Subject (X-Matjero-Subject)
  ↓
authorizeStore (GetStore + Tenant Ownership Check)
  ↓
Authorized Store ID
  ↓
GetActivePrimaryStoreDomain Lookup
```

- **Seller Callers**: The caller's authenticated subject (`X-Matjero-Subject`) is resolved to a Seller ID. If the target Store is not owned by the resolved Seller, Core returns a safe `404 Not Found` (`CodeNotFound`), preventing tenant probing across Sellers.
- **Admin Callers**: Permitted to read any store's storefront host.
- **Supplier Callers**: Expressly rejected at the router level with `403 Forbidden` (`CodeForbidden`).

---

## 3. Domain Selection & Filtering Rules
The database query enforces a strict canonical rule:

```sql
SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
FROM store_domains
WHERE store_id = $1
  AND is_primary = true
  AND status = 'active'
```

### Invariants:
1. **Active Primary Only**: Only domains with `is_primary = true` AND `status = 'active'` are returned.
2. **No Fallback Synthetic Guessing**: If no active primary domain exists, Core returns `404 Not Found` (`CodeNotFound`). Core does NOT:
   - Synthesize `<store-code>.<PlatformDomain>`;
   - Return inactive, pending, failed, or disabled primary domains;
   - Select an arbitrary active secondary domain.
3. **Determinism**: A store may have multiple domain records (e.g. secondary platform domain and active primary custom domain). The query uniquely selects the active primary domain defined by the `store_domains_one_primary_per_store` partial unique index.
4. **Future Compatibility**: When P4.8 domain management later promotes a custom domain to `active` + `is_primary = true`, this endpoint automatically returns the custom domain without any frontend or Seller code changes.

---

## 4. Response Shape & Privacy
The API response exposes only the normalized host needed for URL construction:

```json
{
  "host": "store.example.com"
}
```

### Privacy Guarantees:
- **No Scheme**: Host is a bare domain (e.g., `store.example.com`, NOT `https://store.example.com`).
- **No Sensitive Fields**: Response excludes internal database IDs (`id`, `store_id`, `seller_id`), lifecycle state (`domain_type`, `status`, `is_primary`), verification tokens (`verification_token`), and audit timestamps (`verified_at`, `last_checked_at`, `created_at`, `updated_at`).

---

## 5. OpenAPI Specification & Contract Tests
- **Internal Spec**: Added declaration to `internalRoutes()` in `internal/coreapi/spec.go` under tag `"Stores"`.
- **Generated Output**: Regenerated `docs/api/internal/openapi.json` via `cmd/openapi-gen`.
- **Router/Spec Drift Guard**: Verified via `TestSpecMatchesRouter` in `internal/coreapi/spec_test.go`.

---

## 6. Test Verification
Automated test suite (`internal/coreapi/storefront_host_test.go`) covers:
1. **Authorized Read**: Seller receiving valid normalized active primary domain.
2. **Cross-Seller Isolation**: Seller B requesting Store A receiving safe 404.
3. **Admin Resolution**: Admin caller successfully retrieving store storefront host.
4. **Unknown Store**: Non-existent store UUID returning 404.
5. **Supplier Rejection**: Supplier caller receiving 403 Forbidden.
6. **Domain Selection Rules**: Rejection of pending, disabled, verified-but-inactive, and active secondary domains.
7. **Determinism**: Preferred active primary custom domain over active secondary platform domain.
8. **Lifecycle Promotion**: Store starting with platform primary domain -> promoted custom domain -> endpoint immediately returning custom domain.
9. **Response Privacy**: Verification that JSON payload contains exactly `{"host": "..."}` and no extra internal columns.

---

## 7. Remaining Seller Bridge (Future Work)
When P4.7 UI work resumes in Seller:
- Seller will call `GET /internal/v1/stores/{storeID}/storefront-host` via its Core client.
- Seller will combine the returned `host` with its trusted public scheme configuration (e.g. `https://`) to construct the final Storefront Theme Preview URL:
  `https://<host>/?theme_preview=<token>`
