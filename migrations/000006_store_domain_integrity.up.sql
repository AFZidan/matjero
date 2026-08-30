-- Enforce canonical lowercase domain uniqueness.
--
-- The original 000003 migration created a case-sensitive UNIQUE(domain) constraint
-- (store_domains_domain_key) and 000005 added a redundant non-unique lookup index
-- (store_domains_domain_idx). Replace both with a single case-insensitive unique
-- index on lower(domain) so that Shop.Example.com and shop.example.com cannot
-- coexist.
--
-- Defensive: normalize any existing domain values to canonical lowercase before
-- enforcing uniqueness. This repository has no production data, so no
-- case-insensitive duplicates are expected; the UPDATE is a safety net that also
-- prevents the subsequent unique index from failing on pre-existing mixed-case
-- rows.

UPDATE store_domains SET domain = lower(trim(domain)) WHERE domain IS NOT NULL AND domain <> lower(trim(domain));

ALTER TABLE store_domains DROP CONSTRAINT IF EXISTS store_domains_domain_key;
DROP INDEX IF EXISTS store_domains_domain_idx;

CREATE UNIQUE INDEX store_domains_domain_key ON store_domains (lower(domain));
