package server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/ane/ane/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var notificationTopics = map[string]string{"payment_received": "收款成功", "payment_unmatched": "收款未匹配", "callback_failed": "回调失败", "device_offline": "设备离线", "device_offline_reminder": "离线持续通知", "device_online": "设备恢复", "low_battery": "设备低电量"}

func queueNotification(ctx context.Context, tx pgx.Tx, merchant int64, topic, key, title, message string) error {
	var app any
	subject := title
	var username, merchantName string
	if err := tx.QueryRow(ctx, "SELECT username,name FROM merchants WHERE id=$1", merchant).Scan(&username, &merchantName); err != nil {
		return err
	}
	details := fmt.Sprintf("通知类型: %s\n商户: %s\n商户账号: %s\n商户 ID: %d\n发生时间: %s\n", title, merchantName, username, merchant, time.Now().Format(time.RFC3339))
	if strings.HasPrefix(key, "paid:") || strings.HasPrefix(key, "callback:") {
		trade := strings.SplitN(key, ":", 2)[1]
		o, err := scanOrder(tx.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", trade))
		if err != nil {
			return err
		}
		app = o.AppID
		var raw string
		if err = tx.QueryRow(ctx, "SELECT coalesce((SELECT raw_text FROM events WHERE trade_no=$1 LIMIT 1),'')", trade).Scan(&raw); err != nil {
			return err
		}
		subject = fmt.Sprintf("%s成功收款 ¥%s", core.PaymentLabel(o.PaymentType), core.Money(o.PayAmount))
		if topic == "callback_failed" {
			subject = fmt.Sprintf("应用 %s：订单 %s 回调失败", o.AppID, o.OutTradeNo)
		}
		if o.Discount > 0 && topic == "payment_received" {
			subject += fmt.Sprintf("（优惠 ¥%s）", core.Money(int64(o.Discount)))
		}
		if topic == "payment_received" {
			subject += " · " + o.AppID
		}
		details += fmt.Sprintf("应用: %s\n支付渠道: %s\n平台订单号: %s\n商户订单号: %s\n商品: %s\n原金额: %s 元\n实收: %s 元\n优惠: %s 元\n订单状态: %s\n创建时间: %s\n到期时间: %s\n回调地址: %s\n回调状态: %s\n回调尝试: %d\n回调结果: %s\n附加参数: %s\n收款摘要: %s\n", o.AppID, core.PaymentLabel(o.PaymentType), o.TradeNo, o.OutTradeNo, o.Name, core.Money(o.Amount), core.Money(o.PayAmount), core.Money(int64(o.Discount)), o.Status, o.CreatedAt.Format(time.RFC3339), o.ExpiresAt.Format(time.RFC3339), o.NotifyURL, o.CallbackStatus, o.CallbackAttempts, o.CallbackResult, o.Param, raw)
		if o.PaidAt != nil {
			details += "支付时间: " + o.PaidAt.Format(time.RFC3339) + "\n"
		}
	} else {
		var device, network, wifi, model, version string
		var battery, pending int
		var charging, listener, service bool
		var last *time.Time
		err := tx.QueryRow(ctx, "SELECT name,network,wifi_ssid,model,version,battery,charging,last_seen,listener_connected,service_running,pending_events FROM devices WHERE merchant_id=$1", merchant).Scan(&device, &network, &wifi, &model, &version, &battery, &charging, &last, &listener, &service, &pending)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if device == "" {
			device = merchantName + "的设备"
		}
		label := network
		if network == "wifi" {
			label = "Wi-Fi"
			if wifi != "" {
				label += " " + wifi
			}
		} else if network == "cellular" || network == "mobile" {
			label = "流量"
		}
		switch topic {
		case "device_online":
			subject = device + " 恢复心跳，" + label + " 在线"
		case "device_offline":
			subject = device + " 心跳中断，请检查设备"
		case "device_offline_reminder":
			subject = device + " 仍然离线，请检查设备"
		case "low_battery":
			subject = fmt.Sprintf("%s 电量仅剩 %d%%，请充电", device, battery)
		case "payment_unmatched":
			subject = device + " 收到未匹配收款，请核对"
		}
		details += fmt.Sprintf("设备名称: %s\n设备型号: %s\n客户端版本: %s\n网络: %s\nWi-Fi 名称: %s\n电量: %d%%\n充电中: %t\n通知监听: %t\n后台服务: %t\n待上传通知: %d\n", device, model, version, network, wifi, battery, charging, listener, service, pending)
		if last != nil {
			details += "最近心跳: " + last.Format(time.RFC3339) + "\n"
			if topic == "device_offline_reminder" {
				details += fmt.Sprintf("心跳中断时长: %d 分钟\n提醒频率: 每小时一次\n", max(0, int(time.Since(*last).Minutes())))
			}
		}
		if strings.HasPrefix(key, "unmatched:") {
			var paymentType, raw, reason, notificationID string
			var amount int64
			var posted, received time.Time
			err = tx.QueryRow(ctx, "SELECT payment_type,amount,raw_text,reason,notification_id,posted_at,received_at FROM events WHERE id=$1 AND merchant_id=$2", strings.TrimPrefix(key, "unmatched:"), merchant).Scan(&paymentType, &amount, &raw, &reason, &notificationID, &posted, &received)
			if err != nil {
				return err
			}
			subject = fmt.Sprintf("%s %s收款 ¥%s，未匹配订单", device, core.PaymentLabel(paymentType), core.Money(amount))
			details += fmt.Sprintf("支付渠道: %s\n收款金额: %s 元\n匹配结果: %s\n通知 ID: %s\n通知时间: %s\n上报时间: %s\n收款摘要: %s\n", core.PaymentLabel(paymentType), core.Money(amount), reason, notificationID, posted.Format(time.RFC3339), received.Format(time.RFC3339), raw)
		}
	}
	subject = clipText(strings.Join(strings.Fields(subject), " "), 180)
	details += "事件详情: " + message
	var id int64
	err := tx.QueryRow(ctx, "INSERT INTO notification_events(merchant_id,app_id,topic,dedupe_key,title,message) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(dedupe_key) DO NOTHING RETURNING id", merchant, app, topic, key, subject, details).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_queue(event_id,subscription_id,recipient_id,recipient,subject,body) SELECT $1,s.id,s.merchant_id,s.email,$2,$3 FROM notification_subscriptions s JOIN merchants m ON m.id=s.merchant_id WHERE m.enabled AND s.merchant_id=$4 AND $5=ANY(s.topics) AND ($6::text IS NULL OR $6=ANY(s.apps)) ON CONFLICT DO NOTHING`, id, subject, details, merchant, topic, app)
	return err
}

type SMTPConfig struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	Security    string `json:"security"`
}

func (a *App) smtpConfig(ctx context.Context) (SMTPConfig, error) {
	var s SMTPConfig
	err := a.db.QueryRow(ctx, "SELECT enabled,host,port,username,password,from_address,from_name,security FROM smtp_settings WHERE id=true").Scan(&s.Enabled, &s.Host, &s.Port, &s.Username, &s.Password, &s.FromAddress, &s.FromName, &s.Security)
	return s, err
}
func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && !strings.ContainsAny(s, "\r\n") && len(s) <= 254
}
func (a *App) subscriptions(w http.ResponseWriter, r *http.Request) {
	id, valid := a.scope(w, r, false)
	if !valid {
		return
	}
	if r.Method == "GET" {
		rows, err := a.db.Query(r.Context(), "SELECT id,email,topics,apps FROM notification_subscriptions WHERE merchant_id=$1 ORDER BY id", id)
		if err != nil {
			a.internal(w, err)
			return
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			var sid int64
			var email string
			var topics, apps []string
			if err = rows.Scan(&sid, &email, &topics, &apps); err != nil {
				a.internal(w, err)
				return
			}
			list = append(list, map[string]any{"id": sid, "email": email, "topics": topics, "apps": apps})
		}
		if err = rows.Err(); err != nil {
			a.internal(w, err)
			return
		}
		var enabled bool
		if err = a.db.QueryRow(r.Context(), "SELECT enabled FROM smtp_settings").Scan(&enabled); err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "data": list, "available_topics": notificationTopics, "smtp_enabled": enabled})
		return
	}
	if r.Method != "POST" && r.Method != "PUT" && r.Method != "DELETE" {
		fail(w, 405, "不支持此方法")
		return
	}
	var in struct {
		ID     int64    `json:"id"`
		Email  string   `json:"email"`
		Topics []string `json:"topics"`
		Apps   []string `json:"apps"`
	}
	if !decode(w, r, &in) {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err = lockMerchant(r.Context(), tx, id); err != nil {
		a.internal(w, err)
		return
	}
	if r.Method == "DELETE" {
		tag, e := tx.Exec(r.Context(), "DELETE FROM notification_subscriptions WHERE id=$1 AND merchant_id=$2", in.ID, id)
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			fail(w, 404, "邮箱订阅不存在")
			return
		}
	} else {
		in.Email = strings.TrimSpace(in.Email)
		if !validEmail(in.Email) || len(in.Apps) > 100 || len(in.Topics) > len(notificationTopics) {
			fail(w, 400, "邮箱或订阅选项无效")
			return
		}
		seen := map[string]bool{}
		for _, topic := range in.Topics {
			if _, ok := notificationTopics[topic]; !ok || seen[topic] {
				fail(w, 400, "通知类型无效或重复")
				return
			}
			seen[topic] = true
		}
		seen = map[string]bool{}
		for _, app := range in.Apps {
			var exists bool
			if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM merchant_apps WHERE merchant_id=$1 AND name=$2 AND deleted_at IS NULL)", id, app).Scan(&exists); err != nil {
				a.internal(w, err)
				return
			}
			if !exists || seen[app] {
				fail(w, 400, "应用不存在或重复")
				return
			}
			seen[app] = true
		}
		if in.Topics == nil {
			in.Topics = []string{}
		}
		if in.Apps == nil {
			in.Apps = []string{}
		}
		if r.Method == "POST" {
			var count int
			if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM notification_subscriptions WHERE merchant_id=$1", id).Scan(&count); err != nil {
				a.internal(w, err)
				return
			}
			if count >= 100 {
				fail(w, 400, "最多配置 100 个邮箱")
				return
			}
			err = tx.QueryRow(r.Context(), "INSERT INTO notification_subscriptions(merchant_id,email,topics,apps) VALUES($1,$2,$3,$4) RETURNING id", id, in.Email, in.Topics, in.Apps).Scan(&in.ID)
		} else {
			tag, e := tx.Exec(r.Context(), "UPDATE notification_subscriptions SET email=$1,topics=$2,apps=$3 WHERE merchant_id=$4 AND id=$5", in.Email, in.Topics, in.Apps, id, in.ID)
			err = e
			if err == nil && tag.RowsAffected() == 0 {
				fail(w, 404, "邮箱订阅不存在")
				return
			}
		}
	}
	if err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && p.Code == "23505" {
			fail(w, 409, "该邮箱已配置")
			return
		}
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "id": in.ID})
}
func (a *App) manageSMTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s, err := a.smtpConfig(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		has := s.Password != ""
		s.Password = ""
		respond(w, 200, map[string]any{"code": 1, "data": s, "has_password": has})
	case "PUT":
		var in struct {
			SMTPConfig
			ClearPassword bool `json:"clear_password"`
		}
		if !decode(w, r, &in) {
			return
		}
		s := in.SMTPConfig
		if s.Port < 1 || s.Port > 65535 || len(s.Host) > 253 || strings.ContainsAny(s.Host, "/\r\n :") || strings.ContainsAny(s.FromName, "\r\n") || len(s.FromName) > 100 || len(s.Username) > 254 || len(s.Password) > 1024 {
			fail(w, 400, "SMTP 参数无效")
			return
		}
		if s.Security != "starttls" && s.Security != "tls" && s.Security != "none" {
			fail(w, 400, "请选择 STARTTLS、TLS 或无加密")
			return
		}
		if s.Enabled && (s.Host == "" || !validEmail(s.FromAddress)) {
			fail(w, 400, "启用 SMTP 需要主机和有效发件邮箱")
			return
		}
		_, err := a.db.Exec(r.Context(), `UPDATE smtp_settings SET enabled=$1,host=$2,port=$3,username=$4,password=CASE WHEN $10 THEN '' WHEN $5='' THEN password ELSE $5 END,from_address=$6,from_name=$7,security=$8,updated_at=now() WHERE id=$9`, s.Enabled, s.Host, s.Port, s.Username, s.Password, s.FromAddress, s.FromName, s.Security, true, in.ClearPassword)
		if err != nil {
			a.internal(w, err)
			return
		}
		ok(w)
	default:
		fail(w, 405, "不支持此方法")
	}
}

// SMTP TLS is verified against the configured hostname; plaintext never transmits AUTH credentials.
func sendSMTP(ctx context.Context, s SMTPConfig, to, subject, body string) error {
	if !validEmail(to) || !validEmail(s.FromAddress) {
		return errors.New("invalid email address")
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	config := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	if s.Security == "tls" {
		tc := tls.Client(conn, config)
		if err = tc.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = tc
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support required STARTTLS")
		}
		if err = client.StartTLS(config); err != nil {
			return err
		}
	}
	if s.Username != "" {
		if s.Security == "none" {
			return errors.New("SMTP AUTH requires TLS or STARTTLS")
		}
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err = client.Mail(s.FromAddress); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	from := (&mail.Address{Name: s.FromName, Address: s.FromAddress}).String()
	content := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <" + token() + "@ane>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + encodeMailBody(body)
	if _, err = writer.Write([]byte(content)); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
func (a *App) mailOnce(ctx context.Context) bool {
	s, err := a.smtpConfig(ctx)
	if err != nil || !s.Enabled {
		return false
	}
	var id, recipientID int64
	var to, subject, body string
	var attempts int
	err = a.db.QueryRow(ctx, `UPDATE mail_queue SET locked_until=now()+interval '60 seconds',attempts=attempts+1 WHERE id=(SELECT id FROM mail_queue WHERE status IN ('pending','retrying') AND next_at<=now() AND (locked_until IS NULL OR locked_until<now()) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,recipient_id,recipient,subject,body,attempts`).Scan(&id, &recipientID, &to, &subject, &body, &attempts)
	if err != nil {
		return false
	}
	var allowed bool
	err = a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mail_queue q JOIN merchants m ON m.id=q.recipient_id JOIN notification_events e ON e.id=q.event_id JOIN notification_subscriptions s ON s.id=q.subscription_id WHERE q.id=$1 AND m.enabled AND s.merchant_id=m.id AND m.id=e.merchant_id AND s.email=q.recipient AND e.topic=ANY(s.topics) AND (e.app_id IS NULL OR e.app_id=ANY(s.apps))
 AND (e.topic<>'device_offline_reminder' OR EXISTS(
 SELECT 1 FROM devices d WHERE d.merchant_id=e.merchant_id AND NOT d.revoked
 AND coalesce(d.last_seen,d.created_at)<now()-interval '3 minutes'
 AND e.created_at>=coalesce(d.last_seen,d.created_at))))`, id).Scan(&allowed)
	if err != nil {
		return false
	}
	if !allowed {
		_, _ = a.db.Exec(ctx, "UPDATE mail_queue SET status='cancelled',locked_until=NULL,last_error='订阅已变更或设备已恢复/重置' WHERE id=$1", id)
		return true
	}
	err = a.sendMail(ctx, s, to, subject, body)
	status, last := "sent", ""
	var sent any = time.Now()
	next := time.Now()
	if err != nil {
		sent = nil
		last = clipText(err.Error(), 500)
		status = "retrying"
		if attempts >= 6 {
			status = "failed"
		}
		next = time.Now().Add(time.Duration(1<<min(attempts, 8)) * 30 * time.Second)
	}
	_, _ = a.db.Exec(ctx, "UPDATE mail_queue SET status=$1,last_error=$2,sent_at=$3,next_at=$4,locked_until=NULL WHERE id=$5", status, last, sent, next, id)
	return true
}
func clipText(s string, n int) string {
	r := []rune(strings.ToValidUTF8(s, "�"))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
func encodeMailBody(s string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(s))
	var b strings.Builder
	for len(encoded) > 76 {
		b.WriteString(encoded[:76])
		b.WriteString("\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded)
	b.WriteString("\r\n")
	return b.String()
}

// Explicit administrator action; sends only to the address supplied in the prompt.
func (a *App) testSMTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		fail(w, 405, "不支持此方法")
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if !validEmail(in.Email) {
		fail(w, 400, "请输入有效的测试邮箱")
		return
	}
	s, err := a.smtpConfig(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	if s.Host == "" || !validEmail(s.FromAddress) {
		fail(w, 400, "请先保存 SMTP 主机和发件邮箱")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err = a.sendMail(ctx, s, in.Email, "SMTP 测试邮件", "这是一封 ANE 测试邮件。收到此邮件说明当前 SMTP 配置可以正常发送。\n\nANE"); err != nil {
		fail(w, 502, "测试邮件发送失败："+clipText(err.Error(), 300))
		return
	}
	ok(w)
}
