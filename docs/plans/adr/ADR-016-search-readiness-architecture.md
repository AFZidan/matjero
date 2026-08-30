# ADR-016: Search Readiness Architecture

## Status

Accepted

## Decision

Matjero will keep search as a derived read model. Phase 2 will design Commerce Core entities and domain events so a dedicated search indexer can be introduced later without redesigning core write paths.

Search will not be a Phase 2 runtime dependency. PostgreSQL remains the source of truth, and write operations must succeed even if no search index exists. Initial search implementations may use PostgreSQL where that is appropriate, but a future dedicated search engine may be introduced later for product, store, and supplier discovery.

## Consequences

- Products, stores, suppliers, categories, attributes, variants, SKUs, supplier offers, and seller listings must expose stable IDs and projection-friendly fields.
- Normalized translations remain the source for multilingual indexing.
- Domain events must preserve enough change metadata for incremental indexing.
- A future Search Indexer can consume Commerce Core events and build dedicated search documents without requiring Commerce Core schema redesign.
- No `search-api` microservice is introduced in Phase 2.
