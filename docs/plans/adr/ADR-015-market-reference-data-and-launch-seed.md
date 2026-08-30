# ADR-015: Market Reference Data and Launch Seed

## Status

Accepted

## Decision

Markets are represented as durable reference data with countries, currencies, market locales, and localized country names. The initial launch seed is Egypt, Saudi Arabia, and the United Arab Emirates, with Arabic and English locale coverage.

## Consequences

Market bootstrap data is stored in PostgreSQL and can be queried by actor-facing APIs for startup hydration. Adding a new market should be performed through migration and seed data rather than a one-off code path.
