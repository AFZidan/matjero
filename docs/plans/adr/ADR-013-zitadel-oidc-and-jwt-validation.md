# ADR-013: ZITADEL OIDC and JWT Validation

## Status

Accepted

## Decision

Actor-facing APIs validate ZITADEL-issued OIDC tokens at the transport boundary using issuer discovery and JWKS-backed JWT verification. Each service configures its own audience, converts validated claims into a request principal, and extracts coarse role membership from the ZITADEL project roles claim.

## Consequences

Admin, seller, and supplier APIs can enforce service-specific access rules without duplicating authentication logic. Resource authorization remains separate from JWT validation and will be layered into commerce use cases later.
