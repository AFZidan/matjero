# OpenAPI / Swagger Implementation Report

## Executive Summary

Implemented a dedicated OpenAPI 3.1 foundation for Matjero’s four API applications:

- `admin-api`
- `seller-api`
- `supplier-api`
- `storefront-api`

The solution keeps the existing chi/httpx stack, generates deterministic specs from Go-owned route metadata, serves Swagger UI per application, and adds CI checks that fail on stale specs.

## Selected Architecture

- Existing chi routing remains in place.
- OpenAPI documents are generated from code-owned route metadata and shared contract DTOs.
- One spec is produced per API application.
- Swagger UI is mounted per app and points at that app’s own spec.
- Documentation exposure is controlled by `OPENAPI_DOCS_ENABLED`.

## Libraries and Tools

- `github.com/getkin/kin-openapi/openapi3` for OpenAPI document construction and validation.
- `github.com/swaggo/http-swagger/v2` for Swagger UI serving.
- A dedicated Go generator at `cmd/openapi-gen`.

## APIs Documented

- Admin API
- Seller API
- Supplier API
- Storefront API

Each spec includes the current `/v1/...` routes, operation IDs, tags, parameters, request bodies, response bodies, and security requirements where applicable.

## Swagger UI Endpoints

When enabled:

- `/docs`
- `/openapi.json`

These are exposed separately for each application.

## OpenAPI Spec Paths

- `docs/api/admin/openapi.json`
- `docs/api/seller/openapi.json`
- `docs/api/supplier/openapi.json`
- `docs/api/storefront/openapi.json`

## Domain Tags

The specs use domain tags such as:

- `Identity & Access`
- `Markets`
- `Suppliers`
- `Sellers`
- `Stores`
- `Catalog`
- `Categories`
- `Attributes`
- `Variants`
- `SKUs`
- `Fulfillment Locations`
- `Supplier Offers`
- `Seller Listings`
- `Inventory`
- `Audit`

## API Versioning Decision

- Existing actor-facing APIs stay versioned under `/v1/...`.
- The docs describe the current versioned routes rather than introducing a parallel unversioned surface.

## Authentication Documentation

- Admin, seller, and supplier specs document bearer auth via a ZITADEL/OIDC security scheme.
- Storefront remains public in the spec.
- No credentials, secrets, or tokens are embedded in generated output.

## CI Changes

The backend CI job now:

1. Generates all four OpenAPI specs.
2. Validates the generated specs.
3. Fails if the committed `docs/api/*` output is stale.

The repo also keeps the existing Go, Node, and Docker validation steps.

## Validation Commands

Completed successfully:

- `go run ./cmd/openapi-gen`
- `go test ./internal/openapi`
- `go test ./...`
- `go vet ./...`
- `npm run lint`
- `npm run typecheck`
- `npm run test`
- `npm audit --audit-level=high`
- `docker compose config --quiet`

## Test Results

- OpenAPI generation succeeds for all four specs.
- Generated specs validate with `kin-openapi`.
- Swagger UI responds when enabled.
- Docs can be disabled through configuration.
- Security schemes are present for authenticated APIs.
- Important routes are present in the correct specs.
- Generated specs are deterministic.

## Bugs Found and Fixed

- Fixed Swagger UI serving so `/docs` renders the UI directly.
- Fixed OpenAPI response description handling to match the library’s pointer-based response model.
- Added shared runtime error response types so the docs generator can reflect the actual JSON error shape.

## Architecture Changes

- Added a shared OpenAPI generator package.
- Added per-app docs mounting in the four API entrypoints.
- Added a docs exposure config flag to the shared config loader.
- Added committed generated specs under `docs/api/*`.
- Added a permanent OpenAPI requirement to the project Definition of Done.

## Deferred Items

- Lightweight breaking-change detection is intentionally left as a follow-up.
- The current phase does not replace the HTTP framework or introduce a spec-first code path.

## Known Limitations

- Route metadata still needs to be kept in sync with route code.
- The current phase does not automatically compare against a historical baseline for breaking changes.

## Requirements Imposed on Future Phases

Any future phase that changes endpoints, request or response schemas, authentication, pagination, or filtering must also regenerate and validate the OpenAPI specs and pass the OpenAPI CI checks.

## What Is Next

1. Keep the specs updated as the APIs evolve.
2. Add a lightweight breaking-change checker when the repository has a stable enough baseline to compare against.
3. Reuse the same phase gate for future API changes so the contract stays synchronized with runtime behavior.
