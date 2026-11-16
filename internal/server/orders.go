package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/ane/ane/internal/core"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Order struct {
	TradeNo          string     `json:"trade_no"`
	MerchantID       int64      `json:"merchant_id"`
	AppID            string     `json:"app_id"`
	PaymentType      string     `json:"payment_type"`
	OutTradeNo       string     `json:"out_trade_no"`
	Name             string     `json:"name"`
	Amount           int64      `json:"amount_cents"`
	PayAmount        int64      `json:"pay_amount_cents"`
	Discount         int        `json:"discount_cents"`
	Key              string     `json:"-"`
	NotifyURL        string     `json:"notify_url"`
	ReturnURL        string     `json:"return_url"`
	Param            string     `json:"param"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	PaidAt           *time.Time `json:"paid_at"`
	CallbackStatus   string     `json:"callback_status"`
	CallbackAttempts int        `json:"callback_attempts"`
	CallbackResult   string     `json:"callback_result"`
}

const orderColumns = "trade_no,merchant_id,app_id,payment_type,out_trade_no,name,amount,pay_amount,discount,merchant_key,notify_url,return_url,param,status,created_at,expires_at,paid_at,callback_status,callback_attempts,callback_result"

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.TradeNo, &o.MerchantID, &o.AppID, &o.PaymentType, &o.OutTradeNo, &o.Name, &o.Amount, &o.PayAmount, &o.Discount, &o.Key, &o.NotifyURL, &o.ReturnURL, &o.Param, &o.Status, &o.CreatedAt, &o.ExpiresAt, &o.PaidAt, &o.CallbackStatus, &o.CallbackAttempts, &o.CallbackResult)
	if err == nil && o.Status == "pending" && time.Now().After(o.ExpiresAt) {
		o.Status = "expired"
	}
	return o, err
}

type orderInputError struct{ error }

var errNoAmount = errors.New("优惠金额池已满或可用金额冲突，请稍后重试")
var errDifferentOrder = errors.New("out_trade_no 已存在，订单参数不一致")

func lockMerchant(ctx context.Context, tx pgx.Tx, id int64) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", id)
	return err
}

// Capacity is the highest occupied discount slot. Holes are reusable; an empty tail shrinks immediately.
func syncPools(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, "UPDATE orders SET status='expired' WHERE merchant_id=$1 AND status='pending' AND expires_at<=clock_timestamp()", id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE amount_pools p SET capacity=coalesce((SELECT max(discount)+1 FROM orders o WHERE o.merchant_id=p.merchant_id AND o.payment_type=p.payment_type AND o.amount=p.amount AND o.status='pending'),0) WHERE p.merchant_id=$1`, id)
	return err
}
func (a *App) createOrder(ctx context.Context, m Merchant, v url.Values) (Order, error) {
	var zero Order
	amount, err := core.ParseMoney(v.Get("money"))
	if err != nil {
		return zero, orderInputError{err}
	}
	if amount < 1 {
		return zero, orderInputError{errors.New("订单原金额至少为 0.01 元")}
	}
	if v.Get("type") != "alipay" && v.Get("type") != "wxpay" {
		return zero, orderInputError{errors.New("仅支持 alipay / wxpay")}
	}
	if len(v.Get("out_trade_no")) < 1 || len(v.Get("out_trade_no")) > 128 || len(v.Get("name")) < 1 || len(v.Get("name")) > 256 || len(v.Get("param")) > 2048 {
		return zero, orderInputError{errors.New("订单号、商品或附加参数无效")}
	}
	if err = a.validateCallbackURL(v.Get("notify_url")); err != nil {
		return zero, orderInputError{err}
	}
	if v.Get("return_url") != "" {
		if err = core.ValidateURL(v.Get("return_url")); err != nil {
			return zero, orderInputError{err}
		}
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback(ctx)
	if err = lockMerchant(ctx, tx, m.ID); err != nil {
		return zero, err
	}
	// Reload after locking, so policy changes, key rotation and disable operations are effective atomically.
	current, err := scanMerchant(tx.QueryRow(ctx, "SELECT "+merchantColumns+" FROM merchants WHERE id=$1 FOR UPDATE", m.ID))
	if err != nil {
		return zero, err
	}
	key := ""
	if m.AppID == "" {
		m.AppID = "default"
	}
	err = tx.QueryRow(ctx, "SELECT merchant_key,payment_note FROM merchant_apps WHERE merchant_id=$1 AND name=$2 AND enabled AND deleted_at IS NULL", m.ID, m.AppID).Scan(&key, &current.PaymentNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, orderInputError{errors.New("应用已删除、停用或密钥已变更")}
	}
	if err != nil {
		return zero, err
	}

	if !current.Enabled || !equal(key, m.Key) {
		return zero, orderInputError{errors.New("商户已停用或密钥已变更")}
	}
	current.Key, current.AppID = key, m.AppID
	m = current
	old, err := scanOrder(tx.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE merchant_id=$1 AND app_id=$2 AND out_trade_no=$3", m.ID, m.AppID, v.Get("out_trade_no")))
	if err == nil {
		if old.Amount != amount || old.Name != v.Get("name") || old.NotifyURL != v.Get("notify_url") || old.ReturnURL != v.Get("return_url") || old.Param != v.Get("param") || old.PaymentType != v.Get("type") {
			return zero, errDifferentOrder
		}
		return old, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return zero, err
	}
	if v.Get("type") == "wxpay" {
		m.QR = m.WechatQR
	}
	if m.QR == "" {
		return zero, orderInputError{errors.New("商户尚未配置该支付渠道的收款码")}
	}
	if err = syncPools(ctx, tx, m.ID); err != nil {
		return zero, err
	}
	var discount int
	err = tx.QueryRow(ctx, `SELECT slot FROM generate_series(0,$2::integer) slot WHERE NOT EXISTS(SELECT 1 FROM orders WHERE merchant_id=$1 AND payment_type=$4 AND status='pending' AND pay_amount=$3-slot) ORDER BY slot LIMIT 1`, m.ID, min(m.PoolLimit-1, int(amount-1)), amount, v.Get("type")).Scan(&discount)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, errNoAmount
	}
	if err != nil {
		return zero, err
	}
	o, err := scanOrder(tx.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,name,amount,pay_amount,discount,merchant_key,notify_url,return_url,param,qr_image,payment_note,expires_at,app_id,payment_type)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,clock_timestamp()+make_interval(mins=>$14),$15,$16) RETURNING `+orderColumns, token()[:32], m.ID, v.Get("out_trade_no"), v.Get("name"), amount, amount-int64(discount), discount, m.Key, v.Get("notify_url"), v.Get("return_url"), v.Get("param"), m.QR, m.PaymentNote, m.TTL, m.AppID, v.Get("type")))
	if err != nil {
		return zero, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO amount_pools(merchant_id,payment_type,amount,capacity) VALUES($1,$2,$3,$4) ON CONFLICT(merchant_id,payment_type,amount) DO UPDATE SET capacity=greatest(amount_pools.capacity,excluded.capacity)", m.ID, v.Get("type"), amount, discount+1)
	if err != nil {
		return zero, err
	}
	return o, tx.Commit(ctx)
}
func (a *App) signedRequest(w http.ResponseWriter, r *http.Request) (Merchant, url.Values, bool) {
	v, ok := readParams(w, r)
	if !ok {
		return Merchant{}, nil, false
	}
	id, err := strconv.ParseInt(v.Get("pid"), 10, 64)
	if err != nil {
		fail(w, 400, "商户号或签名无效")
		return Merchant{}, nil, false
	}
	m, err := a.merchant(r.Context(), id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return m, nil, false
	}
	if err != nil || !m.Enabled {
		fail(w, 400, "商户号或签名无效")
		return m, nil, false
	}
	{
		rows, err := a.db.Query(r.Context(), "SELECT name,merchant_key FROM merchant_apps WHERE merchant_id=$1 AND enabled AND deleted_at IS NULL ORDER BY name", m.ID)
		if err != nil {
			a.internal(w, err)
			return m, nil, false
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			var name, key string
			if err = rows.Scan(&name, &key); err != nil {
				a.internal(w, err)
				return m, nil, false
			}
			if core.Verify(v, key) {
				m.AppID, m.Key, found = name, key, true
			}
		}
		if err = rows.Err(); err != nil {
			a.internal(w, err)
			return m, nil, false
		}
		if !found {
			fail(w, 400, "商户号或签名无效")
			return m, nil, false
		}
	}
	return m, v, true
}
func (a *App) submit(w http.ResponseWriter, r *http.Request) {
	m, v, ok := a.signedRequest(w, r)
	if !ok {
		return
	}
	o, err := a.createOrder(r.Context(), m, v)
	if err != nil {
		a.orderError(w, err)
		return
	}
	http.Redirect(w, r, a.cfg.BaseURL+"/pay/"+o.TradeNo, http.StatusSeeOther)
}
func (a *App) mapi(w http.ResponseWriter, r *http.Request) {
	m, v, ok := a.signedRequest(w, r)
	if !ok {
		return
	}
	o, err := a.createOrder(r.Context(), m, v)
	if err != nil {
		a.orderError(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "msg": "success", "trade_no": o.TradeNo, "payurl": a.cfg.BaseURL + "/pay/" + o.TradeNo, "qrcode": a.cfg.BaseURL + "/api/pay/" + o.TradeNo + "/qr", "money": core.Money(o.Amount), "pay_money": core.Money(o.PayAmount), "discount": core.Money(int64(o.Discount))})
}
func (a *App) orderError(w http.ResponseWriter, err error) {
	if errors.Is(err, errNoAmount) || errors.Is(err, errDifferentOrder) {
		fail(w, 409, err.Error())
		return
	}
	var input orderInputError
	if errors.As(err, &input) {
		fail(w, 400, input.Error())
		return
	}
	a.internal(w, err)
}
func (a *App) epayQuery(w http.ResponseWriter, r *http.Request) {
	v, ok := readParams(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(v.Get("pid"), 10, 64)
	m, err := a.merchant(r.Context(), id)
	if err != nil || !m.Enabled {
		fail(w, 401, "商户号或密钥无效")
		return
	}
	{
		var key string
		err = a.db.QueryRow(r.Context(), "SELECT name,merchant_key FROM merchant_apps WHERE merchant_id=$1 AND merchant_key=$2 AND enabled AND deleted_at IS NULL", m.ID, v.Get("key")).Scan(&m.AppID, &key)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 401, "商户号或密钥无效")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		m.Key = key
	}
	switch v.Get("act") {
	case "query":
		var total, paid int
		var revenue int64
		err = a.db.QueryRow(r.Context(), "SELECT count(*),count(*) FILTER(WHERE status='paid'),coalesce(sum(amount) FILTER(WHERE status='paid'),0) FROM orders WHERE merchant_id=$1 AND app_id=$2", m.ID, m.AppID).Scan(&total, &paid, &revenue)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "pid": m.ID, "active": 1, "money": "0.00", "orders": total, "paid_orders": paid, "order_money": core.Money(revenue)})
	case "order":
		o, err := scanOrder(a.db.QueryRow(r.Context(), "SELECT "+orderColumns+" FROM orders WHERE merchant_id=$1 AND app_id=$4 AND (($2<>'' AND trade_no=$2) OR ($2='' AND out_trade_no=$3))", m.ID, v.Get("trade_no"), v.Get("out_trade_no"), m.AppID))
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "订单不存在")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, epayOrder(o))
	case "orders":
		limit, _ := strconv.Atoi(v.Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 20
		}
		offset, _ := strconv.Atoi(v.Get("offset"))
		if offset < 0 || offset > 1000000 {
			fail(w, 400, "offset 无效")
			return
		}
		rows, err := a.db.Query(r.Context(), "SELECT "+orderColumns+" FROM orders WHERE merchant_id=$1 AND app_id=$4 ORDER BY created_at DESC LIMIT $2 OFFSET $3", m.ID, limit, offset, m.AppID)
		if err != nil {
			a.internal(w, err)
			return
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			o, err := scanOrder(rows)
			if err != nil {
				a.internal(w, err)
				return
			}
			list = append(list, epayOrder(o))
		}
		if err = rows.Err(); err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "data": list})
	default:
		fail(w, 400, "支持 query / order / orders")
	}
}
func epayOrder(o Order) map[string]any {
	status := 0
	if o.Status == "paid" {
		status = 1
	}
	end := ""
	if o.PaidAt != nil {
		end = o.PaidAt.Format("2006-01-02 15:04:05")
	}
	return map[string]any{"code": 1, "pid": o.MerchantID, "trade_no": o.TradeNo, "out_trade_no": o.OutTradeNo, "type": o.PaymentType, "name": o.Name, "money": core.Money(o.Amount), "pay_money": core.Money(o.PayAmount), "discount": core.Money(int64(o.Discount)), "status": status, "addtime": o.CreatedAt.Format("2006-01-02 15:04:05"), "endtime": end, "param": o.Param}
}
func callbackValues(o Order) url.Values {
	v := url.Values{"pid": {strconv.FormatInt(o.MerchantID, 10)}, "trade_no": {o.TradeNo}, "out_trade_no": {o.OutTradeNo}, "type": {o.PaymentType}, "name": {o.Name}, "money": {core.Money(o.Amount)}, "trade_status": {"TRADE_SUCCESS"}, "param": {o.Param}}
	v.Set("sign", core.Sign(v, o.Key))
	v.Set("sign_type", "MD5")
	return v
}
func returnLink(o Order) string {
	if o.Status != "paid" || o.ReturnURL == "" {
		return ""
	}
	u, _ := url.Parse(o.ReturnURL)
	q := u.Query()
	for k, v := range callbackValues(o) {
		q[k] = v
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func (a *App) payStatus(w http.ResponseWriter, r *http.Request) {
	o, err := scanOrder(a.db.QueryRow(r.Context(), "SELECT "+orderColumns+" FROM orders WHERE trade_no=$1", r.PathValue("trade")))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "订单不存在")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	var note string
	if err = a.db.QueryRow(r.Context(), "SELECT payment_note FROM orders WHERE trade_no=$1", o.TradeNo).Scan(&note); err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"payment_type": o.PaymentType, "payment_note": sanitizePaymentNote(note), "code": 1, "trade_no": o.TradeNo, "out_trade_no": o.OutTradeNo, "name": o.Name, "amount_cents": o.Amount, "pay_amount_cents": o.PayAmount, "discount_cents": o.Discount, "status": o.Status, "expires_at": o.ExpiresAt, "server_time": time.Now(), "return_url": returnLink(o)})
}
func (a *App) collectionQR(w http.ResponseWriter, r *http.Request) {
	var s string
	err := a.db.QueryRow(r.Context(), "SELECT qr_image FROM orders WHERE trade_no=$1", r.PathValue("trade")).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	b, mime, err := parseImage(s)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	_, _ = w.Write(b)
}
func orderMessage(o Order) string {
	return fmt.Sprintf("订单 %s，实收 %s 元，渠道 %s\n应用：%s\n原金额 %s 元，优惠 %s 元。", o.OutTradeNo, core.Money(o.PayAmount), core.PaymentLabel(o.PaymentType), o.AppID, core.Money(o.Amount), core.Money(int64(o.Discount)))
}
