CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS suppliers (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS supplier_settings (
    supplier_id UUID PRIMARY KEY REFERENCES suppliers(id) ON DELETE CASCADE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS supplier_members (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    principal_subject TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_id, principal_subject)
);

CREATE TABLE IF NOT EXISTS supplier_markets (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    market_code CHAR(2) NOT NULL REFERENCES markets(code),
    status TEXT NOT NULL,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_id, market_code),
    UNIQUE (id, market_code),
    UNIQUE (id, supplier_id, market_code)
);

CREATE TABLE IF NOT EXISTS sellers (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS seller_settings (
    seller_id UUID PRIMARY KEY REFERENCES sellers(id) ON DELETE CASCADE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS seller_members (
    id UUID PRIMARY KEY,
    seller_id UUID NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
    principal_subject TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (seller_id, principal_subject)
);

CREATE TABLE IF NOT EXISTS stores (
    id UUID PRIMARY KEY,
    seller_id UUID NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
    market_code CHAR(2) NOT NULL REFERENCES markets(code),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (seller_id, code),
    UNIQUE (id, market_code)
);

CREATE TABLE IF NOT EXISTS store_domains (
    id UUID PRIMARY KEY,
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    domain TEXT NOT NULL UNIQUE,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS store_settings (
    store_id UUID PRIMARY KEY REFERENCES stores(id) ON DELETE CASCADE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS product_translations (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (product_id, locale)
);

CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY,
    parent_category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS category_translations (
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (category_id, locale)
);

CREATE TABLE IF NOT EXISTS variants (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, code)
);

CREATE TABLE IF NOT EXISTS skus (
    id UUID PRIMARY KEY,
    variant_id UUID NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    code TEXT NOT NULL UNIQUE,
    barcode TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (variant_id, code)
);

CREATE TABLE IF NOT EXISTS attributes (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS attribute_translations (
    attribute_id UUID NOT NULL REFERENCES attributes(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (attribute_id, locale)
);

CREATE TABLE IF NOT EXISTS attribute_values (
    id UUID PRIMARY KEY,
    attribute_id UUID NOT NULL REFERENCES attributes(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attribute_id, code)
);

CREATE TABLE IF NOT EXISTS attribute_value_translations (
    attribute_value_id UUID NOT NULL REFERENCES attribute_values(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (attribute_value_id, locale)
);

CREATE TABLE IF NOT EXISTS media_metadata (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    media_type TEXT NOT NULL,
    uri TEXT NOT NULL,
    alt_text TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS supplier_products (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    supplier_code TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_id, product_id),
    UNIQUE (supplier_id, supplier_code),
    UNIQUE (id, supplier_id)
);

CREATE TABLE IF NOT EXISTS supplier_offers (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    supplier_product_id UUID NOT NULL,
    supplier_market_id UUID NOT NULL,
    market_code CHAR(2) NOT NULL REFERENCES markets(code),
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_product_id, market_code),
    UNIQUE (id, market_code),
    CONSTRAINT supplier_offers_supplier_product_fk
        FOREIGN KEY (supplier_product_id, supplier_id)
        REFERENCES supplier_products (id, supplier_id),
    CONSTRAINT supplier_offers_supplier_market_fk
        FOREIGN KEY (supplier_market_id, supplier_id, market_code)
        REFERENCES supplier_markets (id, supplier_id, market_code)
);

CREATE TABLE IF NOT EXISTS supplier_offer_prices (
    id UUID PRIMARY KEY,
    supplier_offer_id UUID NOT NULL REFERENCES supplier_offers(id) ON DELETE CASCADE,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency_code CHAR(3) NOT NULL REFERENCES currencies(code),
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_offer_id)
);

CREATE TABLE IF NOT EXISTS supplier_offer_availability (
    id UUID PRIMARY KEY,
    supplier_offer_id UUID NOT NULL UNIQUE REFERENCES supplier_offers(id) ON DELETE CASCADE,
    is_available BOOLEAN NOT NULL DEFAULT true,
    available_qty BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (available_qty IS NULL OR available_qty >= 0)
);

CREATE TABLE IF NOT EXISTS seller_listings (
    id UUID PRIMARY KEY,
    store_id UUID NOT NULL,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    supplier_offer_id UUID,
    market_code CHAR(2) NOT NULL REFERENCES markets(code),
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, market_code),
    CONSTRAINT seller_listings_store_fk
        FOREIGN KEY (store_id, market_code)
        REFERENCES stores (id, market_code),
    CONSTRAINT seller_listings_offer_fk
        FOREIGN KEY (supplier_offer_id, market_code)
        REFERENCES supplier_offers (id, market_code)
);

CREATE TABLE IF NOT EXISTS seller_listing_prices (
    id UUID PRIMARY KEY,
    seller_listing_id UUID NOT NULL UNIQUE REFERENCES seller_listings(id) ON DELETE CASCADE,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency_code CHAR(3) NOT NULL REFERENCES currencies(code),
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fulfillment_locations (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    supplier_market_id UUID NOT NULL,
    market_code CHAR(2) NOT NULL REFERENCES markets(code),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    location_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_market_id, code),
    UNIQUE (id, market_code),
    CONSTRAINT fulfillment_locations_supplier_market_fk
        FOREIGN KEY (supplier_market_id, supplier_id, market_code)
        REFERENCES supplier_markets (id, supplier_id, market_code)
);

CREATE TABLE IF NOT EXISTS inventory_snapshots (
    id UUID PRIMARY KEY,
    fulfillment_location_id UUID NOT NULL REFERENCES fulfillment_locations(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    on_hand_qty BIGINT NOT NULL DEFAULT 0,
    reserved_qty BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (fulfillment_location_id, sku_id),
    CHECK (on_hand_qty >= 0),
    CHECK (reserved_qty >= 0),
    CHECK (reserved_qty <= on_hand_qty)
);

CREATE TABLE IF NOT EXISTS inventory_reservations (
    id UUID PRIMARY KEY,
    inventory_snapshot_id UUID NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    status TEXT NOT NULL,
    reservation_token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
