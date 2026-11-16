package server

import (
	"encoding/json"
	"errors"
	"github.com/ane/ane/internal/core"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type EventInput struct {
	NotificationID string `json:"notification_id"`
	Package        string `json:"package"`
	Text           string `json:"text"`
	PostedAt       int64  `json:"posted_at"`
}

func (a *App) deviceEvent(w http.ResponseWriter, r *http.Request) {
	d, authorized := a.authDevice(w, r)
	if !authorized {
		return
	}
	var in EventInput
	if !decode(w, r, &in) {
		return
	}
	if core.PaymentType(in.Package) == "" || len(in.Text) > 2048 || len(in.NotificationID) < 1 || len(in.NotificationID) > 512 || in.PostedAt <= 0 {
		fail(w, 400, "通知参数或来源包名无效")
		return
	}
	posted := time.UnixMilli(in.PostedAt)
	if posted.After(time.Now().Add(2 * time.Minute)) {
		fail(w, 400, "通知时间超前，请校准手机时钟")
		return
	}
	canonical, _ := json.Marshal([]any{in.Package, strings.TrimSpace(in.Text), in.PostedAt})
	fingerprint := digest(string(canonical))
	id := token()[:32]
	paymentType := core.PaymentType(in.Package)
	amount, parseErr := core.ParseNotice(in.Package, in.Text)
	receipt := core.ReceiptText(paymentType, amount)
	status, reason := "unmatched", "没有符合金额及原始通知时间的订单"
	if parseErr != nil {
		status = "ignored"
		reason = parseErr.Error()
	}
	if posted.Before(time.Now().Add(-24 * time.Hour)) {
		status = "ignored"
		reason = "通知超过 24 小时，仅保留记录"
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err = lockMerchant(r.Context(), tx, d.MerchantID); err != nil {
		a.internal(w, err)
		return
	}
	bound, err := bindDevice(tx, r, d)
	if err != nil {
		a.internal(w, err)
		return
	}
	if !bound {
		fail(w, 409, "设备绑定已变化，请重新连接")
		return
	}
	var existingID, existingStatus string
	var existingTrade *string
	err = tx.QueryRow(r.Context(), "SELECT id,status,trade_no FROM events WHERE merchant_id=$1 AND (fingerprint=$2 OR (notification_id=$3 AND posted_at=$4 AND payment_type=$5))", d.MerchantID, fingerprint, in.NotificationID, posted, paymentType).Scan(&existingID, &existingStatus, &existingTrade)
	if err == nil {
		if err = tx.Commit(r.Context()); err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "duplicate": true, "event_id": existingID, "status": existingStatus, "trade_no": existingTrade})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	if err = syncPools(r.Context(), tx, d.MerchantID); err != nil {
		a.internal(w, err)
		return
	}
	var matched *Order
	if status == "unmatched" {
		// Include paid history before choosing: a stale/reposted notification must never consume a reused slot.
		rows, e := tx.Query(r.Context(), "SELECT "+orderColumns+" FROM orders WHERE merchant_id=$1 AND pay_amount=$2 AND payment_type=$4 AND created_at<=$3 AND expires_at>=$3 FOR UPDATE", d.MerchantID, amount, posted, paymentType)
		if e != nil {
			a.internal(w, e)
			return
		}
		candidates := []Order{}
		for rows.Next() {
			o, e := scanOrder(rows)
			if e != nil {
				rows.Close()
				a.internal(w, e)
				return
			}
			candidates = append(candidates, o)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			a.internal(w, err)
			return
		}
		if len(candidates) == 1 && candidates[0].Status != "paid" {
			matched = &candidates[0]
			status = "matched"
			reason = "实际金额及通知时间匹配"
		} else if len(candidates) > 1 {
			reason = "金额复用后的时间窗口存在歧义，请核对支付账单"
		} else if len(candidates) == 1 {
			reason = "对应时段的订单已经支付，未重复入账"
		}
	}
	var trade any
	if matched != nil {
		trade = matched.TradeNo
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO events(id,merchant_id,fingerprint,notification_id,raw_text,payment_type,amount,posted_at,status,reason,trade_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, d.MerchantID, fingerprint, in.NotificationID, receipt, paymentType, amount, posted, status, reason, trade)
	if err != nil {
		a.internal(w, err)
		return
	}
	if matched != nil {
		// End the historical matching window at server confirmation before releasing the amount.
		_, err = tx.Exec(r.Context(), "UPDATE orders SET status='paid',paid_at=$1,event_id=$2,expires_at=least(expires_at,clock_timestamp()),callback_status='pending',callback_next_at=now() WHERE trade_no=$3", posted, id, matched.TradeNo)
		if err != nil {
			a.internal(w, err)
			return
		}
		if err = syncPools(r.Context(), tx, d.MerchantID); err != nil {
			a.internal(w, err)
			return
		}
		err = queueNotification(r.Context(), tx, d.MerchantID, "payment_received", "paid:"+matched.TradeNo, "收款成功", orderMessage(*matched))
	} else if status == "unmatched" {
		err = queueNotification(r.Context(), tx, d.MerchantID, "payment_unmatched", "unmatched:"+id, "收款通知未匹配", receipt+"\n"+reason)
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "duplicate": false, "event_id": id, "status": status, "trade_no": trade, "reason": reason})
}
