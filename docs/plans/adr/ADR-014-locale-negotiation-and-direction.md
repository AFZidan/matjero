# ADR-014: Locale Negotiation and Direction

## Status

Accepted

## Decision

The platform supports Arabic and English as the initial locale set. Request locale is resolved in this order: explicit `locale` query parameter, then `Accept-Language`, then the English default. Responses expose the negotiated language and direction so UI shells can render and hydrate consistently across backend and frontend surfaces.

## Consequences

Locale handling stays centralized in shared middleware and utility packages. Future languages can be added by expanding the supported locale set and negotiation table without rewriting the request pipeline.
