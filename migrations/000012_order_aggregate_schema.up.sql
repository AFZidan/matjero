-- Phase 5.3: Order Aggregate, Sequences, and State Machine foundation.

CREATE TABLE store_order_sequences (
    store_id UUID PRIMARY KEY REFERENCES stores (id) ON DELETE RESTRICT,
    next_value BIGINT NOT NULL DEFAULT 100001
);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    order_number TEXT NOT NULL,
    store_id UUID NOT NULL,
    market_code CHAR(2) NOT NULL,
    customer_id UUID,
    checkout_session_id UUID NOT NULL,
    status TEXT NOT NULL,
    currency_code CHAR(3) NOT NULL REFERENCES currencies (code),
    guest_order_access_token_digest BYTEA,
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    confirmation_deadline_at TIMESTAMPTZ NOT NULL,
    cancellation_reason TEXT,
    aggregate_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orders_status_check
        CHECK (status IN (
            'pending',
            'confirmed',
            'processing',
            'ready_for_shipping',
            'shipped',
            'out_for_delivery',
            'delivered',
            'cancelled',
            'returned'
        )),
    CONSTRAINT orders_guest_digest_check
        CHECK (
            customer_id IS NOT NULL
            OR (
                guest_order_access_token_digest IS NOT NULL
                AND octet_length(guest_order_access_token_digest) = 32
            )
        ),
    CONSTRAINT orders_store_order_number_uidx UNIQUE (store_id, order_number),
    CONSTRAINT orders_checkout_session_id_uidx UNIQUE (checkout_session_id),
    CONSTRAINT orders_id_store_key UNIQUE (id, store_id),
    CONSTRAINT orders_id_currency_key UNIQUE (id, currency_code),
    CONSTRAINT orders_store_market_fk
        FOREIGN KEY (store_id, market_code)
        REFERENCES stores (id, market_code)
        ON DELETE RESTRICT,
    CONSTRAINT orders_customer_store_fk
        FOREIGN KEY (customer_id, store_id)
        REFERENCES customers (id, store_id)
        ON DELETE RESTRICT,
    CONSTRAINT orders_checkout_session_store_fk
        FOREIGN KEY (checkout_session_id, store_id)
        REFERENCES checkout_sessions (id, store_id)
        ON DELETE RESTRICT
);

CREATE INDEX orders_status_deadline_idx
    ON orders (status, confirmation_deadline_at);

CREATE INDEX orders_store_status_idx
    ON orders (store_id, status, created_at DESC);

CREATE TABLE order_items (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    seller_listing_id UUID REFERENCES seller_listings (id) ON DELETE SET NULL,
    product_id UUID REFERENCES products (id) ON DELETE SET NULL,
    variant_id UUID REFERENCES variants (id) ON DELETE SET NULL,
    sku_id UUID REFERENCES skus (id) ON DELETE SET NULL,
    supplier_offer_id UUID REFERENCES supplier_offers (id) ON DELETE RESTRICT,
    source_supplier_id UUID REFERENCES suppliers (id) ON DELETE RESTRICT,
    fulfillment_location_id UUID NOT NULL REFERENCES fulfillment_locations (id) ON DELETE RESTRICT,
    inventory_reservation_id UUID NOT NULL UNIQUE REFERENCES inventory_reservations (id) ON DELETE RESTRICT,
    product_title_snapshot TEXT NOT NULL,
    sku_code_snapshot TEXT NOT NULL,
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    currency_code CHAR(3) NOT NULL REFERENCES currencies (code),
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    line_total_minor BIGINT NOT NULL CHECK (line_total_minor >= 0),
    supplier_cost_minor BIGINT CHECK (supplier_cost_minor IS NULL OR supplier_cost_minor >= 0),
    supplier_cost_currency_code CHAR(3) REFERENCES currencies (code),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT order_items_order_currency_fk
        FOREIGN KEY (order_id, currency_code)
        REFERENCES orders (id, currency_code)
        ON DELETE RESTRICT,
    CONSTRAINT order_items_supplier_snapshot_check
        CHECK (
            (
                supplier_offer_id IS NULL
                AND source_supplier_id IS NULL
                AND supplier_cost_minor IS NULL
                AND supplier_cost_currency_code IS NULL
            )
            OR
            (
                supplier_offer_id IS NOT NULL
                AND source_supplier_id IS NOT NULL
                AND supplier_cost_minor IS NOT NULL
                AND supplier_cost_currency_code IS NOT NULL
            )
        ),
    CONSTRAINT order_items_supplier_cost_currency_check
        CHECK (
            supplier_cost_currency_code IS NULL
            OR supplier_cost_currency_code = currency_code
        )
);

CREATE INDEX order_items_order_id_idx
    ON order_items (order_id);

CREATE TABLE order_addresses (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL UNIQUE REFERENCES orders (id) ON DELETE RESTRICT,
    address_type TEXT NOT NULL CHECK (address_type IN ('shipping')),
    recipient_name TEXT NOT NULL,
    phone TEXT,
    address_line_1 TEXT NOT NULL,
    address_line_2 TEXT,
    city TEXT NOT NULL,
    region TEXT,
    postal_code TEXT,
    country_code CHAR(2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_timeline (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders (id) ON DELETE RESTRICT,
    from_status TEXT,
    to_status TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('checkout', 'customer', 'seller', 'admin', 'scheduler', 'system')),
    actor_subject TEXT,
    reason TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_timeline_order_id_idx
    ON order_timeline (order_id, created_at ASC);

CREATE TABLE order_notes (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders (id) ON DELETE RESTRICT,
    author_subject TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal')),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_notes_order_id_idx
    ON order_notes (order_id, created_at DESC);
