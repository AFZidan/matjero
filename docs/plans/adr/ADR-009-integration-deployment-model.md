# ADR-009: Integration Deployment Model

## Status

Accepted

## Decision

Supplier and seller integrations are independently deployable applications, even when they target the same provider. Shared libraries may contain generic infrastructure and small provider utilities, but synchronization workflows remain separate.

## Consequences

There is no mandatory central Salla, Shopify, or WooCommerce runtime service. Release coupling between supplier and seller integration workflows is avoided.
