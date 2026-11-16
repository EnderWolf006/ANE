package core

import (
	"net/url"
	"testing"
)

func TestSignEPay(t *testing.T) {
	v := url.Values{"pid": {"1000"}, "type": {"alipay"}, "out_trade_no": {"order-1"}, "name": {"测试商品"}, "money": {"0.01"}, "notify_url": {"https://merchant.example/notify"}, "empty": {""}, "sign_type": {"MD5"}}
	// Independent, literal canonical string, including decoded Chinese text.
	want := "73e2ed2075e880a1cad45aa885c3b373"
	got := Sign(v, "secret")
	if got != want {
		t.Fatalf("signature: got %s want %s", got, want)
	}
	v.Set("sign", got)
	if !Verify(v, "secret") {
		t.Fatal("valid signature rejected")
	}
	v.Set("money", "0.02")
	if Verify(v, "secret") {
		t.Fatal("tampered amount accepted")
	}
	v.Set("money", "0.01")
	v["pid"] = []string{"1000", "1000"}
	if Verify(v, "secret") {
		t.Fatal("duplicate parameter accepted")
	}
}
func TestMoney(t *testing.T) {
	for s, want := range map[string]int64{"0.01": 1, "1": 100, "1.2": 120, "999999.99": 99999999} {
		got, err := ParseMoney(s)
		if err != nil || got != want {
			t.Fatalf("%s: %d %v", s, got, err)
		}
		if _, err = ParseMoney(Money(got)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"0", "0.00", "-1", "1.001", "1e2", "01.01", "NaN", " 1.00", "1000000"} {
		if _, err := ParseMoney(s); err == nil {
			t.Errorf("accepted invalid %q", s)
		}
	}
}
func TestNotice(t *testing.T) {
	payer, cents, err := ParseNotice("张**通过扫码向你付款0.01元")
	if err != nil || payer != "张**" || cents != 1 {
		t.Fatalf("%s %d %v", payer, cents, err)
	}
	for _, s := range []string{"支付宝到账0.01元", "张三通过扫码向你付款0.00元", "张三通过扫码向你付款1.001元", "通过扫码向你付款1元", "张三通过扫码向你付款01.01元", "张三通过扫码向你付款1元 其他文本"} {
		if _, _, err := ParseNotice(s); err == nil {
			t.Errorf("accepted invalid %q", s)
		}
	}
}
