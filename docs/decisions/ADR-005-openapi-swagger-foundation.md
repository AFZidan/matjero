# ADR-005: OpenAPI / Swagger Foundation

## Status

Accepted

## Context

Matjero now has four actor-facing APIs with shared `/v1` routing, shared commerce models, and a growing contract surface. The team needs a durable API contract that:

- stays aligned with the actual Go routes,
- supports per-application Swagger UI,
- works across admin, seller, supplier, and storefront APIs,
- and blocks contract drift in CI.

The repo already uses chi/httpx for routing and has no OpenAPI subsystem.

## Decision

- Keep the existing chi/httpx stack.
- Generate OpenAPI 3.1 documents from Go-owned route metadata and shared DTOs.
- Publish one committed spec per application under `docs/api/*/openapi.json`.
- Serve `/openapi.json` and `/docs` from each app when `OPENAPI_DOCS_ENABLED` is enabled.
- Use bearer authentication security schemes in authenticated specs only.
- Require OpenAPI generation, validation, and stale-spec detection in CI.

## Consequences

Positive:

- Specs stay close to runtime behavior.
- Swagger UI becomes available without replacing the HTTP framework.
- Contract drift is visible in CI.
- Future phases inherit a permanent documentation gate.

Tradeoffs:

- The route catalog must be maintained alongside route code.
- A generated spec can still drift if route metadata is changed carelessly, so tests remain important.
- Breaking-change detection is not yet fully automated and remains a follow-up if needed.

## Notes

The selected approach is intentionally incremental. It preserves the current architecture and avoids a framework swap just to satisfy documentation tooling.
