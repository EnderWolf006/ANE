-- Schemas 2 and 3 predate named applications; v4 deployments already have this table.
CREATE TABLE IF NOT EXISTS merchant_apps (
 merchant_id bigint NOT NULL REFERENCES merchants(id), name text NOT NULL,
 merchant_key text NOT NULL, enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(merchant_id,name),
 UNIQUE(merchant_id,merchant_key)
);
ALTER TABLE merchant_apps ADD COLUMN payment_note text NOT NULL DEFAULT '';
ALTER TABLE merchant_apps ADD COLUMN deleted_at timestamptz;
-- Reserve default for the original merchant credential. Keep a pre-existing app of that name.
DO $$ DECLARE r record; replacement text; n integer; BEGIN
 FOR r IN SELECT a.merchant_id FROM merchant_apps a JOIN merchants m ON m.id=a.merchant_id WHERE a.name='default' AND a.merchant_key<>m.merchant_key LOOP
  n:=1; replacement:='default_legacy';
  WHILE EXISTS(SELECT 1 FROM merchant_apps WHERE merchant_id=r.merchant_id AND name=replacement) LOOP n:=n+1; replacement:='default_legacy_'||n; END LOOP;
  UPDATE merchant_apps SET name=replacement WHERE merchant_id=r.merchant_id AND name='default';
  UPDATE orders SET app_id=replacement WHERE merchant_id=r.merchant_id AND app_id='default';
 END LOOP;
END $$;
INSERT INTO merchant_apps(merchant_id,name,merchant_key,payment_note) SELECT id,'default',merchant_key,payment_note FROM merchants ON CONFLICT(merchant_id,name) DO NOTHING;
UPDATE merchant_apps a SET payment_note=m.payment_note FROM merchants m WHERE a.merchant_id=m.id AND a.name<>'default';
UPDATE orders SET app_id='default' WHERE app_id='';
CREATE TABLE notification_subscriptions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 merchant_id bigint NOT NULL REFERENCES merchants(id), email text NOT NULL,
 topics text[] NOT NULL DEFAULT '{}', apps text[] NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(merchant_id,email)
);
INSERT INTO notification_subscriptions(merchant_id,email,topics,apps)
 SELECT id,email,notification_topics,coalesce((SELECT array_agg(name ORDER BY name) FROM merchant_apps WHERE merchant_id=m.id AND deleted_at IS NULL),'{}') FROM merchants m WHERE email<>'';
ALTER TABLE notification_events ADD COLUMN app_id text;
UPDATE notification_events e SET app_id=o.app_id FROM orders o WHERE e.dedupe_key IN ('paid:'||o.trade_no,'callback:'||o.trade_no);
ALTER TABLE mail_queue ADD COLUMN subscription_id bigint REFERENCES notification_subscriptions(id) ON DELETE SET NULL;
UPDATE mail_queue q SET subscription_id=s.id FROM notification_subscriptions s WHERE q.recipient_id=s.merchant_id AND q.recipient=s.email;
ALTER TABLE mail_queue DROP CONSTRAINT mail_queue_event_id_recipient_id_key;
CREATE UNIQUE INDEX mail_queue_event_recipient ON mail_queue(event_id,recipient_id,recipient);
-- All-merchant subscriptions are retired. Retain sent history and cancel unsent cross-merchant mail.
UPDATE mail_queue q SET status='cancelled',locked_until=NULL,last_error='跨商户订阅已取消' FROM notification_events e WHERE q.event_id=e.id AND q.recipient_id<>e.merchant_id AND q.status IN ('pending','retrying');
UPDATE merchants SET notify_all=false;
UPDATE schema_version SET version=5 WHERE version=4;
