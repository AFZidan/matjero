-- Reverse migration for 000008_storefront_revisions.
--
-- Revisions are cache generations derived from commerce state, not a source of
-- truth, so dropping the table loses nothing: the next migration up backfills
-- every store again and consumers simply observe a new generation.

DROP TABLE IF EXISTS storefront_revisions;
