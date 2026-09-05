-- Phase 5.2: Checkout Sessions and server-computed finalization fingerprints.

CREATE TABLE checkout_sessions (
    id UUID PRIMARY KEY,
    store_id UUID NOT NULL,
    cart_id UUID NOT NULL,
    customer_id UUID,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    shipping_address_snapshot JSONB,
    contact_email TEXT,
    finalize_fingerprint TEXT,
    guest_order_access_token_digest BYTEA NOT NULL,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT checkout_sessions_status_check
        CHECK (status IN ('open', 'finalized', 'expired')),
    CONSTRAINT checkout_sessions_guest_digest_check
        CHECK (octet_length(guest_order_access_token_digest) = 32),
    CONSTRAINT checkout_sessions_id_store_key UNIQUE (id, store_id),
    CONSTRAINT checkout_sessions_store_fk
        FOREIGN KEY (store_id)
        REFERENCES stores (id)
        ON DELETE RESTRICT,
    CONSTRAINT checkout_sessions_cart_store_fk
        FOREIGN KEY (cart_id, store_id)
        REFERENCES carts (id, store_id)
        ON DELETE RESTRICT,
    CONSTRAINT checkout_sessions_customer_store_fk
        FOREIGN KEY (customer_id, store_id)
        REFERENCES customers (id, store_id)
        ON DELETE RESTRICT
);

CREATE INDEX checkout_sessions_cart_idx
    ON checkout_sessions (cart_id, created_at DESC, id DESC);

CREATE INDEX checkout_sessions_status_expiry_idx
    ON checkout_sessions (status, expires_at, id);
