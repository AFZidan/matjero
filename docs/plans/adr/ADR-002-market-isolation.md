# ADR-002: Market Isolation

## Status

Accepted

## Decision

The platform enforces `Store Market = Seller Listing Market = Supplier Offer Market` in domain logic and PostgreSQL constraints. Each Seller Store belongs to exactly one Market. Cross-border supplier sourcing is unsupported initially.

## Consequences

Schema design must carry `market_id` through market-scoped entities. Future listing constraints will use composite foreign keys to prevent invalid cross-market combinations.
