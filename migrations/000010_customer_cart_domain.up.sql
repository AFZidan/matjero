-- Phase 5.1: seller-owned locations, lightweight Customers, and Carts.

ALTER TABLE fulfillment_locations
    ALTER COLUMN supplier_id DROP NOT NULL,
    ALTER COLUMN supplier_market_id DROP NOT NULL;

ALTER TABLE fulfillment_locations
    ADD COLUMN store_id UUID;

ALTER TABLE fulfillment_locations
    ADD CONSTRAINT fulfillment_locations_store_market_fk
        FOREIGN KEY (store_id, market_code)
        REFERENCES stores (id, market_code)
        ON DELETE RESTRICT,
    ADD CONSTRAINT fulfillment_locations_ownership_check
        CHECK (
            (supplier_id IS NOT NULL AND supplier_market_id IS NOT NULL AND store_id IS NULL)
            OR
            (supplier_id IS NULL AND supplier_market_id IS NULL AND store_id IS NOT NULL)
        );

CREATE UNIQUE INDEX fulfillment_locations_store_code_uidx
    ON fulfillment_locations (store_id, code)
    WHERE store_id IS NOT NULL;

CREATE INDEX fulfillment_locations_store_market_idx
    ON fulfillment_locations (store_id, market_code)
    WHERE store_id IS NOT NULL;

CREATE TABLE customers (
    id UUID PRIMARY KEY,
    store_id UUID NOT NULL,
    market_code CHAR(2) NOT NULL,
    identity_provider TEXT,
    identity_subject TEXT,
    email TEXT,
    display_name TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT customers_status_check CHECK (status IN ('active', 'blocked')),
    CONSTRAINT customers_store_market_fk
        FOREIGN KEY (store_id, market_code)
        REFERENCES stores (id, market_code)
        ON DELETE RESTRICT,
    CONSTRAINT customers_id_store_key UNIQUE (id, store_id)
);

CREATE UNIQUE INDEX customers_identity_uidx
    ON customers (store_id, identity_provider, identity_subject)
    WHERE identity_provider IS NOT NULL AND identity_subject IS NOT NULL;

CREATE INDEX customers_store_idx ON customers (store_id, created_at DESC, id DESC);

CREATE TABLE customer_addresses (
    id UUID PRIMARY KEY,
    customer_id UUID NOT NULL,
    store_id UUID NOT NULL,
    label TEXT,
    recipient_name TEXT NOT NULL,
    phone TEXT,
    address_line_1 TEXT NOT NULL,
    address_line_2 TEXT,
    city TEXT NOT NULL,
    region TEXT,
    postal_code TEXT,
    country_code CHAR(2) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT customer_addresses_customer_store_fk
        FOREIGN KEY (customer_id, store_id)
        REFERENCES customers (id, store_id)
        ON DELETE CASCADE
);

CREATE INDEX customer_addresses_customer_idx
    ON customer_addresses (customer_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX customer_addresses_one_default_uidx
    ON customer_addresses (customer_id, store_id)
    WHERE is_default;

CREATE TABLE carts (
    id UUID PRIMARY KEY,
    store_id UUID NOT NULL,
    market_code CHAR(2) NOT NULL,
    customer_id UUID,
    cart_token_digest TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT carts_status_check CHECK (status IN ('active', 'checked_out', 'abandoned', 'expired')),
    CONSTRAINT carts_store_market_fk
        FOREIGN KEY (store_id, market_code)
        REFERENCES stores (id, market_code)
        ON DELETE RESTRICT,
    CONSTRAINT carts_id_store_key UNIQUE (id, store_id),
    CONSTRAINT carts_customer_store_fk
        FOREIGN KEY (customer_id, store_id)
        REFERENCES customers (id, store_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX carts_active_customer_uidx
    ON carts (customer_id, store_id)
    WHERE status = 'active' AND customer_id IS NOT NULL;

CREATE INDEX carts_store_status_idx ON carts (store_id, status, updated_at DESC);

CREATE TABLE cart_items (
    id UUID PRIMARY KEY,
    cart_id UUID NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    seller_listing_id UUID NOT NULL REFERENCES seller_listings (id) ON DELETE RESTRICT,
    sku_id UUID NOT NULL REFERENCES skus (id) ON DELETE RESTRICT,
    quantity BIGINT NOT NULL CHECK (quantity > 0 AND quantity <= 10000),
    expected_unit_price_minor BIGINT NOT NULL CHECK (expected_unit_price_minor >= 0),
    expected_currency_code CHAR(3) NOT NULL REFERENCES currencies (code),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT cart_items_identity_uidx UNIQUE (cart_id, seller_listing_id, sku_id)
);

CREATE INDEX cart_items_cart_order_idx
    ON cart_items (cart_id, seller_listing_id, sku_id);
