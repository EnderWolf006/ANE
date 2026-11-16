package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPaymentNoteSanitization(t *testing.T) {
	input := `<p onclick="alert(1)"><strong>付款提示</strong><br>请勿重复付款</p><a href="https://example.com/help">帮助</a><script>alert(1)</script><img src=x onerror=alert(1)><a href="javascript:alert(1)">bad</a><svg onload=alert(1)></svg><iframe src="https://evil.test"></iframe>`
	clean := sanitizePaymentNote(input)
	for _, bad := range []string{"onclick", "script", "onerror", "<img", "<svg", "<iframe"} {
		if strings.Contains(clean, bad) {
			t.Fatalf("unsafe content retained: %s", clean)
		}
	}
	for _, safe := range []string{"<strong>付款提示</strong>", "<br>", "https://example.com/help"} {
		if !strings.Contains(clean, safe) {
			t.Fatalf("formatting lost: %s", clean)
		}
	}
}
func TestZeroDiscountDeviceRenameAndPaymentNote(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "details", 1)
	h := session(t, a, m.Username, "merchant-password-123")
	ctx := context.Background()
	o := create(t, a, m, "one-cent", "0.01")
	if o.PayAmount != 1 || o.Discount != 0 {
		t.Fatal("original amount not preferred", o)
	}
	if _, err := a.createOrder(ctx, m, values("full", "0.01")); err == nil {
		t.Fatal("capacity not enforced")
	}
	w := request(t, a, "GET", "/api/manage/pools", nil, h)
	if !strings.Contains(w.Body.String(), `"capacity":1`) {
		t.Fatal(w.Body.String())
	}
	_, _ = a.db.Exec(ctx, "UPDATE orders SET expires_at=now()-interval '1 second' WHERE trade_no=$1", o.TradeNo)
	w = request(t, a, "GET", "/api/manage/pools", nil, h)
	var pools struct {
		Data []any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pools)
	if len(pools.Data) != 0 {
		t.Fatal("empty pool still visible", w.Body.String())
	}
	dev := device(t, a, m)
	if w = request(t, a, "POST", "/api/device/heartbeat", Telemetry{Battery: 70, Network: "wifi", WiFiSSID: "ANE 测试网络"}, dev); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "PATCH", "/api/manage/device", map[string]string{"name": "新设备名称"}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "POST", "/api/device/heartbeat", Telemetry{Battery: 70, Network: "wifi", WiFiSSID: "ANE 测试网络"}, dev); w.Code != 200 {
		t.Fatal("rename invalidated token", w.Body.String())
	}
	w = request(t, a, "GET", "/api/manage/device", nil, h)
	if !strings.Contains(w.Body.String(), "新设备名称") || !strings.Contains(w.Body.String(), "ANE 测试网络") {
		t.Fatal(w.Body.String())
	}
	w = request(t, a, "PATCH", "/api/manage/apps", map[string]any{"name": "default", "payment_note": `<strong style="color: #e53935; font-size: 16px; position: fixed">联系商户</strong><script>alert(1)</script>`}, h)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m, _ = a.merchant(ctx, m.ID)
	o = create(t, a, m, "note", "1.00")
	_, _ = a.db.Exec(ctx, "UPDATE merchant_apps SET payment_note='changed' WHERE merchant_id=$1", m.ID)
	w = request(t, a, "GET", "/api/pay/"+o.TradeNo, nil, nil)
	var pay struct {
		Note string `json:"payment_note"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pay)
	if pay.Note != `<strong style="color: #e53935; font-size: 16px">联系商户</strong>` {
		t.Fatal("unsafe or non-snapshot payment note", pay.Note)
	}
}
func TestSMTPPromptEndpoint(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	admin := session(t, a, "admin", a.cfg.AdminPassword)
	m := testMerchant(t, a, "smtp_user", 2)
	merchant := session(t, a, m.Username, "merchant-password-123")
	if w := request(t, a, "POST", "/api/manage/smtp/test", map[string]string{"email": "test@example.test"}, merchant); w.Code != 403 {
		t.Fatal("merchant can test global SMTP")
	}
	_, _ = a.db.Exec(ctx, "UPDATE smtp_settings SET host='smtp.example.test',from_address='ane@example.test',password='saved-secret'")
	calls := 0
	a.sendMail = func(_ context.Context, c SMTPConfig, to, subject, body string) error {
		calls++
		if to != "test@example.test" || c.Password != "saved-secret" {
			t.Fatal("wrong saved config or recipient")
		}
		return nil
	}
	if w := request(t, a, "POST", "/api/manage/smtp/test", map[string]string{"email": "bad\r\nBcc:x"}, admin); w.Code != 400 {
		t.Fatal("invalid email accepted")
	}
	if w := request(t, a, "POST", "/api/manage/smtp/test", map[string]string{"email": "test@example.test"}, admin); w.Code != 200 || calls != 1 {
		t.Fatal(w.Body.String(), calls)
	}
	a.sendMail = func(context.Context, SMTPConfig, string, string, string) error {
		return fmt.Errorf("connection refused")
	}
	if w := request(t, a, "POST", "/api/manage/smtp/test", map[string]string{"email": "test@example.test"}, admin); w.Code != 502 || !strings.Contains(w.Body.String(), "connection refused") {
		t.Fatal("missing delivery error", w.Body.String())
	}
}
func TestUpgradeV2PreservesOrders(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "upgrade", 3)
	_ = create(t, a, m, "zero", "10.00")
	old := create(t, a, m, "old", "10.00")
	downgradeV5Fixture(t, a)
	// Reconstruct v2 shape in this isolated schema while retaining a pre-upgrade paid/discounted order.
	_, err := a.db.Exec(ctx, `DROP TABLE merchant_apps;
 DELETE FROM orders WHERE discount=0;
 ALTER TABLE merchants DROP COLUMN payment_note;
 ALTER TABLE devices DROP COLUMN wifi_ssid;
 ALTER TABLE orders DROP COLUMN payment_note;
 ALTER TABLE orders DROP CONSTRAINT orders_amount_check,DROP CONSTRAINT orders_check,DROP CONSTRAINT orders_discount_check;
 ALTER TABLE orders ADD CONSTRAINT orders_amount_check CHECK(amount>1),ADD CONSTRAINT orders_check CHECK(pay_amount>0 AND pay_amount<amount),ADD CONSTRAINT orders_discount_check CHECK(discount>0);
 UPDATE schema_version SET version=2;`)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	_ = upgraded.db.QueryRow(ctx, "SELECT max(version) FROM schema_version").Scan(&version)
	if version != 6 {
		t.Fatal(version)
	}
	var pay int64
	_ = upgraded.db.QueryRow(ctx, "SELECT pay_amount FROM orders WHERE trade_no=$1", old.TradeNo).Scan(&pay)
	if pay != 999 {
		t.Fatal("old order changed", pay)
	}
	m, _ = upgraded.merchant(ctx, m.ID)
	if next := create(t, upgraded, m, "after", "10.00"); next.Discount != 0 {
		t.Fatal("zero slot not added")
	}
}

func TestPaymentNoteColorsAndFontSizes(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		keep, drop  []string
	}{
		{"hex and px", `<span style="color: #e53935; font-size: 16px; position: fixed; background-image: url(https://evil.test)">提示</span>`, []string{"<span", "color: #e53935", "font-size: 16px", "提示"}, []string{"position", "background", "evil.test"}},
		{"named and relative", `<p style="color: red; font-size: 1.25rem"><strong style="font-size: 120%">重点</strong></p>`, []string{"color: red", "font-size: 1.25rem", "font-size: 120%", "<strong"}, nil},
		{"rgb and pt", `<a href="https://example.com" style="color: rgb(10, 20, 30); font-size: 12pt">帮助</a>`, []string{"rgb(10, 20, 30)", "font-size: 12pt", "https://example.com"}, nil},
		{"malicious CSS", `<span onclick="alert(1)" style="color: expression(alert(1)); font-size: url(https://evil.test); display: none; --x: red">提示</span>`, []string{"提示"}, []string{"style=", "onclick", "expression", "url(", "display", "--x"}},
		{"invalid size", `<p style="color: #123; font-size: -12px">提示</p>`, []string{"color: #123"}, []string{"font-size"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clean := sanitizePaymentNote(tc.input)
			for _, v := range tc.keep {
				if !strings.Contains(clean, v) {
					t.Errorf("lost %q: %s", v, clean)
				}
			}
			for _, v := range tc.drop {
				if strings.Contains(clean, v) {
					t.Errorf("retained %q: %s", v, clean)
				}
			}
			if again := sanitizePaymentNote(clean); again != clean {
				t.Errorf("not stable on order read: %s => %s", clean, again)
			}
		})
	}
}
