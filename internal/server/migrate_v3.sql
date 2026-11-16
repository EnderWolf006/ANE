ALTER TABLE merchants ADD COLUMN payment_note text NOT NULL DEFAULT '请按显示的精确金额支付，勿修改金额或重复付款。<br>付款成功后本页会自动更新。';
ALTER TABLE devices ADD COLUMN wifi_ssid text NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN payment_note text NOT NULL DEFAULT '请按显示的精确金额支付，勿修改金额或重复付款。<br>付款成功后本页会自动更新。';
ALTER TABLE orders DROP CONSTRAINT orders_amount_check, DROP CONSTRAINT orders_check, DROP CONSTRAINT orders_discount_check;
ALTER TABLE orders ADD CONSTRAINT orders_amount_check CHECK(amount>0), ADD CONSTRAINT orders_pay_amount_check CHECK(pay_amount>0 AND pay_amount<=amount), ADD CONSTRAINT orders_discount_check CHECK(discount>=0);
UPDATE amount_pools p SET capacity=coalesce((SELECT max(discount)+1 FROM orders o WHERE o.merchant_id=p.merchant_id AND o.amount=p.amount AND o.status='pending' AND o.expires_at>now()),0);
UPDATE schema_version SET version=3 WHERE version=2;
