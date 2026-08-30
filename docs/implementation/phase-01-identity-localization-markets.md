# Phase 1 Implementation Plan

## Scope

Phase 1 covers the platform identity, localization, and market-reference-data foundation. It stops before commerce workflows such as catalog, checkout, orders, payments, shipping, inventory reservations, and settlements.

## Deliverables

- ZITADEL-backed JWT/OIDC validation for actor-facing APIs.
- Principal mapping with coarse roles for admin, seller, and supplier access.
- Resource-authorization foundation at the transport boundary.
- Shared Arabic/English locale negotiation with RTL/LTR direction handling.
- Markets, countries, currencies, and market locales reference data.
- Initial Egypt, Saudi Arabia, and United Arab Emirates seed configuration.
- API bootstrap endpoints that expose the authenticated actor context and localized market list.
- Migration, repository, and integration test coverage for the Phase 1 schema.

## Implementation Notes

- Admin, seller, and supplier APIs use the shared OIDC verifier and role gate helpers.
- The storefront API uses the same localized market data without requiring authenticated access.
- Locale is negotiated centrally so backend responses and frontend shells can share the same direction and language contract.
- Market data is read from PostgreSQL, not hardcoded into the API layer.

## Validation Plan

- Run Go formatting, tests, build, and vet checks.
- Run the focused PostgreSQL integration test for seeded markets.
- Run frontend lint, typecheck, tests, and dependency audit.
- Validate Docker Compose configuration.
- Confirm the implementation report and architecture docs reflect the shipped behavior.
