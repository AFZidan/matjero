-- Store domain lifecycle support.
-- Adds domain classification (platform vs custom), a DNS verification token,
-- a last-checked timestamp, constrained status values, an invariant enforcing
-- a single primary domain per store, and a lookup index on the domain column.

ALTER TABLE store_domains
    ADD COLUMN IF NOT EXISTS domain_type TEXT NOT NULL DEFAULT 'platform',
    ADD COLUMN IF NOT EXISTS verification_token TEXT,
    ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ;

ALTER TABLE store_domains
    DROP CONSTRAINT IF EXISTS store_domains_status_check;

ALTER TABLE store_domains
    ADD CONSTRAINT store_domains_status_check
    CHECK (status IN ('pending', 'verified', 'active', 'failed', 'disabled'));

ALTER TABLE store_domains
    ADD CONSTRAINT store_domains_domain_type_check
    CHECK (domain_type IN ('platform', 'custom'));

CREATE UNIQUE INDEX IF NOT EXISTS store_domains_one_primary_per_store
    ON store_domains (store_id) WHERE is_primary = true;

CREATE INDEX IF NOT EXISTS store_domains_domain_idx ON store_domains (domain);
