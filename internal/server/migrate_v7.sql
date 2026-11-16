-- Split allocation by payment channel without changing existing order amounts,
-- keys, expiry windows, callbacks or QR snapshots.
ALTER TABLE amount_pools ADD COLUMN payment_type text NOT NULL DEFAULT 'alipay' CHECK(payment_type IN ('alipay','wxpay'));
ALTER TABLE amount_pools DROP CONSTRAINT amount_pools_pkey;
ALTER TABLE amount_pools ADD PRIMARY KEY(merchant_id,payment_type,amount);
INSERT INTO amount_pools(merchant_id,payment_type,amount,capacity)
 SELECT merchant_id,payment_type,amount,max(discount)+1 FROM orders
 WHERE status='pending' AND expires_at>clock_timestamp()
 GROUP BY merchant_id,payment_type,amount
 ON CONFLICT(merchant_id,payment_type,amount) DO UPDATE SET capacity=excluded.capacity;
UPDATE amount_pools p SET capacity=coalesce((SELECT max(o.discount)+1 FROM orders o
 WHERE o.merchant_id=p.merchant_id AND o.payment_type=p.payment_type AND o.amount=p.amount
 AND o.status='pending' AND o.expires_at>clock_timestamp()),0);
DROP INDEX active_pay_amount;
CREATE UNIQUE INDEX active_pay_amount ON orders(merchant_id,payment_type,pay_amount) WHERE status='pending';
UPDATE schema_version SET version=7 WHERE version=6;
