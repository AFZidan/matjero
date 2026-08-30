-- Restore the pre-000006 case-sensitive uniqueness model: a non-unique lookup
-- index plus a case-sensitive UNIQUE(domain) constraint. Domain values were
-- normalized to lowercase by the up migration, so the case-sensitive constraint
-- remains valid.

DROP INDEX IF EXISTS store_domains_domain_key;

CREATE UNIQUE INDEX store_domains_domain_idx ON store_domains (domain);
ALTER TABLE store_domains ADD CONSTRAINT store_domains_domain_key UNIQUE (domain);
