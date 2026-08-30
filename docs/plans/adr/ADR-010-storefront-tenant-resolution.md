# ADR-010: Storefront Tenant Resolution

## Status

Accepted

## Decision

Public storefront requests resolve tenant context from a trusted domain-to-store mapping. Storefront requests must scope catalog, cart, checkout, theme, settings, pricing, and availability to the resolved store.

## Deferred Details

Exact domain verification and custom-domain onboarding flows are implemented in the storefront phase.
