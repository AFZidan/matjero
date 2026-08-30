# ADR-004: Fulfillment Location

## Status

Accepted

## Decision

Fulfillment Location is a first-class domain concept. Inventory belongs to SKU plus Fulfillment Location, not directly to a Seller Listing.

## Consequences

Supplier operations can later support warehouses, branches, 3PLs, and platform warehouses without redesigning inventory ownership.
