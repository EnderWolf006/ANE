package server

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ane/ane/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) manageAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/manage/")
	switch path {
	case "me":
		if r.Method != "GET" {
			fail(w, 405, "不支持此方法")
			return
		}
		m := actor(r)
		m.Key = ""
		m.QR = ""
		respond(w, 200, map[string]any{"code": 1, "user": m, "base_url": a.cfg.BaseURL})
		return
	case "logout":
		if r.Method != "POST" {
			fail(w, 405, "不支持此方法")
			return
		}
		c, _ := r.Cookie("ane_session")
		_, err := a.db.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", digest(c.Value))
		if err != nil {
			a.internal(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "ane_session", Value: "", Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.BaseURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: -1})
		ok(w)
		return
	case "merchants":
		if !requireAdmin(w, r) {
			return
		}
		a.manageMerchants(w, r)
		return
	case "smtp/test":
		if !requireAdmin(w, r) {
			return
		}
		a.testSMTP(w, r)
		return
	case "smtp":
		if !requireAdmin(w, r) {
			return
		}
		a.manageSMTP(w, r)
		return
	case "subscriptions":
		a.subscriptions(w, r)
		return
	case "mail":
		if r.Method != "GET" {
			fail(w, 405, "不支持此方法")
			return
		}
		id, valid := a.scope(w, r, false)
		if !valid {
			return
		}
		a.mailHistory(w, r, id)
		return
	}
	if strings.HasPrefix(path, "merchants/") {
		if !requireAdmin(w, r) {
			return
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(path, "merchants/"), 10, 64)
		if err != nil {
			fail(w, 400, "商户 ID 无效")
			return
		}
		a.editMerchant(w, r, id)
		return
	}
	id, valid := a.scope(w, r, path == "dashboard" || path == "orders" || path == "orders/filters" || path == "events" || path == "pools")
	if !valid {
		return
	}
	switch {
	case path == "dashboard" && r.Method == "GET":
		a.dashboard(w, r, id)
	case path == "orders" && r.Method == "GET":
		a.adminOrders(w, r, id)
	case path == "orders/filters" && r.Method == "GET":
		a.jsonList(w, r, `SELECT jsonb_build_object('name',name) FROM (SELECT name FROM merchant_apps WHERE deleted_at IS NULL AND ($1::bigint=0 OR merchant_id=$1) UNION SELECT app_id AS name FROM orders WHERE ($1::bigint=0 OR merchant_id=$1)) apps ORDER BY name`, id)
	case path == "events" && r.Method == "GET":
		_, offset := page(r)
		a.jsonList(w, r, `SELECT to_jsonb(e) FROM (SELECT e.*,m.name AS merchant_name FROM events e JOIN merchants m ON m.id=e.merchant_id WHERE ($1::bigint=0 OR e.merchant_id=$1) ORDER BY e.received_at DESC LIMIT 31 OFFSET $2) e`, id, offset)
	case path == "pools" && r.Method == "GET":
		a.pools(w, r, id)
	case path == "device":
		a.manageDevice(w, r, id)
	case path == "settings":
		a.merchantSettings(w, r, id)
	case path == "apps":
		a.manageApps(w, r, id)
	case strings.HasPrefix(path, "retry/") && r.Method == "POST":
		tag, err := a.db.Exec(r.Context(), "UPDATE orders SET callback_status='pending',callback_attempts=0,callback_next_at=now() WHERE trade_no=$1 AND merchant_id=$2 AND status='paid' AND (callback_locked_until IS NULL OR callback_locked_until<now())", strings.TrimPrefix(path, "retry/"), id)
		if err != nil {
			a.internal(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 409, "订单不存在、未付款或回调执行中")
			return
		}
		ok(w)
	default:
		fail(w, 404, "接口不存在或不支持此方法")
	}
}
func (a *App) dashboard(w http.ResponseWriter, r *http.Request, id int64) {
	var b []byte
	err := a.db.QueryRow(r.Context(), `SELECT jsonb_build_object('orders',count(*),'paid',count(*) FILTER(WHERE status='paid'),'pending',count(*) FILTER(WHERE status='pending' AND expires_at>now()),'failed_callbacks',count(*) FILTER(WHERE callback_status='failed'),'revenue_cents',coalesce(sum(pay_amount) FILTER(WHERE status='paid'),0),'discount_cents',coalesce(sum(discount) FILTER(WHERE status='paid'),0),'today_cents',coalesce(sum(pay_amount) FILTER(WHERE status='paid' AND paid_at>=date_trunc('day',now())),0),'unmatched',(SELECT count(*) FROM events WHERE status='unmatched' AND ($1::bigint=0 OR merchant_id=$1)),'online_devices',(SELECT count(*) FROM devices WHERE NOT revoked AND last_seen>now()-interval '3 minutes' AND ($1::bigint=0 OR merchant_id=$1))) FROM orders WHERE ($1::bigint=0 OR merchant_id=$1)`, id).Scan(&b)
	if err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "data": json.RawMessage(b)})
}
func page(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	p = max(1, min(p, 50000))
	return p, (p - 1) * 30
}
func (a *App) adminOrders(w http.ResponseWriter, r *http.Request, id int64) {
	p, offset := page(r)
	q := r.URL.Query().Get("q")
	if len(q) > 128 {
		fail(w, 400, "搜索过长")
		return
	}
	v := r.URL.Query()
	status, callback, discount, period, sort := v.Get("status"), v.Get("callback_status"), v.Get("discount"), v.Get("period"), v.Get("sort")
	allowed := func(value string, options ...string) bool {
		if value == "" {
			return true
		}
		for _, option := range options {
			if value == option {
				return true
			}
		}
		return false
	}
	if !allowed(status, "pending", "paid", "expired") || !allowed(callback, "waiting", "pending", "retrying", "success", "failed") || !allowed(discount, "none", "discounted") || !allowed(period, "today", "7d", "30d", "90d") || !allowed(sort, "newest", "oldest", "amount_desc", "amount_asc") || len(v.Get("app_id")) > 128 {
		fail(w, 400, "筛选条件无效")
		return
	}
	start, end, err := datetimeRange(r)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}

	ordering := "created_at DESC,trade_no DESC"
	switch sort {
	case "oldest":
		ordering = "created_at ASC,trade_no ASC"
	case "amount_desc":
		ordering = "amount DESC,created_at DESC,trade_no DESC"
	case "amount_asc":
		ordering = "amount ASC,created_at DESC,trade_no DESC"
	}
	query := "SELECT " + orderColumns + ` FROM orders WHERE ($1::bigint=0 OR merchant_id=$1)
 AND ($2='' OR out_trade_no ILIKE '%'||$2||'%' OR trade_no ILIKE '%'||$2||'%' OR app_id ILIKE '%'||$2||'%' OR name ILIKE '%'||$2||'%')
 AND ($4='' OR CASE WHEN status='pending' AND expires_at<=now() THEN 'expired' ELSE status END=$4)
 AND ($5='' OR callback_status=$5) AND (NOT $6::boolean OR app_id=$7)
 AND ($8='' OR ($8='none' AND discount=0) OR ($8='discounted' AND discount>0))
 AND ($9='' OR ($9='today' AND created_at>=date_trunc('day',now())) OR ($9='7d' AND created_at>=now()-interval '7 days') OR ($9='30d' AND created_at>=now()-interval '30 days') OR ($9='90d' AND created_at>=now()-interval '90 days'))
 AND ($10::timestamptz IS NULL OR created_at>=$10) AND ($11::timestamptz IS NULL OR created_at<=$11)
 ORDER BY ` + ordering
	args := []any{id, q, offset, status, callback, v.Has("app_id"), v.Get("app_id"), discount, period, start, end}
	exporting := v.Get("export") == "csv"
	// Keep parameter positions identical between paginated listing and complete export.
	if exporting {
		query += " LIMIT ALL OFFSET ($3::integer * 0)"
	} else {
		query += " LIMIT 31 OFFSET $3"
	}
	rows, err := a.db.Query(r.Context(), query, args...)

	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			a.internal(w, err)
			return
		}
		list = append(list, o)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	if exporting {
		var b bytes.Buffer
		b.WriteString("\xef\xbb\xbf")
		writer := csv.NewWriter(&b)
		_ = writer.Write([]string{"平台订单号", "商户订单号", "应用", "支付渠道", "商品", "原金额", "实收", "优惠", "状态", "回调", "创建时间", "付款时间"})
		for _, o := range list {
			paid := ""
			if o.PaidAt != nil {
				paid = o.PaidAt.Format(time.RFC3339)
			}
			record := []string{o.TradeNo, o.OutTradeNo, o.AppID, core.PaymentLabel(o.PaymentType), o.Name, core.Money(o.Amount), core.Money(o.PayAmount), core.Money(int64(o.Discount)), o.Status, o.CallbackStatus, o.CreatedAt.Format(time.RFC3339), paid}
			for i, value := range record {
				trimmed := strings.TrimSpace(value)
				if len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
					record[i] = "'" + value
				}
			}
			if err := writer.Write(record); err != nil {
				a.internal(w, err)
				return
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			a.internal(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="orders.csv"`)
		_, _ = w.Write(b.Bytes())
		return
	}

	more := len(list) > 30
	if more {
		list = list[:30]
	}
	respond(w, 200, map[string]any{"code": 1, "data": list, "page": p, "has_more": more})
}
func (a *App) mailHistory(w http.ResponseWriter, r *http.Request, id int64) {
	p, offset := page(r)
	q, status, topic := r.URL.Query().Get("q"), r.URL.Query().Get("status"), r.URL.Query().Get("topic")
	if len(q) > 128 || (status != "" && status != "pending" && status != "retrying" && status != "sent" && status != "failed" && status != "cancelled") {
		fail(w, 400, "筛选条件无效")
		return
	}
	if topic != "" {
		if _, ok := notificationTopics[topic]; !ok {
			fail(w, 400, "通知类型无效")
			return
		}
	}
	start, end, err := datetimeRange(r)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}

	rows, err := a.db.Query(r.Context(), `SELECT to_jsonb(q)-'body' || jsonb_build_object('topic',e.topic,'merchant_username',m.username) FROM mail_queue q LEFT JOIN notification_events e ON e.id=q.event_id LEFT JOIN merchants m ON m.id=e.merchant_id WHERE q.recipient_id=$1 AND ($2='' OR q.status=$2) AND ($3='' OR e.topic=$3) AND ($4='' OR q.subject ILIKE '%'||$4||'%' OR q.recipient ILIKE '%'||$4||'%' OR m.username ILIKE '%'||$4||'%') AND ($6::timestamptz IS NULL OR q.created_at>=$6) AND ($7::timestamptz IS NULL OR q.created_at<=$7) ORDER BY q.created_at DESC,q.id DESC LIMIT 31 OFFSET $5`, id, status, topic, q, offset, start, end)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			a.internal(w, err)
			return
		}
		list = append(list, json.RawMessage(b))
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	more := len(list) > 30
	if more {
		list = list[:30]
	}
	respond(w, 200, map[string]any{"code": 1, "data": list, "page": p, "has_more": more})
}
func (a *App) pools(w http.ResponseWriter, r *http.Request, id int64) {
	// Derive occupancy using current expiry time, so the UI is exact even between maintenance ticks.
	a.jsonList(w, r, `SELECT jsonb_build_object('merchant_id',p.merchant_id,'merchant_name',m.name,'amount_cents',p.amount,'payment_type',p.payment_type,'pool_limit',m.pool_limit,'capacity',coalesce(max(o.discount)+1,0),'occupied',count(o.trade_no),'slots',coalesce(jsonb_agg(jsonb_build_object('discount',o.discount,'trade_no',o.trade_no,'out_trade_no',o.out_trade_no,'expires_at',o.expires_at) ORDER BY o.discount) FILTER(WHERE o.trade_no IS NOT NULL),'[]'::jsonb)) FROM amount_pools p JOIN merchants m ON m.id=p.merchant_id LEFT JOIN orders o ON o.merchant_id=p.merchant_id AND o.payment_type=p.payment_type AND o.amount=p.amount AND o.status='pending' AND o.expires_at>clock_timestamp() WHERE ($1::bigint=0 OR p.merchant_id=$1) GROUP BY p.merchant_id,p.payment_type,p.amount,m.name,m.pool_limit HAVING count(o.trade_no)>0 ORDER BY p.merchant_id,p.payment_type,p.amount LIMIT 100`, id)
}
func (a *App) manageMerchants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		a.jsonList(w, r, `SELECT jsonb_build_object('id',m.id,'username',m.username,'name',m.name,'is_admin',m.is_admin,'enabled',m.enabled,'pool_limit',m.pool_limit,'order_ttl_minutes',m.order_ttl_minutes,'app_count',(SELECT count(*) FROM merchant_apps WHERE merchant_id=m.id AND deleted_at IS NULL),'created_at',m.created_at,'has_qr',(m.qr_image<>'' OR m.wechat_qr_image<>''),'device_online',coalesce(NOT d.revoked AND d.last_seen>now()-interval '3 minutes',false)) FROM merchants m LEFT JOIN devices d ON d.merchant_id=m.id ORDER BY m.id`)
	case "POST":
		var in struct {
			Username  string `json:"username"`
			Name      string `json:"name"`
			Password  string `json:"password"`
			PoolLimit int    `json:"pool_limit"`
			TTL       int    `json:"order_ttl_minutes"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !validUsername(in.Username) || !validPassword(in.Password) || len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 100 || in.PoolLimit < 1 || in.PoolLimit > 9999 || in.TTL < 1 || in.TTL > 60 {
			fail(w, 400, "账号、密码、名称或收款策略无效，密码需 16–72 字节")
			return
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		var id int64
		err := a.db.QueryRow(r.Context(), "WITH m AS (INSERT INTO merchants(username,name,password_hash,merchant_key,pool_limit,order_ttl_minutes) VALUES($1,$2,$3,$4,$5,$6) RETURNING *) INSERT INTO merchant_apps(merchant_id,name,merchant_key,payment_note) SELECT id,'default',merchant_key,payment_note FROM m RETURNING merchant_id", in.Username, strings.TrimSpace(in.Name), string(h), token()+"_default", in.PoolLimit, in.TTL).Scan(&id)
		if err != nil {
			var p *pgconn.PgError
			if errors.As(err, &p) && p.Code == "23505" {
				fail(w, 409, "登录账号已存在")
				return
			}
			a.internal(w, err)
			return
		}
		respond(w, 201, map[string]any{"code": 1, "id": id})
	default:
		fail(w, 405, "不支持此方法")
	}
}
func (a *App) editMerchant(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != "PATCH" {
		fail(w, 405, "不支持此方法")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Enabled  bool   `json:"enabled"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 100 || (in.Password != "" && !validPassword(in.Password)) {
		fail(w, 400, "名称或新密码无效")
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
	m, err := scanMerchant(tx.QueryRow(r.Context(), "SELECT "+merchantColumns+" FROM merchants WHERE id=$1 FOR UPDATE", id))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "商户不存在")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if m.IsAdmin && (!in.Enabled || in.Password != "") {
		fail(w, 400, "管理员账号不能停用，密码通过 env 管理")
		return
	}
	_, err = tx.Exec(r.Context(), "UPDATE merchants SET name=$1,enabled=$2,updated_at=now() WHERE id=$3", strings.TrimSpace(in.Name), in.Enabled, id)
	if err != nil {
		a.internal(w, err)
		return
	}
	if in.Password != "" {
		h, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		_, err = tx.Exec(r.Context(), "UPDATE merchants SET password_hash=$1 WHERE id=$2", string(h), id)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if in.Password != "" || !in.Enabled {
		_, err = tx.Exec(r.Context(), "DELETE FROM sessions WHERE merchant_id=$1", id)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	ok(w)
}
func (a *App) merchantSettings(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case "GET":
		m, err := a.merchant(r.Context(), id)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "data": m, "base_url": a.cfg.BaseURL})
	case "PUT":
		var in struct {
			Name        string  `json:"name"`
			QR          string  `json:"qr_image"`
			WechatQR    *string `json:"wechat_qr_image"`
			PaymentNote string  `json:"payment_note"`
			PoolLimit   int     `json:"pool_limit"`
			TTL         int     `json:"order_ttl_minutes"`
		}
		if !decode(w, r, &in) {
			return
		}
		if len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 100 || in.PoolLimit < 1 || in.PoolLimit > 9999 || in.TTL < 1 || in.TTL > 60 {
			fail(w, 400, "名称或金额池配置无效")
			return
		}
		if len(in.PaymentNote) > 8000 {
			fail(w, 400, "付款提示不能超过 8000 字节")
			return
		}
		in.PaymentNote = sanitizePaymentNote(in.PaymentNote)
		if in.QR != "" {
			if _, _, err := parseImage(in.QR); err != nil {
				fail(w, 400, err.Error())
				return
			}
		}
		if in.WechatQR != nil && *in.WechatQR != "" {
			if _, _, err := parseImage(*in.WechatQR); err != nil {
				fail(w, 400, err.Error())
				return
			}
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
		_, err = tx.Exec(r.Context(), "UPDATE merchants SET name=$1,qr_image=$2,pool_limit=$3,order_ttl_minutes=$4,payment_note=$6,wechat_qr_image=coalesce($7,wechat_qr_image),updated_at=now() WHERE id=$5", strings.TrimSpace(in.Name), in.QR, in.PoolLimit, in.TTL, id, in.PaymentNote, in.WechatQR)
		if err != nil {
			a.internal(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.internal(w, err)
			return
		}
		ok(w)
	default:
		fail(w, 405, "不支持此方法")
	}
}
func parseImage(value string) ([]byte, string, error) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 || len(value) > 3<<20 {
		return nil, "", errors.New("收款码需为小于 2 MiB 的 PNG/JPEG 图片")
	}
	mime := ""
	switch parts[0] {
	case "data:image/png;base64":
		mime = "image/png"
	case "data:image/jpeg;base64":
		mime = "image/jpeg"
	default:
		return nil, "", errors.New("仅支持 PNG/JPEG")
	}
	b, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(b) > 2<<20 || http.DetectContentType(b) != mime {
		return nil, "", errors.New("图片内容或大小无效")
	}
	return b, mime, nil
}

func datetimeRange(r *http.Request) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	for key, target := range map[string]**time.Time{"start": &start, "end": &end} {
		if raw := r.URL.Query().Get(key); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, nil, fmt.Errorf("日期时间格式无效")
			}
			*target = &t
		}
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, fmt.Errorf("开始时间不能晚于结束时间")
	}
	return start, end, nil
}
