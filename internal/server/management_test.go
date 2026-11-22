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
)

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
	if subject != "支付宝成功收款 ¥10.00 · default" || strings.Contains(subject, "优惠") || !strings.Contains(body, "平台订单号: "+o.TradeNo) || strings.Contains(body, "张**") {
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
		_, err := a.db.Exec(ctx, `UPDATE orders SET created_at='2026-11-16T09:30:45Z',name='=formula' WHERE trade_no=$1`, o.TradeNo)
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
