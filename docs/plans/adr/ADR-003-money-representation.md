# ADR-003: Money Representation

## Status

Accepted

## Decision

Money is never represented with floating-point values. Monetary values use `amount_minor` plus ISO currency when currency precision is standard. More complex currencies or calculations require an explicit decimal model and centralized rounding rules.

## Consequences

Go code must avoid `float32` and `float64` for monetary calculations. Database schemas must store currency and deterministic numeric representation.
