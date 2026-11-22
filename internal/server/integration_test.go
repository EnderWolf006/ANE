package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ane/ane/internal/core"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ane_test_" + token()[:12]
	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); conn.Close(ctx) })
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	a, err := New(ctx, Config{DatabaseURL: u.String(), BaseURL: "http://ane.test", AdminUser: "admin", AdminPassword: "test-admin-password", AllowPrivateCallbacks: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}
func qrFixture() string {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
func testMerchant(t *testing.T, a *App, name string, limit int) Merchant {
	t.Helper()
	h, _ := bcrypt.GenerateFromPassword([]byte("merchant-password-123"), bcrypt.MinCost)
	m, err := scanMerchant(a.db.QueryRow(context.Background(), "INSERT INTO merchants(username,name,password_hash,merchant_key,pool_limit,qr_image) VALUES($1,$1,$2,$3,$4,$5) RETURNING "+merchantColumns, name, string(h), token(), limit, qrFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.db.Exec(context.Background(), "INSERT INTO merchant_apps(merchant_id,name,merchant_key,payment_note) VALUES($1,'default',$2,$3)", m.ID, m.Key, m.PaymentNote); err != nil {
		t.Fatal(err)
	}
	return m
}
func values(out, money string) url.Values {
	return url.Values{"type": {"alipay"}, "out_trade_no": {out}, "name": {"测试商品 & café"}, "money": {money}, "notify_url": {"http://127.0.0.1/notify"}, "return_url": {"https://merchant.example/return"}, "param": {"user=1"}}
}
func create(t *testing.T, a *App, m Merchant, out, amount string) Order {
	t.Helper()
	o, err := a.createOrder(context.Background(), m, values(out, amount))
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func request(t *testing.T, a *App, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &b)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", a.cfg.BaseURL)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func session(t *testing.T, a *App, user, password string) map[string]string {
	t.Helper()
	w := request(t, a, "POST", "/api/auth/login", map[string]string{"username": user, "password": password}, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	return map[string]string{"Cookie": w.Result().Cookies()[0].String()}
}
func device(t *testing.T, a *App, m Merchant) map[string]string {
	t.Helper()
	tok := token()
	_, err := a.db.Exec(context.Background(), "INSERT INTO devices(merchant_id,name,token_hash) VALUES($1,'测试设备',$2)", m.ID, digest(tok))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"Authorization": "Bearer " + tok, "X-ANE-Device-ID": token()}
}
func notice(t *testing.T, a *App, o Order, h map[string]string, posted time.Time) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, a, "POST", "/api/device/events", EventInput{NotificationID: "alipay:" + o.TradeNo, Package: "com.eg.android.AlipayGphone", Text: "张**通过扫码向你付款" + core.Money(o.PayAmount) + "元", PostedAt: posted.UnixMilli()}, h)
}
func TestPoolAllocationReuseShrinkAndIsolation(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "merchant_a", 3)
	other := testMerchant(t, a, "merchant_b", 3)
	h := device(t, a, m)
	ctx := context.Background()
	first := create(t, a, m, "first", "10.00")
	second := create(t, a, m, "second", "10.00")
	third := create(t, a, m, "third", "10.00")
	if first.Discount != 0 || second.Discount != 1 || third.Discount != 2 {
		t.Fatal("not minimum-discount allocation")
	}
	if _, err := a.createOrder(ctx, m, values("full", "10.00")); !errors.Is(err, errNoAmount) {
		t.Fatal("capacity not enforced", err)
	}
	independent := create(t, a, other, "first", "10.00")
	if independent.PayAmount != first.PayAmount {
		t.Fatal("merchant pools not independent")
	}
	time.Sleep(2 * time.Millisecond)
	stamp := time.Now()
	w := notice(t, a, second, h, stamp)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"matched"`) {
		t.Fatal(w.Body.String())
	}
	reused := create(t, a, m, "reused", "10.00")
	if reused.Discount != 1 {
		t.Fatal("hole was not reused")
	}
	// Old duplicate must not pay the new order occupying the same amount.
	w = notice(t, a, second, h, stamp)
	if !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatal(w.Body.String())
	}
	var state string
	_ = a.db.QueryRow(ctx, "SELECT status FROM orders WHERE trade_no=$1", reused.TradeNo).Scan(&state)
	if state != "pending" {
		t.Fatal("old notice settled reused slot")
	}
	_, _ = a.db.Exec(ctx, "UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE trade_no=$1", third.TradeNo)
	if err := a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	var capacity int
	_ = a.db.QueryRow(ctx, "SELECT capacity FROM amount_pools WHERE merchant_id=$1 AND amount=1000", m.ID).Scan(&capacity)
	if capacity != 2 {
		t.Fatalf("tail did not shrink: %d", capacity)
	}
	reexpanded := create(t, a, m, "expanded", "10.00")
	if reexpanded.Discount != 2 {
		t.Fatal("pool did not re-expand")
	}
	_, _ = a.db.Exec(ctx, "UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE merchant_id=$1 AND status='pending'", m.ID)
	if err := a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	_ = a.db.QueryRow(ctx, "SELECT capacity FROM amount_pools WHERE merchant_id=$1 AND amount=1000", m.ID).Scan(&capacity)
	if capacity != 0 {
		t.Fatal("empty pool did not shrink to zero")
	}
	if create(t, a, m, "fresh", "10.00").Discount != 0 {
		t.Fatal("expired slots not reusable")
	}
}
func TestCrossAmountCollisionAndConcurrentAllocation(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "concurrent", 30)
	first := create(t, a, m, "ten", "10.00")
	other := create(t, a, m, "ten-one", "10.01")
	third := create(t, a, m, "ten-two", "10.01")
	if first.PayAmount == other.PayAmount || first.PayAmount == third.PayAmount || other.PayAmount == third.PayAmount {
		t.Fatal("cross-amount collision")
	}
	if third.Discount != 2 {
		t.Fatalf("expected skip occupied 10.00: %d", third.Discount)
	}
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	amounts := make(chan int64, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o, e := a.createOrder(context.Background(), m, values(fmt.Sprint("parallel-", i), "20.00"))
			if e != nil {
				errs <- e
			} else {
				amounts <- o.PayAmount
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	close(amounts)
	for err := range errs {
		t.Error(err)
	}
	seen := map[int64]bool{}
	for amount := range amounts {
		if seen[amount] {
			t.Fatal("duplicate amount", amount)
		}
		seen[amount] = true
	}
	if len(seen) != n {
		t.Fatal("missing concurrent orders")
	}
	if _, err := a.createOrder(context.Background(), m, values("too-small", "0.00")); err == nil {
		t.Fatal("zero payable amount allowed")
	}
}
func TestMerchantAccessAndSingleDeviceBinding(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "merchant_a", 3)
	other := testMerchant(t, a, "merchant_b", 3)
	h := session(t, a, m.Username, "merchant-password-123")
	admin := session(t, a, "admin", a.cfg.AdminPassword)
	for _, path := range []string{"settings", "orders", "device", "events", "pools", "dashboard"} {
		w := request(t, a, "GET", fmt.Sprintf("/api/manage/%s?merchant_id=%d", path, other.ID), nil, h)
		if w.Code != 403 {
			t.Fatalf("cross-merchant %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"merchants", "smtp"} {
		w := request(t, a, "GET", "/api/manage/"+path, nil, h)
		if w.Code != 403 {
			t.Fatal("non-admin access", path, w.Code)
		}
	}
	w := request(t, a, "GET", fmt.Sprintf("/api/manage/settings?merchant_id=%d", other.ID), nil, admin)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	dev := device(t, a, m)
	telemetry := Telemetry{Version: "2.0.0", Battery: 80, Network: "wifi", NetworkValidated: true, ListenerConnected: true, ServiceRunning: true}
	w = request(t, a, "POST", "/api/device/heartbeat", telemetry, dev)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	impostor := map[string]string{"Authorization": dev["Authorization"], "X-ANE-Device-ID": token()}
	w = request(t, a, "POST", "/api/device/heartbeat", telemetry, impostor)
	if w.Code != 409 {
		t.Fatal("second physical device accepted", w.Body.String())
	}
	w = request(t, a, "POST", "/api/manage/device", map[string]string{"name": "new phone"}, h)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request(t, a, "POST", "/api/device/heartbeat", telemetry, dev)
	if w.Code != 401 {
		t.Fatal("old token survived reset")
	}
	// A merchant cannot change another merchant by spoofing body fields.
	w = request(t, a, "PUT", "/api/manage/settings", map[string]any{"merchant_id": other.ID}, h)
	if w.Code != 400 {
		t.Fatal("unknown ownership field accepted")
	}
	w = request(t, a, "POST", "/api/manage/logout", map[string]any{}, map[string]string{"Cookie": h["Cookie"], "Origin": "https://evil.test"})
	if w.Code != 403 {
		t.Fatal("csrf accepted")
	}
}
func TestEPayCallbackAndKeyRotation(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "callback", 10)
	var received url.Values
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		received = r.URL.Query()
		if calls == 1 {
			w.WriteHeader(503)
			fmt.Fprint(w, "later")
		} else {
			fmt.Fprint(w, "success")
		}
	}))
	defer s.Close()
	v := values("epay-1", "10.00")
	v.Set("pid", fmt.Sprint(m.ID))
	v.Set("notify_url", s.URL)
	v.Set("sign", core.Sign(v, m.Key))
	v.Set("sign_type", "MD5")
	w := request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var data struct {
		TradeNo string `json:"trade_no"`
		Pay     string `json:"pay_money"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &data)
	if data.Pay != "10.00" {
		t.Fatal(w.Body.String())
	}
	o, err := a.createOrder(context.Background(), m, v)
	if err != nil || o.TradeNo != data.TradeNo {
		t.Fatal("idempotency failed", err)
	}
	changed := values("epay-1", "11.00")
	if _, err = a.createOrder(context.Background(), m, changed); !errors.Is(err, errDifferentOrder) {
		t.Fatal("changed order accepted", err)
	}
	v.Set("money", "99.00")
	w = request(t, a, "GET", "/mapi.php?"+v.Encode(), nil, nil)
	if w.Code != 400 {
		t.Fatal("tampered signature accepted")
	}
	dev := device(t, a, m)
	time.Sleep(2 * time.Millisecond)
	w = notice(t, a, o, dev, time.Now())
	if w.Code != 200 || !strings.Contains(w.Body.String(), "matched") {
		t.Fatal(w.Body.String())
	}
	_, _ = a.db.Exec(context.Background(), "UPDATE merchant_apps SET merchant_key=$1 WHERE merchant_id=$2 AND name='default'", token(), m.ID)
	if !a.callbackOnce(context.Background()) {
		t.Fatal("missing callback")
	}
	_, _ = a.db.Exec(context.Background(), "UPDATE orders SET callback_next_at=now() WHERE trade_no=$1", o.TradeNo)
	if !a.callbackOnce(context.Background()) {
		t.Fatal("missing retry")
	}
	if !core.Verify(received, m.Key) || received.Get("money") != "10.00" || received.Get("pid") != fmt.Sprint(m.ID) {
		t.Fatal("callback did not preserve original amount and key")
	}
	var status string
	_ = a.db.QueryRow(context.Background(), "SELECT callback_status FROM orders WHERE trade_no=$1", o.TradeNo).Scan(&status)
	if status != "success" {
		t.Fatal(status)
	}
	w = request(t, a, "GET", "/api/pay/"+o.TradeNo, nil, nil)
	if !strings.Contains(w.Body.String(), `"status":"paid"`) {
		t.Fatal(w.Body.String())
	}
}
func TestLateNotificationDoesNotConsumeReusedExpiredSlot(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "late", 5)
	o := create(t, a, m, "old", "3.00")
	dev := device(t, a, m)
	ctx := context.Background()
	// Give the old order a historical interval. A new order can reuse its amount.
	_, _ = a.db.Exec(ctx, "UPDATE orders SET created_at=now()-interval '10 minutes',expires_at=now()-interval '1 minute' WHERE trade_no=$1", o.TradeNo)
	recent := create(t, a, m, "new", "3.00")
	if recent.PayAmount != o.PayAmount {
		t.Fatal("expired amount not reused")
	}
	w := notice(t, a, o, dev, time.Now().Add(-2*time.Minute))
	if !strings.Contains(w.Body.String(), `"status":"matched"`) {
		t.Fatal(w.Body.String())
	}
	var status string
	_ = a.db.QueryRow(ctx, "SELECT status FROM orders WHERE trade_no=$1", recent.TradeNo).Scan(&status)
	if status != "pending" {
		t.Fatal("late notice paid new order")
	}
}
func TestNotificationsDeduplicateAndRespectSubscriptions(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "notify", 5)
	dev := device(t, a, m)
	ctx := context.Background()
	_, err := a.db.Exec(ctx, "INSERT INTO notification_subscriptions(merchant_id,email,topics,apps) VALUES($1,'merchant@example.test',ARRAY['low_battery','device_offline','device_online','payment_received'],ARRAY['default'])", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.db.Exec(ctx, "INSERT INTO notification_subscriptions(merchant_id,email,topics) SELECT id,'admin@example.test',ARRAY['low_battery'] FROM merchants WHERE is_admin")
	telemetry := Telemetry{Version: "2.0", Battery: 10, Network: "wifi", WiFiSSID: "办公网络"}
	for i := 0; i < 2; i++ {
		w := request(t, a, "POST", "/api/device/heartbeat", telemetry, dev)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	var events, mails int
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM notification_events WHERE topic='low_battery'").Scan(&events)
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM mail_queue").Scan(&mails)
	if events != 1 || mails != 1 {
		t.Fatalf("alert dedup/merchant isolation: %d %d", events, mails)
	}
	_, _ = a.db.Exec(ctx, "UPDATE devices SET last_seen=now()-interval '5 minutes' WHERE merchant_id=$1", m.ID)
	if err = a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	if err = a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM notification_events WHERE topic='device_offline'").Scan(&events)
	if events != 1 {
		t.Fatal("offline alert repeated")
	}
	w := request(t, a, "POST", "/api/device/heartbeat", telemetry, dev)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM notification_events WHERE topic='device_online'").Scan(&events)
	if events != 1 {
		t.Fatal("missing recovery alert")
	}
	var subject, body string
	_ = a.db.QueryRow(ctx, "SELECT title,message FROM notification_events WHERE topic='device_online'").Scan(&subject, &body)
	if subject != "测试设备 恢复心跳，Wi-Fi 办公网络 在线" || !strings.Contains(body, "Wi-Fi 名称: 办公网络") {
		t.Fatal(subject, body)
	}

	_, _ = a.db.Exec(ctx, "UPDATE smtp_settings SET enabled=true,host='localhost',from_address='ane@example.test'")
	sent := 0
	a.sendMail = func(context.Context, SMTPConfig, string, string, string) error {
		sent++
		if sent == 1 {
			return errors.New("temporary SMTP failure")
		}
		return nil
	}
	if !a.mailOnce(ctx) {
		t.Fatal("missing mail")
	}
	var state string
	_ = a.db.QueryRow(ctx, "SELECT status FROM mail_queue ORDER BY id LIMIT 1").Scan(&state)
	if state != "retrying" {
		t.Fatal(state)
	}
	_, _ = a.db.Exec(ctx, "UPDATE mail_queue SET next_at=now()")
	if !a.mailOnce(ctx) {
		t.Fatal("missing mail retry")
	}
	_ = a.db.QueryRow(ctx, "SELECT status FROM mail_queue ORDER BY id LIMIT 1").Scan(&state)
	if state != "sent" {
		t.Fatal(state)
	}
	_, _ = a.db.Exec(ctx, "UPDATE notification_subscriptions SET topics='{}'")
	for a.mailOnce(ctx) {
	}
	if sent != 2 {
		t.Fatal("unsubscribed mail sent", sent)
	}
}
func TestPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "::1", "::ffff:127.0.0.1", "169.254.169.254", "100.64.1.1", "192.0.2.1", "2001:db8::1", "fc00::1"} {
		if publicIP(netip.MustParseAddr(s)) {
			t.Fatal("private address accepted", s)
		}
	}
}

func TestManagementConfigurationAndCredentialLifecycle(t *testing.T) {
	a := testApp(t)
	admin := session(t, a, "admin", a.cfg.AdminPassword)
	w := request(t, a, "POST", "/api/manage/merchants", map[string]any{"username": "managed_shop", "name": "新商户", "password": "new-merchant-password", "pool_limit": 4, "order_ttl_minutes": 15}, admin)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	merchant := session(t, a, "managed_shop", "new-merchant-password")
	settings := map[string]any{"name": "商户设置", "pool_limit": 2, "order_ttl_minutes": 20, "qr_image": qrFixture()}
	if w = request(t, a, "PUT", "/api/manage/settings", settings, merchant); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m, err := a.merchant(context.Background(), created.ID)
	if err != nil || m.PoolLimit != 2 || m.TTL != 20 || m.QR == "" || !strings.HasSuffix(m.Key, "_default") {
		t.Fatal("settings not persisted", m, err)
	}
	for _, path := range []string{"dashboard", "orders", "events", "pools", "device", "settings", "mail", "subscriptions"} {
		if w = request(t, a, "GET", "/api/manage/"+path, nil, merchant); w.Code != 200 {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
	}
	smtp := map[string]any{"enabled": false, "host": "localhost", "port": 587, "username": "ane", "password": "test-smtp-secret", "from_address": "ane@example.test", "from_name": "ANE", "security": "starttls"}
	if w = request(t, a, "PUT", "/api/manage/smtp", smtp, admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request(t, a, "GET", "/api/manage/smtp", nil, admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "test-smtp-secret") || !strings.Contains(w.Body.String(), `"has_password":true`) {
		t.Fatal("SMTP secret exposed", w.Body.String())
	}
	smtp["password"] = ""
	request(t, a, "PUT", "/api/manage/smtp", smtp, admin)
	stored, _ := a.smtpConfig(context.Background())
	if stored.Password != "test-smtp-secret" {
		t.Fatal("blank password did not preserve secret")
	}
	smtp["clear_password"] = true
	request(t, a, "PUT", "/api/manage/smtp", smtp, admin)
	stored, _ = a.smtpConfig(context.Background())
	if stored.Password != "" {
		t.Fatal("clear password failed")
	}
	patch := map[string]any{"name": "商户设置", "enabled": true, "password": "reset-merchant-password"}
	if w = request(t, a, "PATCH", fmt.Sprintf("/api/manage/merchants/%d", created.ID), patch, admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "GET", "/api/manage/me", nil, merchant); w.Code != 401 {
		t.Fatal("password reset did not revoke session")
	}
	merchant = session(t, a, "managed_shop", "reset-merchant-password")
	patch["enabled"] = false
	patch["password"] = ""
	request(t, a, "PATCH", fmt.Sprintf("/api/manage/merchants/%d", created.ID), patch, admin)
	if w = request(t, a, "GET", "/api/manage/me", nil, merchant); w.Code != 401 {
		t.Fatal("disabled merchant retained session")
	}
}

func TestConcurrentFirstDeviceBinding(t *testing.T) {
	a := testApp(t)
	m := testMerchant(t, a, "binding_race", 5)
	first := device(t, a, m)
	second := map[string]string{"Authorization": first["Authorization"], "X-ANE-Device-ID": token()}
	result := make(chan int, 2)
	var wg sync.WaitGroup
	for _, h := range []map[string]string{first, second} {
		wg.Add(1)
		go func(h map[string]string) {
			defer wg.Done()
			result <- request(t, a, "POST", "/api/device/heartbeat", Telemetry{Battery: 80, Network: "wifi"}, h).Code
		}(h)
	}
	wg.Wait()
	close(result)
	accepted, rejected := 0, 0
	for code := range result {
		switch code {
		case 200:
			accepted++
		case 409:
			rejected++
		default:
			t.Fatal("unexpected binding result", code)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatal("more than one device bound", accepted, rejected)
	}
}

func TestStartupKeepsCurrentDatabase(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	var adminKey string
	if err := a.db.QueryRow(ctx, `SELECT a.merchant_key FROM merchant_apps a
 JOIN merchants m ON m.id=a.merchant_id WHERE m.is_admin AND a.name='default'`).Scan(&adminKey); err != nil || !strings.HasSuffix(adminKey, "_default") {
		t.Fatal("fresh install did not create the default admin app", err)
	}
	m := testMerchant(t, a, "restart", 3)
	order := create(t, a, m, "existing", "10.00")
	if _, err := a.db.Exec(ctx, "UPDATE merchant_apps SET deleted_at=now() WHERE merchant_id=$1", m.ID); err != nil {
		t.Fatal(err)
	}

	restarted, err := New(ctx, a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	var gotKey string
	if err = restarted.db.QueryRow(ctx, "SELECT merchant_key FROM merchant_apps WHERE merchant_id=$1 AND deleted_at IS NOT NULL", m.ID).Scan(&gotKey); err != nil || gotKey != m.Key {
		t.Fatal("restart changed the app", err)
	}
	got, err := scanOrder(restarted.db.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", order.TradeNo))
	if err != nil || got.PayAmount != order.PayAmount || got.Status != order.Status {
		t.Fatal("restart changed the order", err)
	}
}

func TestUnsupportedDatabaseIsNotModified(t *testing.T) {
	a := testApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, version := range []int{1, 2, 3, 4, 5, 6, databaseSchemaVersion + 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			if _, err := a.db.Exec(ctx, "UPDATE schema_version SET version=$1", version); err != nil {
				t.Fatal(err)
			}
			another, err := New(ctx, a.cfg)
			if another != nil {
				another.Close()
				t.Fatal("unsupported database accepted")
			}
			if err == nil || !strings.Contains(err.Error(), "unsupported ANE database schema") {
				t.Fatal(err)
			}
			var got int
			if err := a.db.QueryRow(ctx, "SELECT version FROM schema_version").Scan(&got); err != nil || got != version {
				t.Fatal("unsupported database was modified", got, err)
			}
		})
	}
	if _, err := a.db.Exec(ctx, "DROP TABLE schema_version"); err != nil {
		t.Fatal(err)
	}
	another, err := New(ctx, a.cfg)
	if another != nil {
		another.Close()
		t.Fatal("nonempty unrecognized database accepted")
	}
	if err == nil || !strings.Contains(err.Error(), "requires an empty schema") {
		t.Fatal(err)
	}
}
