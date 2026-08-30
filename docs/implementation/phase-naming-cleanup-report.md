# Phase Naming Cleanup Report

## Scope

This cleanup removes phase-history naming from production and technical files where it was still present, while preserving historical documentation under `docs/implementation/` and planning docs.

## Initial Audit

Repo-wide filename scan on the continuation branch showed no remaining phase-named production filenames. The only filenames containing phase terminology were the historical implementation docs:

- `docs/implementation/phase-00-engineering-architecture-foundation.md`
- `docs/implementation/phase-00-implementation-report.md`
- `docs/implementation/phase-01-implementation-report.md`
- `docs/implementation/phase-01-identity-localization-markets.md`
- `docs/implementation/phase-02-commerce-domain-foundation.md`
- `docs/implementation/phase-02-implementation-report.md`
- `docs/implementation/phase-03-implementation-report.md`

These are intentionally retained because they are historical planning and implementation artifacts.

## Production File Naming

No additional production source filenames required renaming in this pass. The previously phase-named migration files were already renamed on the branch to responsibility-based names:

- `migrations/000001_phase0_foundation.up.sql` -> `migrations/000001_event_delivery_foundation.up.sql`
- `migrations/000001_phase0_foundation.down.sql` -> `migrations/000001_event_delivery_foundation.down.sql`
- `migrations/000002_phase1_identity_localization_markets.up.sql` -> `migrations/000002_market_reference_data.up.sql`
- `migrations/000002_phase1_identity_localization_markets.down.sql` -> `migrations/000002_market_reference_data.down.sql`
- `migrations/000003_phase2_commerce_domain_foundation.up.sql` -> `migrations/000003_commerce_domain_schema.up.sql`
- `migrations/000003_phase2_commerce_domain_foundation.down.sql` -> `migrations/000003_commerce_domain_schema.down.sql`

The `000004` migration was reviewed and already had a responsibility-based name:

- `migrations/000004_admin_supplier_seller_platforms.up.sql`
- `migrations/000004_admin_supplier_seller_platforms.down.sql`

Because the migration framework tracks versions from the numeric prefix, keeping `000004` unchanged preserves execution order and replay behavior. No semantic changes were made to that migration.

## Additional Identifier Cleanup

Several non-filename historical labels were normalized where it was safe and low risk:

- `Makefile`
  - `commerce-*-api:phase0` image tags -> `commerce-*-api:foundation`
- `.github/workflows/ci.yml`
  - migration file checks updated to the renamed `000001_event_delivery_foundation.*` paths
- `migrations/000002_market_reference_data.up.sql`
  - seed `configuration` JSON changed from `{"phase":"phase1"}` to `{"release_track":"launch"}`
- `internal/actorapi/router_test.go`
  - test fixture configuration key changed from `phase` to `release_track`

These changes remove historical phase labels from technical configuration without changing runtime behavior.

## Test Infrastructure Fix

### Root Cause

`go test ./internal/markets ./internal/commerce` could deadlock because both integration tests were connecting to the same PostgreSQL database and applying the same migrations concurrently. The packages were fighting over shared DDL and migration work.

### Fix

A new helper was added at `internal/testdb/testdb.go` to give each integration test package its own isolated PostgreSQL schema:

- create a unique schema per test package
- set `search_path` to that schema for the test pool
- keep the shared `pgcrypto` extension available
- drop the isolated schema during cleanup

Both integration tests now use `internal/testdb.Open(...)` instead of sharing the default schema.

## Migration Compatibility Analysis

The migration renames preserved numeric prefixes and file pairings:

- same version numbers
- same up/down pairing
- no content changes to the core schema migrations
- no change to execution order

This means migration tracking remains compatible with the existing version-based migration flow.

## Final Remaining Phase-Named Files

Only historical documentation files remain phase-named:

- `docs/implementation/phase-00-engineering-architecture-foundation.md`
- `docs/implementation/phase-00-implementation-report.md`
- `docs/implementation/phase-01-implementation-report.md`
- `docs/implementation/phase-01-identity-localization-markets.md`
- `docs/implementation/phase-02-commerce-domain-foundation.md`
- `docs/implementation/phase-02-implementation-report.md`
- `docs/implementation/phase-03-implementation-report.md`

No non-documentation filenames with phase terminology remain.

## Validation

Passed:

- `go test ./internal/markets ./internal/commerce ./internal/actorapi`
- `go test ./...`
- `go vet ./...`
- `go build ./...`
- `npm run lint`
- `npm run typecheck`
- `npm run test`
- `npm run build --workspaces --if-present`
- `npm audit --audit-level=high`
- `docker compose config --quiet`
- `make migrate-check`

PostgreSQL concurrency verification:

- the targeted integration packages were run again after the schema isolation fix
- the earlier package-parallel deadlock no longer reproduced
- normal `go test ./...` passed without `-p 1`

## Notes

- Historical documentation under `docs/implementation/` was intentionally preserved.
- No production Go source files were renamed or split in this continuation because no phase-named production filenames remained on the branch.
