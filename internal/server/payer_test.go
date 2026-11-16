package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAlipayPayerAppearsInOrdersSearchCSVAndSubject(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "alipay_payer", 5)
	h := session(t, a, m.Username, "merchant-password-123")
	w := request(t, a, "POST", "/api/manage/subscriptions", map[string]any{"email": "payer@example.test", "topics": []string{"payment_received"}, "apps": []string{"default"}}, h)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_ = create(t, a, m, "occupy", "10.00")
	v := values("paid", "10.00")
	v.Set("username", "business_alice")
	order, err := a.createOrder(ctx, m, v)
	if err != nil {
		t.Fatal(err)
	}
	if w = notice(t, a, order, device(t, a, m), time.Now()); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	query := "q=" + url.QueryEscape("张**")
	w = request(t, a, "GET", "/api/manage/orders?"+query, nil, h)
	var result struct {
		Data []Order `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Data) != 1 {
		t.Fatal(w.Body.String(), err)
	}
	got := result.Data[0]
	if got.Payer != "张**" || got.Username != "business_alice" || got.TradeNo != order.TradeNo {
		t.Fatal(got)
	}
	var subject, body string
	if err = a.db.QueryRow(ctx, "SELECT subject,body FROM mail_queue").Scan(&subject, &body); err != nil {
		t.Fatal(err)
	}
	if subject != "张** 成功支付 ¥9.99（优惠 ¥0.01） · default" || !strings.Contains(body, "业务用户名: business_alice") {
		t.Fatal(subject, body)
	}
	w = request(t, a, "GET", "/api/manage/orders?export=csv&"+query, nil, h)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(records) != 2 || records[1][3] != "张**" || records[1][4] != "business_alice" {
		t.Fatal(w.Body.String(), err)
	}
}
