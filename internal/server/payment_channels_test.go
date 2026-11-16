package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ane/ane/internal/core"
	"strings"
	"testing"
	"time"
)

// Reconstruct schema 5 so upgrade tests exercise the real destructive cleanup.
func downgradeV6Fixture(t *testing.T, a *App) {
	t.Helper()
	downgradeV7Fixture(t, a)
	_, err := a.db.Exec(context.Background(), `ALTER TABLE merchants DROP COLUMN wechat_qr_image;
 ALTER TABLE orders DROP COLUMN payment_type;
 ALTER TABLE events DROP COLUMN payment_type;
 ALTER TABLE orders ADD COLUMN username text NOT NULL DEFAULT '';
 ALTER TABLE events ADD COLUMN payer text NOT NULL DEFAULT '';
 UPDATE schema_version SET version=5;`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWechatChannelsKeepQRCallbackAndNotificationIsolated(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "channels", 5)
	h := session(t, a, m.Username, "merchant-password-123")
	wxQR := qrFixture() + "\n"
	settings := map[string]any{"name": m.Name, "qr_image": m.QR, "wechat_qr_image": wxQR, "pool_limit": 5, "order_ttl_minutes": 10}
	if w := request(t, a, "PUT", "/api/manage/settings", settings, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(t, a, "POST", "/api/manage/subscriptions", map[string]any{"email": "channels@example.test", "topics": []string{"payment_received"}, "apps": []string{"default"}}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	ali := create(t, a, m, "ali", "10.00")
	v := values("wx", "10.00")
	v.Set("type", "wxpay")
	v.Set("username", "discarded_business_name")
	v.Set("pid", fmt.Sprint(m.ID))
	v.Set("sign", core.Sign(v, m.Key))
	w := request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var created struct {
		TradeNo string `json:"trade_no"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	wx, err := scanOrder(a.db.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", created.TradeNo))
	if err != nil {
		t.Fatal(err)
	}
	if wx.PaymentType != "wxpay" || wx.PayAmount != 1000 {
		t.Fatal(wx)
	}
	var savedQR string
	_ = a.db.QueryRow(ctx, "SELECT qr_image FROM orders WHERE trade_no=$1", wx.TradeNo).Scan(&savedQR)
	if savedQR != wxQR {
		t.Fatal("wrong QR snapshot")
	}
	callback := callbackValues(wx)
	if callback.Get("type") != "wxpay" || !core.Verify(callback, m.Key) || epayOrder(wx)["type"] != "wxpay" {
		t.Fatal("wrong callback or query channel")
	}
	dev := device(t, a, m)
	now := time.Now().UnixMilli()
	wrong := EventInput{NotificationID: "wx-wrong", Package: core.WechatPackage, Text: "微信支付收款9.99元(朋友到店)", PostedAt: now}
	w = request(t, a, "POST", "/api/device/events", wrong, dev)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"unmatched"`) {
		t.Fatal(w.Body.String())
	}
	var status string
	_ = a.db.QueryRow(ctx, "SELECT status FROM orders WHERE trade_no=$1", ali.TradeNo).Scan(&status)
	if status != "pending" {
		t.Fatal("WeChat receipt paid Alipay order")
	}
	paid := EventInput{NotificationID: "wx-paid", Package: core.WechatPackage, Text: "[4条]微信收款助手: 微信支付收款10元(老顾客第3次消费)", PostedAt: now}
	w = request(t, a, "POST", "/api/device/events", paid, dev)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"matched"`) {
		t.Fatal(w.Body.String())
	}
	paid.Text = core.ReceiptText("wxpay", 1000)
	w = request(t, a, "POST", "/api/device/events", paid, dev)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatal("normalized retry duplicated", w.Body.String())
	}
	var subject, body string
	if err = a.db.QueryRow(ctx, "SELECT subject,body FROM mail_queue WHERE subject LIKE '微信%' LIMIT 1").Scan(&subject, &body); err != nil {
		t.Fatal(err)
	}
	if subject != "微信支付成功收款 ¥10.00 · default" || strings.Contains(body, "用户名") || strings.Contains(body, "discarded_business_name") {
		t.Fatal(subject, body)
	}
	w = request(t, a, "GET", "/api/manage/orders", nil, h)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"username"`) || strings.Contains(w.Body.String(), `"payer"`) {
		t.Fatal(w.Body.String())
	}
	// New settings fields are optional for older clients, and must not erase WeChat configuration.
	delete(settings, "wechat_qr_image")
	if w = request(t, a, "PUT", "/api/manage/settings", settings, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var preserved string
	_ = a.db.QueryRow(ctx, "SELECT wechat_qr_image FROM merchants WHERE id=$1", m.ID).Scan(&preserved)
	if preserved != wxQR {
		t.Fatal("legacy settings erased WeChat QR")
	}
	v.Set("type", "alipay")
	v.Set("sign", core.Sign(v, m.Key))
	if w = request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil); w.Code != 409 {
		t.Fatal("idempotent order channel changed", w.Body.String())
	}
}

func TestV6MigrationErasesTransactionUsernamesAndPreservesBusiness(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "migration6", 5)
	o := create(t, a, m, "old", "10.00")
	h := session(t, a, m.Username, "merchant-password-123")
	_ = request(t, a, "POST", "/api/manage/subscriptions", map[string]any{"email": "old@example.test", "topics": []string{"payment_received"}, "apps": []string{"default"}}, h)
	dev := device(t, a, m)
	if w := notice(t, a, o, dev, time.Now()); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	downgradeV6Fixture(t, a)
	_, err := a.db.Exec(ctx, `UPDATE orders SET username='private_business_name'; UPDATE events SET payer='private_payer_name',raw_text='private_payer_name通过扫码向你付款10元';
 UPDATE notification_events SET title='private_payer_name 成功支付',message='private_business_name private_payer_name';
 UPDATE mail_queue SET subject='private_payer_name 成功支付',body='private_business_name private_payer_name';`)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := scanOrder(upgraded.db.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", o.TradeNo))
	if err != nil || got.Key != o.Key || got.PayAmount != o.PayAmount || got.PaymentType != "alipay" || got.Status != "paid" {
		t.Fatal(got, err)
	}
	var leaked bool
	err = upgraded.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name='orders' AND column_name='username') OR (table_name='events' AND column_name='payer')))
 OR EXISTS(SELECT 1 FROM events WHERE raw_text LIKE '%private_%')
 OR EXISTS(SELECT 1 FROM notification_events WHERE title LIKE '%private_%' OR message LIKE '%private_%')
 OR EXISTS(SELECT 1 FROM mail_queue WHERE subject LIKE '%private_%' OR body LIKE '%private_%')`).Scan(&leaked)
	if err != nil || leaked {
		t.Fatal("username cleanup incomplete", err)
	}
	if w := request(t, upgraded, "GET", "/api/manage/me", nil, h); w.Code != 200 || !strings.Contains(w.Body.String(), m.Username) {
		t.Fatal("login or session lost", w.Body.String())
	}
	// Old Android may retry the original text after the server has removed it.
	retry := EventInput{NotificationID: "alipay:" + o.TradeNo, Package: core.AlipayPackage, Text: "张**通过扫码向你付款10.00元", PostedAt: got.PaidAt.UnixMilli()}
	if w := request(t, upgraded, "POST", "/api/device/events", retry, dev); w.Code != 200 || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatal("legacy retry", w.Body.String())
	}
	again, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal("restart", err)
	}
	again.Close()
}
