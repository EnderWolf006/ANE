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
	for _, tc := range []struct {
		pkg, text string
		cents     int64
	}{
		{AlipayPackage, "张**通过扫码向你付款0.01元", 1},
		{AlipayPackage, "你已成功收款0.01元.\n已转入余额 做生意收钱提现免费>>", 1},
		{AlipayPackage, "支付宝到账 12.3 元！", 1230},
		{AlipayPackage, "收款成功：￥999999.99元（商家收款）", 99999999},
		{WechatPackage, "[4条]微信收款助手: 微信支付收款0.01元(朋友到店)", 1},
		{WechatPackage, "微信支付收款0.01元(老顾客第3次消费)", 1},
		{WechatPackage, "微信收款：12.30元", 1230},
	} {
		got, err := ParseNotice(tc.pkg, tc.text)
		if err != nil || got != tc.cents {
			t.Errorf("%s: %d %v", tc.text, got, err)
		}
	}
	for _, tc := range []struct{ pkg, text string }{
		{AlipayPackage, "张三通过扫码向你付款0.00元"}, {AlipayPackage, "成功收款1.001元"},
		{AlipayPackage, "成功收款01.01元"}, {AlipayPackage, "成功收款1000000元"},
		{AlipayPackage, "退款到账1元"}, {AlipayPackage, "收款失败1元"},
		{AlipayPackage, "待付款1元"}, {AlipayPackage, "成功收款1元，余额2元"},
		{WechatPackage, "微信支付收款1元；微信支付收款1元"},
		{WechatPackage, "张三通过扫码向你付款1元"}, {AlipayPackage, "微信支付收款1元"},
		{"com.other.app", "微信支付收款1元"},
	} {
		if _, err := ParseNotice(tc.pkg, tc.text); err == nil {
			t.Errorf("accepted %s: %s", tc.pkg, tc.text)
		}
	}
}
