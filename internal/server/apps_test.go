package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/ane/ane/internal/core"
)

func TestAppsSharePoolAndScopeOrders(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "apps", 3)
	h := session(t, a, m.Username, "merchant-password-123")
	for _, name := range []string{"shop_a", "shop_b"} {
		w := request(t, a, "POST", "/api/manage/apps", map[string]any{"name": name, "merchant_key": "secret_" + name, "enabled": true}, h)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	for _, bad := range []map[string]any{{"name": "bad-name", "merchant_key": "secret_bad-name", "enabled": true}, {"name": "valid", "merchant_key": "wrong", "enabled": true}} {
		if w := request(t, a, "POST", "/api/manage/apps", bad, h); w.Code != 400 {
			t.Fatal(w.Body.String())
		}
	}
	v := values("same", "10.00")
	v.Set("pid", fmt.Sprint(m.ID))
	v.Set("username", "alice")
	createSigned := func(key string) Order {
		v.Set("sign", core.Sign(v, key))
		w := request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var result struct {
			TradeNo string `json:"trade_no"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		o, err := scanOrder(a.db.QueryRow(context.Background(), "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", result.TradeNo))
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	first := createSigned("secret_shop_a")
	second := createSigned("secret_shop_b")
	if first.TradeNo == second.TradeNo || first.PayAmount != 1000 || second.PayAmount != 999 {
		t.Fatalf("orders do not share pool: %+v %+v", first, second)
	}
	if again := createSigned("secret_shop_b"); again.TradeNo != second.TradeNo {
		t.Fatal("idempotency", again)
	}
	v.Set("username", "bob")
	v.Set("sign", core.Sign(v, "secret_shop_b"))
	if w := request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	callback := callbackValues(first)
	if callback.Get("app_id") != "" || callback.Get("username") != "" || !core.Verify(callback, "secret_shop_a") {
		t.Fatal(callback)
	}
	query := url.Values{"act": {"order"}, "pid": {fmt.Sprint(m.ID)}, "key": {"secret_shop_b"}, "out_trade_no": {"same"}}
	w := request(t, a, "GET", "/api.php?"+query.Encode(), nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), second.TradeNo) {
		t.Fatal(w.Body.String())
	}
	query.Set("key", "secret_shop_a")
	query.Set("trade_no", second.TradeNo)
	if w = request(t, a, "GET", "/api.php?"+query.Encode(), nil, nil); w.Code != 404 {
		t.Fatal("cross-app query accepted", w.Body.String())
	}
	query.Del("trade_no")
	query.Set("key", "secret_shop_b")
	if w = request(t, a, "PUT", "/api/manage/apps", map[string]any{"name": "shop_b", "merchant_key": "new_shop_b", "enabled": false}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "GET", "/api.php?"+query.Encode(), nil, nil); w.Code != 401 {
		t.Fatal("disabled key accepted", w.Body.String())
	}
	if !core.Verify(callbackValues(second), "secret_shop_b") {
		t.Fatal("old callback key lost")
	}
	if w = request(t, a, "PUT", "/api/manage/apps", map[string]any{"name": "shop_b", "merchant_key": "new_shop_b", "enabled": true}, h); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	query.Set("key", "new_shop_b")
	if w = request(t, a, "GET", "/api.php?"+query.Encode(), nil, nil); w.Code != 200 || !strings.Contains(w.Body.String(), second.TradeNo) {
		t.Fatal("new key query", w.Body.String())
	}
}

func TestMailHistoryFiltersAndPagination(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "mailuser", 3)
	ctx := context.Background()
	_, err := a.db.Exec(ctx, "INSERT INTO notification_subscriptions(merchant_id,email,topics,apps) VALUES($1,'a@example.test',ARRAY['payment_received'],ARRAY['default'])", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 31; i++ {
		tx, err := a.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = queueNotification(ctx, tx, m.ID, "payment_received", fmt.Sprint(i), "应用 default：alice 成功支付 ¥10.00", "订单 123，实收 10.00 元，用户 alice"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	h := session(t, a, m.Username, "merchant-password-123")
	w := request(t, a, "GET", "/api/manage/mail?topic=payment_received&status=pending&q=alice", nil, h)
	var result struct {
		Data []struct {
			Subject  string `json:"subject"`
			Username string `json:"merchant_username"`
		} `json:"data"`
		More bool `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Data) != 30 || !result.More {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
	if result.Data[0].Username != m.Username {
		t.Fatal(result.Data[0])
	}

	_, err = a.db.Exec(ctx, "UPDATE mail_queue SET created_at='2026-11-16T09:30:45Z'")
	if err != nil {
		t.Fatal(err)
	}
	w = request(t, a, "GET", "/api/manage/mail?start=2026-11-16T09:30:45Z&end=2026-11-16T09:30:45Z", nil, h)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Data) != 30 || !result.More {
		t.Fatal("mail datetime bounds", w.Body.String())
	}
	w = request(t, a, "GET", "/api/manage/mail?start=2026-11-16T09:30:46Z", nil, h)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Data) != 0 {
		t.Fatal("mail datetime exclusion", w.Body.String())
	}
	w = request(t, a, "GET", "/api/manage/mail?page=2&topic=payment_received", nil, h)
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Data) != 1 || result.More {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
}
