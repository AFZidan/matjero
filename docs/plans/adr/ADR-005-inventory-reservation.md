# ADR-005: Inventory Reservation

## Status

Accepted

## Decision

Inventory reservation is strongly consistent and must be implemented transactionally in PostgreSQL. The preferred initial strategy is an atomic conditional update against an inventory snapshot row, with row-level locking for more complex multi-row workflows when justified.

## Deferred Details

Exact reservation tables, expiry behavior, and concurrency tests are implemented in the inventory phase.
