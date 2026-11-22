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
