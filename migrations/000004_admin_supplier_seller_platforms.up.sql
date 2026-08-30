CREATE TABLE IF NOT EXISTS product_categories (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, category_id)
);

CREATE INDEX IF NOT EXISTS product_categories_category_idx
    ON product_categories (category_id, product_id);

CREATE TABLE IF NOT EXISTS inventory_movements (
    id UUID PRIMARY KEY,
    inventory_snapshot_id UUID NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    movement_type TEXT NOT NULL,
    quantity_delta BIGINT NOT NULL,
    on_hand_qty BIGINT NOT NULL,
    reserved_qty BIGINT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    principal_subject TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS inventory_movements_snapshot_created_idx
    ON inventory_movements (inventory_snapshot_id, created_at DESC, id DESC);
