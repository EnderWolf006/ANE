-- Existing orders, callbacks and QR snapshots stay on Alipay.
ALTER TABLE merchants ADD COLUMN wechat_qr_image text NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN payment_type text NOT NULL DEFAULT 'alipay' CHECK(payment_type IN ('alipay','wxpay'));
ALTER TABLE events ADD COLUMN payment_type text NOT NULL DEFAULT 'alipay' CHECK(payment_type IN ('alipay','wxpay'));
-- Remove extracted names and free-form receipt copies, retaining matching metadata.
UPDATE events SET raw_text=CASE WHEN amount>0 THEN '支付宝收款 '||(amount/100)::text||'.'||lpad((amount%100)::text,2,'0')||' 元' ELSE '支付宝通知（未识别金额，原文不保留）' END;
-- Rebuild payment notification copies from retained transaction fields rather than
-- retaining names in old subjects, bodies or embedded raw notification text.
UPDATE notification_events SET title=CASE topic WHEN 'payment_received' THEN '收款成功' WHEN 'callback_failed' THEN '回调失败' ELSE '收款未匹配' END,
 message='历史付款通知中的用户名及原文已清除。'
 WHERE topic IN ('payment_received','payment_unmatched','callback_failed');
UPDATE notification_events e SET
 title=CASE WHEN e.topic='callback_failed' THEN '应用 '||o.app_id||'：订单 '||o.out_trade_no||' 回调失败'
 ELSE '支付宝成功收款 ¥'||(o.pay_amount/100)::text||'.'||lpad((o.pay_amount%100)::text,2,'0')
 ||CASE WHEN o.discount>0 THEN '（优惠 ¥'||(o.discount/100)::text||'.'||lpad((o.discount%100)::text,2,'0')||'）' ELSE '' END||' · '||o.app_id END,
 message=format(E'通知类型: %s\n商户: %s\n商户账号: %s\n商户 ID: %s\n发生时间: %s\n应用: %s\n支付渠道: 支付宝\n平台订单号: %s\n商户订单号: %s\n商品: %s\n原金额: %s 元\n实收: %s 元\n优惠: %s 元\n订单状态: %s\n创建时间: %s\n到期时间: %s\n支付时间: %s\n回调地址: %s\n回调状态: %s\n回调尝试: %s\n回调结果: %s\n附加参数: %s\n收款摘要: %s\n',
 CASE WHEN e.topic='callback_failed' THEN '回调失败' ELSE '收款成功' END,m.name,m.username,m.id,e.created_at,o.app_id,o.trade_no,o.out_trade_no,o.name,
 to_char(o.amount::numeric/100,'FM999999990.00'),to_char(o.pay_amount::numeric/100,'FM999999990.00'),to_char(o.discount::numeric/100,'FM999999990.00'),o.status,o.created_at,o.expires_at,o.paid_at,o.notify_url,o.callback_status,o.callback_attempts,o.callback_result,o.param,coalesce((SELECT raw_text FROM events WHERE id=o.event_id),''))
 FROM orders o JOIN merchants m ON m.id=o.merchant_id
 WHERE e.dedupe_key IN ('paid:'||o.trade_no,'callback:'||o.trade_no) AND e.topic IN ('payment_received','callback_failed');
UPDATE notification_events e SET title='支付宝收款 ¥'||to_char(n.amount::numeric/100,'FM999999990.00')||'，未匹配订单',
 message=format(E'通知类型: 收款未匹配\n商户 ID: %s\n支付渠道: 支付宝\n收款金额: %s 元\n通知 ID: %s\n通知时间: %s\n上报时间: %s\n匹配结果: %s\n收款摘要: %s\n',n.merchant_id,to_char(n.amount::numeric/100,'FM999999990.00'),n.notification_id,n.posted_at,n.received_at,n.reason,n.raw_text)
 FROM events n WHERE e.dedupe_key='unmatched:'||n.id AND e.topic='payment_unmatched';
UPDATE mail_queue q SET subject=e.title,body=e.message FROM notification_events e
 WHERE q.event_id=e.id AND e.topic IN ('payment_received','payment_unmatched','callback_failed');
ALTER TABLE orders DROP COLUMN username;
ALTER TABLE events DROP COLUMN payer;
UPDATE schema_version SET version=6 WHERE version=5;
