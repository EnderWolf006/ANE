package server

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"regexp"
	"strings"
)

var appNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

func (a *App) manageApps(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method == "GET" {
		a.jsonList(w, r, `SELECT jsonb_build_object('name',a.name,'merchant_key',a.merchant_key,'enabled',a.enabled,'payment_note',a.payment_note,'emails',coalesce((SELECT jsonb_agg(jsonb_build_object('id',s.id,'email',s.email) ORDER BY s.email) FROM notification_subscriptions s WHERE s.merchant_id=a.merchant_id AND a.name=ANY(s.apps)),'[]'::jsonb)) FROM merchant_apps a WHERE merchant_id=$1 AND deleted_at IS NULL ORDER BY name`, id)
		return
	}
	if r.Method != "POST" && r.Method != "PATCH" && r.Method != "PUT" && r.Method != "DELETE" {
		fail(w, 405, "不支持此方法")
		return
	}
	var in struct {
		Name    string  `json:"name"`
		Key     string  `json:"merchant_key"`
		Enabled *bool   `json:"enabled"`
		Rotate  bool    `json:"rotate"`
		Note    *string `json:"payment_note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !appNamePattern.MatchString(in.Name) || (in.Note != nil && len(*in.Note) > 8000) {
		fail(w, 400, "应用名或付款页文案无效")
		return
	}
	if in.Note != nil {
		n := sanitizePaymentNote(*in.Note)
		in.Note = &n
	}
	if r.Method == "POST" && in.Key == "" {
		in.Key = token() + "_" + in.Name
	}
	if (r.Method == "POST" || r.Method == "PUT") && (len(in.Key) > 256 || !strings.HasSuffix(in.Key, "_"+in.Name) || len(in.Key) <= len(in.Name)+1 || strings.ContainsAny(in.Key, "\r\n\x00")) {
		fail(w, 400, "密钥必须以 _应用名 结尾")
		return
	}
	if r.Method == "PATCH" && (in.Key != "" || (!in.Rotate && in.Enabled == nil && in.Note == nil)) {
		fail(w, 400, "请提供要修改的配置")
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
	if r.Method == "POST" {
		var count int
		if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM merchant_apps WHERE merchant_id=$1 AND deleted_at IS NULL", id).Scan(&count); err != nil {
			a.internal(w, err)
			return
		}
		if count >= 100 {
			fail(w, 400, "每个商户最多配置 100 个应用")
			return
		}
		// Deleted names stay reserved so new apps cannot inherit historical order numbers.
		_, err = tx.Exec(r.Context(), `INSERT INTO merchant_apps(merchant_id,name,merchant_key,enabled,payment_note) SELECT id,$2,$3,coalesce($4,true),coalesce($5,payment_note) FROM merchants WHERE id=$1`, id, in.Name, in.Key, in.Enabled, in.Note)
	} else if r.Method == "DELETE" {
		var name string
		err = tx.QueryRow(r.Context(), "UPDATE merchant_apps SET enabled=false,deleted_at=now() WHERE merchant_id=$1 AND name=$2 AND deleted_at IS NULL RETURNING name", id, in.Name).Scan(&name)
		if err == nil {
			_, err = tx.Exec(r.Context(), "UPDATE notification_subscriptions SET apps=array_remove(apps,$2) WHERE merchant_id=$1", id, in.Name)
		}
	} else {
		if in.Rotate {
			in.Key = token() + "_" + in.Name
		}
		err = tx.QueryRow(r.Context(), `UPDATE merchant_apps SET merchant_key=CASE WHEN $3='' THEN merchant_key ELSE $3 END,enabled=coalesce($4,enabled),payment_note=coalesce($5,payment_note) WHERE merchant_id=$1 AND name=$2 AND deleted_at IS NULL RETURNING merchant_key`, id, in.Name, in.Key, in.Enabled, in.Note).Scan(&in.Key)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "应用不存在")
		return
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(w, 409, "应用名或密钥已存在（已删除的应用名仍保留历史记录）")
			return
		}
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"code": 1, "name": in.Name, "merchant_key": in.Key})
}
