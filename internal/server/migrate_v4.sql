ALTER TABLE orders ADD COLUMN IF NOT EXISTS app_id text NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS username text NOT NULL DEFAULT '';
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_merchant_id_out_trade_no_key;
CREATE UNIQUE INDEX IF NOT EXISTS orders_app_out_trade_no ON orders(merchant_id,app_id,out_trade_no);
UPDATE schema_version SET version=4 WHERE version=3;
