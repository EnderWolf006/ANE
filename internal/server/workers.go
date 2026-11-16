package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *App) RunCallbacks(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 8; i++ {
				if !a.callbackOnce(ctx) {
					break
				}
			}
		}
	}
}
func (a *App) callbackOnce(ctx context.Context) bool {
	o, err := scanOrder(a.db.QueryRow(ctx, `UPDATE orders SET callback_locked_until=now()+interval '60 seconds',callback_attempts=callback_attempts+1 WHERE trade_no=(SELECT trade_no FROM orders WHERE status='paid' AND callback_status IN ('pending','retrying') AND callback_next_at<=now() AND (callback_locked_until IS NULL OR callback_locked_until<now()) ORDER BY callback_next_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+orderColumns))
	if err != nil {
		return false
	}
	result := ""
	success := false
	if err = a.validateCallbackURL(o.NotifyURL); err == nil {
		u, _ := url.Parse(o.NotifyURL)
		q := u.Query()
		for k, v := range callbackValues(o) {
			q[k] = v
		}
		u.RawQuery = q.Encode()
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		err = e
		if err == nil {
			req.Header.Set("User-Agent", "ANE/2.0")
			res, e := a.callbackClient.Do(req)
			err = e
			if err == nil {
				body, e := io.ReadAll(io.LimitReader(res.Body, 4097))
				res.Body.Close()
				success = e == nil && len(body) <= 4096 && res.StatusCode >= 200 && res.StatusCode < 300 && strings.EqualFold(strings.TrimSpace(string(body)), "success")
				result = fmt.Sprintf("HTTP %d: %s", res.StatusCode, string(body))
				if e != nil {
					result = e.Error()
				}
			}
		}
	}
	if err != nil {
		result = err.Error()
	}
	result = clipText(result, 1000)
	status := "retrying"
	var next any
	if success {
		status = "success"
	} else if o.CallbackAttempts >= 12 {
		status = "failed"
	} else {
		next = time.Now().Add(time.Duration(1<<min(o.CallbackAttempts, 10)) * 15 * time.Second)
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "UPDATE orders SET callback_status=$1,callback_next_at=$2,callback_result=$3,callback_locked_until=NULL WHERE trade_no=$4", status, next, result, o.TradeNo)
	if err != nil {
		return false
	}
	if status == "failed" {
		if err = queueNotification(ctx, tx, o.MerchantID, "callback_failed", "callback:"+o.TradeNo, "商户回调失败", orderMessage(o)+"\n"+result); err != nil {
			return false
		}
	}
	return tx.Commit(ctx) == nil
}
func (a *App) RunMaintenance(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.maintenance(ctx); err != nil {
				slog.Error("maintenance", "error", err)
			}
		}
	}
}
func (a *App) maintenance(ctx context.Context) error {
	rows, err := a.db.Query(ctx, "SELECT id FROM merchants")
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		tx, err := a.db.Begin(ctx)
		if err != nil {
			return err
		}
		err = func() error {
			defer tx.Rollback(ctx)
			if e := lockMerchant(ctx, tx, id); e != nil {
				return e
			}
			if e := syncPools(ctx, tx, id); e != nil {
				return e
			}
			tag, e := tx.Exec(ctx, `UPDATE devices SET offline_alerted=true WHERE merchant_id=$1 AND NOT revoked AND NOT offline_alerted AND (last_seen<now()-interval '3 minutes' OR (last_seen IS NULL AND created_at<now()-interval '3 minutes')) AND EXISTS(SELECT 1 FROM merchants WHERE id=$1 AND enabled)`, id)
			if e != nil {
				return e
			}
			if tag.RowsAffected() > 0 {
				if e = queueNotification(ctx, tx, id, "device_offline", token(), "设备离线", "超过 3 分钟未收到设备心跳，请检查网络、通知权限和后台服务。"); e != nil {
					return e
				}
			}
			return tx.Commit(ctx)
		}()
		if err != nil {
			return err
		}
	}
	_, err = a.db.Exec(ctx, "DELETE FROM sessions WHERE expires_at<now()")
	return err
}
func (a *App) RunMail(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 5; i++ {
				if !a.mailOnce(ctx) {
					break
				}
			}
		}
	}
}
