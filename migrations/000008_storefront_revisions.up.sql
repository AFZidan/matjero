-- Public storefront cache revisions.
--
-- One authoritative, monotonically increasing revision per store. It is the
-- cache generation of everything the public storefront renders for that store:
-- bootstrap, published theme, categories, browse, search and product detail.
-- Core bumps it in the same transaction as the business write that changes
-- public output, so a committed change is never visible under an old revision
-- and a rolled-back change never advances the revision.
--
-- The revision is opaque to consumers: it carries no business meaning beyond
-- "the public output of this store changed". A consumer that includes it in a
-- cache key never has to delete anything, because a bump moves every subsequent
-- lookup into a new namespace and the abandoned entries expire on their own.
-- That is what removes any need for a wildcard scan or key registry.
--
-- A store with no row here is treated as revision 1, so a store created by a
-- path that predates this table still resolves to a usable generation and the
-- first bump moves it to 2.

CREATE TABLE IF NOT EXISTS storefront_revisions (
    store_id   UUID PRIMARY KEY REFERENCES stores (id) ON DELETE CASCADE,
    revision   BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill every existing store so no storefront starts without a revision.
INSERT INTO storefront_revisions (store_id)
SELECT id FROM stores
ON CONFLICT (store_id) DO NOTHING;
