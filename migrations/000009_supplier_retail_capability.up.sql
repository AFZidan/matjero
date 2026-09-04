-- Explicit 1:1 Supplier <-> Seller Retail Capability Affiliation.
--
-- Maps a wholesale Supplier profile to its direct retail Seller profile.
-- A Supplier profile has at most one linked Seller profile, and a Seller profile
-- has at most one linked Supplier profile. Stores remain strictly seller-owned
-- (stores.seller_id).

CREATE TABLE IF NOT EXISTS supplier_seller_affiliations (
    supplier_id UUID PRIMARY KEY REFERENCES suppliers(id) ON DELETE CASCADE,
    seller_id   UUID UNIQUE NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
