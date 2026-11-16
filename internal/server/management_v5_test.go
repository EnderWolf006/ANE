package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ane/ane/internal/core"
)

// Recreate the actual pre-upgrade shape, including the implicit merchant credential.
func downgradeV5Fixture(t *testing.T, a *App) {
	t.Helper()
	_, err := a.db.Exec(context.Background(), `ALTER TABLE mail_queue DROP COLUMN subscription_id;
 DROP TABLE notification_subscriptions;
 DROP INDEX IF EXISTS mail_queue_event_recipient;
 ALTER TABLE mail_queue DROP CONSTRAINT IF EXISTS mail_queue_event_id_recipient_id_recipient_key;
 ALTER TABLE mail_queue ADD CONSTRAINT mail_queue_event_id_recipient_id_key UNIQUE(event_id,recipient_id);
 ALTER TABLE notification_events DROP COLUMN app_id;
 UPDATE orders SET app_id='' WHERE app_id='default';
 DELETE FROM merchant_apps WHERE name='default';
 ALTER TABLE merchant_apps DROP COLUMN payment_note,DROP COLUMN deleted_at;
 UPDATE schema_version SET version=4;`)
	if err != nil {
		t.Fatal(err)
	}
}
func TestV5MigrationPreservesCredentialsOrdersAndSubscriptions(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "migration5", 5)
	old := create(t, a, m, "old", "10.00")
	downgradeV5Fixture(t, a)
	_, err := a.db.Exec(ctx, `UPDATE merchants SET email='old@example.test',notification_topics=ARRAY['payment_received'],payment_note='old note' WHERE id=$1`, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, `INSERT INTO merchant_apps(merchant_id,name,merchant_key) VALUES($1,'default','different_default'),($1,'default_legacy','another_default_legacy');
 `, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, `INSERT INTO notification_events(merchant_id,topic,dedupe_key,title,message) VALUES($1,'payment_received',$2,'old title','old body');`, m.ID, "paid:"+old.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, `INSERT INTO mail_queue(event_id,recipient_id,recipient,subject,body) SELECT id,$1,'old@example.test','old subject','old body' FROM notification_events`, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var key, note, app string
	if err = upgraded.db.QueryRow(ctx, `SELECT merchant_key,payment_note FROM merchant_apps WHERE merchant_id=$1 AND name='default'`, m.ID).Scan(&key, &note); err != nil || key != m.Key || note != "old note" {
		t.Fatalf("default credential changed: %v", err)
	}
	got, err := scanOrder(upgraded.db.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", old.TradeNo))
	if err != nil || got.AppID != "default" || got.Key != old.Key || got.PayAmount != old.PayAmount {
		t.Fatalf("order changed: %+v %v", got, err)
	}
	if err = upgraded.db.QueryRow(ctx, `SELECT name FROM merchant_apps WHERE merchant_id=$1 AND merchant_key='different_default'`, m.ID).Scan(&app); err != nil || app != "default_legacy_2" {
		t.Fatal("existing named default lost", app, err)
	}
	var ok bool
	if err = upgraded.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mail_queue q JOIN notification_subscriptions s ON s.id=q.subscription_id JOIN notification_events e ON e.id=q.event_id WHERE s.email='old@example.test' AND 'default'=ANY(s.apps) AND s.topics=ARRAY['payment_received'] AND e.app_id='default' AND q.subject='old subject')`).Scan(&ok); err != nil || !ok {
		t.Fatal("mail migration", err)
	}
	v := values("old", "10.00")
	v.Set("pid", fmt.Sprint(m.ID))
	v.Set("sign", core.Sign(v, m.Key))
	w := request(t, upgraded, "GET", "/mapi.php?"+v.Encode(), nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), old.TradeNo) {
		t.Fatal("legacy idempotency", w.Body.String())
	}
	h := session(t, upgraded, m.Username, "merchant-password-123")
	if w = request(t, upgraded, "DELETE", "/api/manage/apps", map[string]any{"name": "default"}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, upgraded, "GET", "/mapi.php?"+v.Encode(), nil, nil); w.Code == 200 {
		t.Fatal("deleted default still authenticates")
	}
	restarted, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if w = request(t, restarted, "GET", "/mapi.php?"+v.Encode(), nil, nil); w.Code == 200 {
		t.Fatal("default resurrected on restart")
	}
	if !core.Verify(callbackValues(got), old.Key) {
		t.Fatal("historical callback key changed")
	}
}
func TestIndependentEmailApplicationPreferences(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "email_apps", 10)
	h := session(t, a, m.Username, "merchant-password-123")
	if w := request(t, a, "POST", "/api/manage/apps", map[string]any{"name": "shop"}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	ids := []int64{}
	for _, s := range []map[string]any{{"email": "one@example.test", "topics": []string{"payment_received"}, "apps": []string{"default"}}, {"email": "two@example.test", "topics": []string{"payment_received", "device_online"}, "apps": []string{"shop"}}, {"email": "three@example.test", "topics": []string{"payment_received"}, "apps": []string{"default", "shop"}}} {
		w := request(t, a, "POST", "/api/manage/subscriptions", s, h)
		var r struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		ids = append(ids, r.ID)
	}
	o := create(t, a, m, "paid", "10.00")
	dev := device(t, a, m)
	if w := notice(t, a, o, dev, time.Now()); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var count int
	var subject, body string
	if err := a.db.QueryRow(ctx, `SELECT count(*),min(subject),min(body) FROM mail_queue`).Scan(&count, &subject, &body); err != nil || count != 2 {
		t.Fatal("app routing", count, err)
	}
	if subject != "张** 成功支付 ¥10.00 · default" || strings.Contains(subject, "优惠") || !strings.Contains(body, "平台订单号: "+o.TradeNo) || !strings.Contains(body, "支付宝用户名: 张**") {
		t.Fatal(subject, body)
	}
	if w := request(t, a, "DELETE", "/api/manage/subscriptions", map[string]any{"id": ids[0]}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_, _ = a.db.Exec(ctx, `UPDATE smtp_settings SET enabled=true`)
	sent := 0
	a.sendMail = func(context.Context, SMTPConfig, string, string, string) error { sent++; return nil }
	for a.mailOnce(ctx) {
	}
	if sent != 1 {
		t.Fatal("deleted subscription still sends", sent)
	}
	if w := request(t, a, "DELETE", "/api/manage/apps", map[string]any{"name": "shop"}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var exists bool
	_ = a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_subscriptions WHERE merchant_id=$1 AND 'shop'=ANY(apps))`, m.ID).Scan(&exists)
	if exists {
		t.Fatal("deleted app still subscribed")
	}
}
func TestCSVExportsAllFilteredRowsAndDateTimes(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "csv_dates", 99)
	h := session(t, a, m.Username, "merchant-password-123")
	for i := 0; i < 35; i++ {
		o := create(t, a, m, fmt.Sprint(i), "10.00")
		_, err := a.db.Exec(ctx, `UPDATE orders SET created_at='2026-11-16T09:30:45Z',username='=formula' WHERE trade_no=$1`, o.TradeNo)
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = create(t, a, m, "outside", "20.00")
	params := url.Values{"start": {"2026-11-16T01:30:45-08:00"}, "end": {"2026-11-16T09:30:45Z"}, "app_id": {"default"}, "export": {"csv"}, "page": {"2"}}
	w := request(t, a, "GET", "/api/manage/orders?"+params.Encode(), nil, h)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || w.Code != 200 || len(records) != 36 {
		t.Fatalf("export: %d %v %s", len(records), err, w.Body.String())
	}
	if records[1][4] != "'=formula" {
		t.Fatal("unsafe CSV", records[1])
	}
	params.Del("export")
	w = request(t, a, "GET", "/api/manage/orders?"+params.Encode(), nil, h)
	var list struct {
		Data []Order `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if w.Code != 200 || len(list.Data) != 5 {
		t.Fatal("datetime pagination", w.Body.String())
	}
	for _, path := range []string{"orders", "mail"} {
		for _, bad := range []string{"start=invalid", "start=2026-11-17T00:00:00Z&end=2026-11-16T00:00:00Z"} {
			if w = request(t, a, "GET", "/api/manage/"+path+"?"+bad, nil, h); w.Code != 400 {
				t.Fatal("invalid range accepted", path, bad, w.Body.String())
			}
		}
	}
}
