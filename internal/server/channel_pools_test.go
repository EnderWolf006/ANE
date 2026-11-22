package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ane/ane/internal/core"
)

func wechatMerchant(t *testing.T, a *App, name string, limit int) Merchant {
	t.Helper()
	m := testMerchant(t, a, name, limit)
	_, err := a.db.Exec(context.Background(), "UPDATE merchants SET wechat_qr_image=qr_image WHERE id=$1", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func createChannel(t *testing.T, a *App, m Merchant, out, amount, channel string) Order {
	t.Helper()
	v := values(out, amount)
	v.Set("type", channel)
	o, err := a.createOrder(context.Background(), m, v)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestChannelPoolsAllocateReuseShrinkAndMatchIndependently(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := wechatMerchant(t, a, "splitpools", 2)
	ali := createChannel(t, a, m, "ali", "10.00", "alipay")
	wx := createChannel(t, a, m, "wx", "10.00", "wxpay")
	ali2 := createChannel(t, a, m, "ali2", "10.00", "alipay")
	wx2 := createChannel(t, a, m, "wx2", "10.00", "wxpay")
	if ali.PayAmount != 1000 || wx.PayAmount != 1000 || ali2.PayAmount != 999 || wx2.PayAmount != 999 {
		t.Fatal("channels did not start from their own original amount", ali, wx, ali2, wx2)
	}
	for _, channel := range []string{"alipay", "wxpay"} {
		v := values("full_"+channel, "10.00")
		v.Set("type", channel)
		if _, err := a.createOrder(ctx, m, v); !errors.Is(err, errNoAmount) {
			t.Fatal("channel capacity not enforced", channel, err)
		}
	}
	h := session(t, a, m.Username, "merchant-password-123")
	w := request(t, a, "GET", "/api/manage/pools", nil, h)
	var pools struct {
		Data []struct {
			PaymentType string `json:"payment_type"`
			Capacity    int    `json:"capacity"`
			Occupied    int    `json:"occupied"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &pools); err != nil || w.Code != 200 || len(pools.Data) != 2 {
		t.Fatal(w.Body.String(), err)
	}
	seen := map[string]bool{}
	for _, p := range pools.Data {
		seen[p.PaymentType] = true
		if p.Capacity != 2 || p.Occupied != 2 {
			t.Fatal(p)
		}
	}
	if !seen["alipay"] || !seen["wxpay"] {
		t.Fatal(pools)
	}
	dev := device(t, a, m)
	receipt := EventInput{NotificationID: "wx-split", Package: core.WechatPackage, Text: "微信支付收款10元", PostedAt: time.Now().UnixMilli()}
	w = request(t, a, "POST", "/api/device/events", receipt, dev)
	var matched struct {
		TradeNo string `json:"trade_no"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &matched)
	if w.Code != 200 || matched.TradeNo != wx.TradeNo {
		t.Fatal("same amount settled the wrong channel", w.Body.String())
	}
	var state string
	if err := a.db.QueryRow(ctx, "SELECT status FROM orders WHERE trade_no=$1", ali.TradeNo).Scan(&state); err != nil || state != "pending" {
		t.Fatal("Alipay changed after WeChat payment", state, err)
	}
	if fresh := createChannel(t, a, m, "wx_reused", "10.00", "wxpay"); fresh.Discount != 0 {
		t.Fatal("WeChat hole not reused", fresh)
	}
	if _, err := a.db.Exec(ctx, "UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE trade_no=$1", wx2.TradeNo); err != nil {
		t.Fatal(err)
	}
	if err := a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	for channel, want := range map[string]int{"alipay": 2, "wxpay": 1} {
		var got int
		if err := a.db.QueryRow(ctx, "SELECT capacity FROM amount_pools WHERE merchant_id=$1 AND payment_type=$2 AND amount=1000", m.ID, channel).Scan(&got); err != nil || got != want {
			t.Fatal("independent tail shrink", channel, got, err)
		}
	}
	// Cross-amount collisions remain blocked within a channel; the other channel is independent.
	if first := createChannel(t, a, m, "ali_1001", "10.01", "alipay"); first.PayAmount != 1001 {
		t.Fatal(first)
	}
	if _, err := a.createOrder(ctx, m, values("ali_1001_full", "10.01")); !errors.Is(err, errNoAmount) {
		t.Fatal("same-channel cross-amount collision", err)
	}
	if free := createChannel(t, a, m, "wx_999", "9.99", "wxpay"); free.Discount != 0 {
		t.Fatal("other channel blocked free amount", free)
	}
}

func TestChannelPoolsConcurrentAllocationSharedByApplications(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := wechatMerchant(t, a, "parallelchannels", 10)
	_, err := a.db.Exec(ctx, "INSERT INTO merchant_apps(merchant_id,name,merchant_key) VALUES($1,'other','test_other_key')", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	other := m
	other.AppID = "other"
	other.Key = "test_other_key"
	var wg sync.WaitGroup
	results := make(chan Order, 20)
	errs := make(chan error, 20)
	for _, channel := range []string{"alipay", "wxpay"} {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(channel string, i int) {
				defer wg.Done()
				merchant := m
				if i%2 == 1 {
					merchant = other
				}
				v := values(fmt.Sprintf("%s_%d", channel, i), "10.00")
				v.Set("type", channel)
				o, err := a.createOrder(ctx, merchant, v)
				if err != nil {
					errs <- err
					return
				}
				results <- o
			}(channel, i)
		}
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	seen := map[string]map[int64]bool{"alipay": {}, "wxpay": {}}
	for o := range results {
		if seen[o.PaymentType][o.PayAmount] {
			t.Fatal("duplicate in channel", o)
		}
		seen[o.PaymentType][o.PayAmount] = true
	}
	for channel, amounts := range seen {
		if len(amounts) != 10 || !amounts[1000] || !amounts[991] {
			t.Fatal("applications did not share channel pool", channel, amounts)
		}
	}
}
