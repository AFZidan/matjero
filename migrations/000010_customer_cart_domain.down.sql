DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS customer_addresses;
DROP TABLE IF EXISTS customers;

DROP INDEX IF EXISTS fulfillment_locations_store_market_idx;
DROP INDEX IF EXISTS fulfillment_locations_store_code_uidx;

ALTER TABLE fulfillment_locations
    DROP CONSTRAINT IF EXISTS fulfillment_locations_ownership_check,
    DROP CONSTRAINT IF EXISTS fulfillment_locations_store_market_fk,
    DROP COLUMN IF EXISTS store_id,
    ALTER COLUMN supplier_market_id SET NOT NULL,
    ALTER COLUMN supplier_id SET NOT NULL;
