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

func TestGeneratedAppKeysAndRotation(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "generated", 3)
	h := session(t, a, m.Username, "merchant-password-123")
	w := request(t, a, "POST", "/api/manage/apps", map[string]any{"name": "my_shop_2"}, h)
	var result struct {
		Key string `json:"merchant_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || !strings.HasSuffix(result.Key, "_my_shop_2") || len(result.Key) != 74 {
		t.Fatalf("create: %s %v", w.Body.String(), err)
	}
	oldKey := result.Key
	m.AppID, m.Key = "my_shop_2", oldKey
	order := create(t, a, m, "before_rotation", "10.00")
	w = request(t, a, "PATCH", "/api/manage/apps", map[string]any{"name": m.AppID, "enabled": false}, h)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var key string
	var enabled bool
	if err := a.db.QueryRow(context.Background(), "SELECT merchant_key,enabled FROM merchant_apps WHERE merchant_id=$1 AND name='my_shop_2'", m.ID).Scan(&key, &enabled); err != nil || enabled || key != oldKey {
		t.Fatalf("disable changed key: %v", err)
	}
	w = request(t, a, "PATCH", "/api/manage/apps", map[string]any{"name": m.AppID, "rotate": true}, h)
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Key == oldKey || !strings.HasSuffix(result.Key, "_my_shop_2") {
		t.Fatal("rotation", w.Body.String())
	}
	if err := a.db.QueryRow(context.Background(), "SELECT enabled FROM merchant_apps WHERE merchant_id=$1 AND name='my_shop_2'", m.ID).Scan(&enabled); err != nil || enabled {
		t.Fatal("rotation re-enabled app", err)
	}
	if !core.Verify(callbackValues(order), oldKey) {
		t.Fatal("old order callback changed")
	}
	w = request(t, a, "PATCH", "/api/manage/apps", map[string]any{"name": m.AppID, "enabled": true}, h)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m.Key = result.Key
	if _, err := a.createOrder(context.Background(), m, values("new_key", "20.00")); err != nil {
		t.Fatal(err)
	}
	m.Key = oldKey
	if _, err := a.createOrder(context.Background(), m, values("old_key", "20.00")); err == nil {
		t.Fatal("old key accepted")
	}
}

func TestOrderFilters(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "filters", 5)
	h := session(t, a, m.Username, "merchant-password-123")
	zero := create(t, a, m, "zero", "10.00")
	discounted := create(t, a, m, "discounted", "10.00")
	expired := create(t, a, m, "expired", "20.00")
	_, err := a.db.Exec(ctx, "UPDATE orders SET expires_at=now()-interval '1 minute' WHERE trade_no=$1", expired.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, "UPDATE orders SET status='paid',callback_status='failed' WHERE trade_no=$1", discounted.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, "INSERT INTO merchant_apps(merchant_id,name,merchant_key) VALUES($1,'filter_app','secret_filter_app')", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	m.AppID, m.Key = "filter_app", "secret_filter_app"
	app := create(t, a, m, "app_order", "30.00")
	_, err = a.db.Exec(ctx, "UPDATE orders SET created_at=now()-interval '100 days' WHERE trade_no=$1", app.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	check := func(query string, expected ...string) {
		t.Helper()
		w := request(t, a, "GET", "/api/manage/orders?"+query, nil, h)
		var result struct {
			Data []Order `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
			t.Fatalf("%s: %s %v", query, w.Body.String(), err)
		}
		if len(result.Data) != len(expected) {
			t.Fatalf("%s: got %d want %d: %s", query, len(result.Data), len(expected), w.Body.String())
		}
		for i, o := range result.Data {
			if o.TradeNo != expected[i] {
				t.Fatalf("%s: wrong order at %d", query, i)
			}
		}
	}
	check("status=pending", zero.TradeNo, app.TradeNo)
	check("status=expired", expired.TradeNo)
	check("status=paid&callback_status=failed&discount=discounted&q=discounted", discounted.TradeNo)
	check("app_id=filter_app", app.TradeNo)
	check("app_id=default&status=pending", zero.TradeNo)
	check("discount=none&sort=amount_asc", zero.TradeNo, expired.TradeNo, app.TradeNo)
	check("sort=amount_desc", app.TradeNo, expired.TradeNo, discounted.TradeNo, zero.TradeNo)
	check("period=7d&status=pending", zero.TradeNo)
	check("q="+url.QueryEscape("测试商品 & café")+"&status=paid", discounted.TradeNo)
	for _, bad := range []string{"status=invalid", "sort=amount%3BDROP", "period=forever", "callback_status=bad", "discount=bad"} {
		if w := request(t, a, "GET", "/api/manage/orders?"+bad, nil, h); w.Code != 400 {
			t.Fatal("invalid filter accepted", bad, w.Body.String())
		}
	}
	other := testMerchant(t, a, "other_filter", 3)
	otherH := session(t, a, other.Username, "merchant-password-123")
	w := request(t, a, "GET", "/api/manage/orders/filters", nil, otherH)
	if w.Code != 200 || strings.Contains(w.Body.String(), "filter_app") {
		t.Fatal("filter choices leaked", w.Body.String())
	}
}

func TestOrderSummaryAcrossPagesAndScopes(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "summary", 5)
	other := testMerchant(t, a, "summary_other", 5)
	create(t, a, other, "outside_scope", "999.00")
	h := session(t, a, m.Username, "merchant-password-123")
	for i := 0; i < 35; i++ {
		o := create(t, a, m, fmt.Sprintf("summary_%02d", i), "10.00")
		_, err := a.db.Exec(ctx, "UPDATE orders SET status='paid',pay_amount=999,discount=1,callback_status='success' WHERE trade_no=$1", o.TradeNo)
		if err != nil {
			t.Fatal(err)
		}
	}
	expired := create(t, a, m, "expired_summary", "20.00")
	_, err := a.db.Exec(ctx, "UPDATE orders SET expires_at=now()-interval '1 minute' WHERE trade_no=$1", expired.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	type totals struct {
		Orders   int
		Paid     int
		Revenue  int64 `json:"revenue_cents"`
		Discount int64 `json:"discount_cents"`
	}
	check := func(query string, headers map[string]string, want totals, rows int) {
		t.Helper()
		w := request(t, a, "GET", "/api/manage/orders?"+query, nil, headers)
		var result struct {
			Data    []Order
			Summary totals
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
			t.Fatal(w.Body.String(), err)
		}
		if result.Summary != want || len(result.Data) != rows {
			t.Fatalf("%s: totals=%+v rows=%d want=%+v rows=%d", query, result.Summary, len(result.Data), want, rows)
		}
	}
	check("page=1", h, totals{36, 35, 34965, 35}, 30)
	check("page=2", h, totals{36, 35, 34965, 35}, 6)
	check("page=9", h, totals{36, 35, 34965, 35}, 0)
	check("status=paid&callback_status=success&app_id=default&discount=discounted&q=summary_&period=7d&start=2020-01-01T00%3A00%3A00Z&end=2030-01-01T00%3A00%3A00Z", h, totals{35, 35, 34965, 35}, 30)
	check("status=expired", h, totals{1, 0, 0, 0}, 1)
	check("q=no_matches", h, totals{}, 0)
	check("", session(t, a, other.Username, "merchant-password-123"), totals{1, 0, 0, 0}, 1)
	admin := session(t, a, "admin", "test-admin-password")
	check("merchant_id="+fmt.Sprint(m.ID), admin, totals{36, 35, 34965, 35}, 30)
	check("merchant_id=all", admin, totals{37, 35, 34965, 35}, 30)
}
