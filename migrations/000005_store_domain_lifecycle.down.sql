-- Rollback for 000005_store_domain_lifecycle.

ALTER TABLE store_domains
    DROP CONSTRAINT IF EXISTS store_domains_domain_type_check;

ALTER TABLE store_domains
    DROP CONSTRAINT IF EXISTS store_domains_status_check;

DROP INDEX IF EXISTS store_domains_one_primary_per_store;

DROP INDEX IF EXISTS store_domains_domain_idx;

ALTER TABLE store_domains
    DROP COLUMN IF EXISTS domain_type,
    DROP COLUMN IF EXISTS verification_token,
    DROP COLUMN IF EXISTS last_checked_at;
