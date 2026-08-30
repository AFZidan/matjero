# OpenAPI / Swagger Documentation Foundation

## Purpose

Establish OpenAPI 3.1 as the canonical API contract format for Matjero and make the generated Swagger UI available per application without replacing the current chi/httpx HTTP stack.

This phase covers:

- `admin-api`
- `seller-api`
- `supplier-api`
- `storefront-api`

## Selected Strategy

- Keep the existing chi router and `/v1` actor routes.
- Generate OpenAPI from code-owned route metadata and shared request/response DTOs.
- Serve one spec per API application.
- Expose `/openapi.json` and `/docs` when enabled.
- Keep docs disabled by default in production unless explicitly enabled.

## Generation Mechanism

- A dedicated Go generator builds the four specs from the route catalog.
- The generator uses the same contract types as the runtime JSON payloads where possible.
- The output is deterministic and committed under `docs/api/*/openapi.json`.

## Swagger UI Architecture

- Each application mounts a small docs router.
- `/openapi.json` serves the generated spec for that application.
- `/docs` serves Swagger UI for the same spec.
- Docs exposure is controlled with `OPENAPI_DOCS_ENABLED`.

## API Versioning

- Existing external API routes remain under `/v1/...`.
- The OpenAPI docs document the current versioned routes without introducing a parallel unversioned surface.

## Domain Tagging

The specs use OpenAPI tags to group operations by domain, including:

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

Only tags used by a given API are attached to that API’s operations.

## Spec Locations

- `docs/api/admin/openapi.json`
- `docs/api/seller/openapi.json`
- `docs/api/supplier/openapi.json`
- `docs/api/storefront/openapi.json`

## Security Model

- Admin, seller, and supplier specs document ZITADEL/OIDC bearer authentication.
- Storefront routes remain public in the spec.
- The docs never include secrets, tokens, or private configuration.

## CI Validation

CI now:

1. Generates all four specs.
2. Validates the documents.
3. Lints the documents with the OpenAPI test suite.
4. Fails if the committed specs drift from the generator output.

## Testing

- Spec generation succeeds for all four APIs.
- Generated specs validate with `kin-openapi`.
- Swagger UI endpoints respond when enabled.
- Docs endpoints are absent when disabled.
- Important routes appear in the correct API spec.
- Tags and auth requirements are present where expected.
- Generated output is deterministic.

## Future-Phase Requirements

Any future phase that changes:

- endpoints
- request schemas
- response schemas
- authentication
- pagination
- filtering

must also update the generated OpenAPI specs and pass the OpenAPI CI checks.

## Acceptance Criteria

- Four committed OpenAPI specs exist under `docs/api/*/openapi.json`.
- Each API can expose `/openapi.json` and `/docs` when enabled.
- CI blocks stale or invalid specs.
- The current HTTP stack remains intact.
- The new OpenAPI requirement is captured in the project Definition of Done.
