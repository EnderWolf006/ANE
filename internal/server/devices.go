package server

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type deviceAuth struct {
	MerchantID              int64
	TokenHash, Installation string
}

func (a *App) authDevice(w http.ResponseWriter, r *http.Request) (deviceAuth, bool) {
	var d deviceAuth
	h := r.Header.Get("Authorization")
	d.Installation = r.Header.Get("X-ANE-Device-ID")
	if !strings.HasPrefix(h, "Bearer ") || len(h) != 71 || len(d.Installation) < 16 || len(d.Installation) > 128 {
		fail(w, 401, "设备令牌或设备标识无效")
		return d, false
	}
	d.TokenHash = digest(h[7:])
	var bound string
	err := a.db.QueryRow(r.Context(), "SELECT d.merchant_id,d.installation_id FROM devices d JOIN merchants m ON m.id=d.merchant_id WHERE d.token_hash=$1 AND NOT d.revoked AND m.enabled", d.TokenHash).Scan(&d.MerchantID, &bound)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "设备令牌无效或商户已停用")
		return d, false
	}
	if err != nil {
		a.internal(w, err)
		return d, false
	}
	if bound != "" && bound != d.Installation {
		fail(w, 409, "该商户已绑定另一台设备，请在 Web 面板重置绑定")
		return d, false
	}
	return d, true
}
func bindDevice(tx pgx.Tx, r *http.Request, d deviceAuth) (bool, error) {
	var id int64
	err := tx.QueryRow(r.Context(), `UPDATE devices SET installation_id=$2,bound_at=coalesce(bound_at,now()) WHERE merchant_id=$1 AND token_hash=$3 AND NOT revoked AND (installation_id='' OR installation_id=$2) AND EXISTS(SELECT 1 FROM merchants WHERE id=$1 AND enabled) RETURNING merchant_id`, d.MerchantID, d.Installation, d.TokenHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

type Telemetry struct {
	WiFiSSID          string `json:"wifi_ssid"`
	Version           string `json:"version"`
	Model             string `json:"model"`
	AndroidVersion    string `json:"android_version"`
	Battery           int    `json:"battery"`
	Charging          bool   `json:"charging"`
	Network           string `json:"network"`
	NetworkValidated  bool   `json:"network_validated"`
	ListenerConnected bool   `json:"listener_connected"`
	ServiceRunning    bool   `json:"service_running"`
	PendingEvents     int    `json:"pending_events"`
	UptimeSeconds     int64  `json:"uptime_seconds"`
}

func (a *App) heartbeat(w http.ResponseWriter, r *http.Request) {
	d, okAuth := a.authDevice(w, r)
	if !okAuth {
		return
	}
	var in Telemetry
	if !decode(w, r, &in) {
		return
	}
	if in.Battery < -1 || in.Battery > 100 || in.PendingEvents < 0 || in.PendingEvents > 1000000 || in.UptimeSeconds < 0 || len(in.WiFiSSID) > 128 || len(in.Version) > 64 || len(in.Model) > 128 || len(in.AndroidVersion) > 40 {
		fail(w, 400, "设备遥测数据无效")
		return
	}
	switch in.Network {
	case "wifi", "cellular", "ethernet", "vpn", "none", "other", "unknown":
	default:
		fail(w, 400, "网络类型无效")
		return
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
	var wasOffline, wasLow bool
	err = tx.QueryRow(r.Context(), "SELECT offline_alerted,low_battery_alerted FROM devices WHERE merchant_id=$1 FOR UPDATE", d.MerchantID).Scan(&wasOffline, &wasLow)
	if err != nil {
		a.internal(w, err)
		return
	}
	low := in.Battery >= 0 && in.Battery <= 20 && !in.Charging
	_, err = tx.Exec(r.Context(), `UPDATE devices SET last_seen=now(),version=$2,model=$3,android_version=$4,battery=$5,charging=$6,network=$7,network_validated=$8,listener_connected=$9,service_running=$10,pending_events=$11,uptime_seconds=$12,offline_alerted=false,low_battery_alerted=$13,wifi_ssid=$14 WHERE merchant_id=$1`, d.MerchantID, in.Version, in.Model, in.AndroidVersion, in.Battery, in.Charging, in.Network, in.NetworkValidated, in.ListenerConnected, in.ServiceRunning, in.PendingEvents, in.UptimeSeconds, low, in.WiFiSSID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if wasOffline {
		err = queueNotification(r.Context(), tx, d.MerchantID, "device_online", token(), "设备恢复在线", "ANE 已重新收到设备心跳。")
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if low && !wasLow {
		err = queueNotification(r.Context(), tx, d.MerchantID, "low_battery", token(), "设备电量偏低", "设备电量已低于或等于 20%，请及时连接充电器。")
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "server_time": time.Now().UnixMilli(), "merchant_id": d.MerchantID, "device_id": d.Installation})
}
func (a *App) manageDevice(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case "GET":
		a.jsonList(w, r, `SELECT to_jsonb(d)-'token_hash' || jsonb_build_object('online',NOT d.revoked AND d.last_seen>now()-interval '3 minutes','heartbeat_age_seconds',extract(epoch from now()-d.last_seen)::integer) FROM devices d WHERE merchant_id=$1`, id)
	case "PATCH":
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if len(in.Name) < 1 || len(in.Name) > 100 {
			fail(w, 400, "设备名称需为 1–100 字节")
			return
		}
		tag, err := a.db.Exec(r.Context(), "UPDATE devices SET name=$1 WHERE merchant_id=$2", in.Name, id)
		if err != nil {
			a.internal(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 404, "请先创建设备连接")
			return
		}
		ok(w)
	case "POST":
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if len(in.Name) < 1 || len(in.Name) > 100 {
			fail(w, 400, "设备名称需为 1–100 字节")
			return
		}
		t := token()
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
		_, err = tx.Exec(r.Context(), `INSERT INTO devices(merchant_id,name,token_hash) VALUES($1,$2,$3) ON CONFLICT(merchant_id) DO UPDATE SET name=excluded.name,token_hash=excluded.token_hash,installation_id='',revoked=false,bound_at=NULL,last_seen=NULL,version='',model='',android_version='',battery=-1,charging=false,network='unknown',wifi_ssid='',network_validated=false,listener_connected=false,service_running=false,pending_events=0,uptime_seconds=0,offline_alerted=false,low_battery_alerted=false,created_at=now()`, id, in.Name, digest(t))
		if err != nil {
			a.internal(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"code": 1, "token": t, "server_url": a.cfg.BaseURL})
	case "DELETE":
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
		_, err = tx.Exec(r.Context(), "UPDATE devices SET revoked=true WHERE merchant_id=$1", id)
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
