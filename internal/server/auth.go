package server

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Merchant struct {
	AppID       string `json:"-"`
	PaymentNote string `json:"payment_note"`
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	IsAdmin     bool   `json:"is_admin"`
	Enabled     bool   `json:"enabled"`
	Key         string `json:"merchant_key,omitempty"`
	PoolLimit   int    `json:"pool_limit"`
	TTL         int    `json:"order_ttl_minutes"`
	QR          string `json:"qr_image,omitempty"`
}

const merchantColumns = "id,username,name,is_admin,enabled,merchant_key,pool_limit,order_ttl_minutes,qr_image,payment_note"

func scanMerchant(row pgx.Row) (Merchant, error) {
	var m Merchant
	err := row.Scan(&m.ID, &m.Username, &m.Name, &m.IsAdmin, &m.Enabled, &m.Key, &m.PoolLimit, &m.TTL, &m.QR, &m.PaymentNote)
	return m, err
}
func (a *App) merchant(ctx context.Context, id int64) (Merchant, error) {
	return scanMerchant(a.db.QueryRow(ctx, "SELECT "+merchantColumns+" FROM merchants WHERE id=$1", id))
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{3,40}$`)

func validUsername(s string) bool { return usernamePattern.MatchString(s) }
func validPassword(s string) bool { return len(s) >= 16 && len(s) <= 72 }

type actorKey struct{}

func actor(r *http.Request) Merchant { return r.Context().Value(actorKey{}).(Merchant) }
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	key := host + "|" + in.Username
	a.loginMu.Lock()
	now := time.Now()
	attempt := a.loginAttempts[key]
	if now.After(attempt.until) {
		attempt = loginAttempt{until: now.Add(10 * time.Minute)}
	}
	if attempt.count >= 10 {
		a.loginMu.Unlock()
		fail(w, 429, "登录尝试过多，请 10 分钟后再试")
		return
	}
	if len(a.loginAttempts) >= 4096 {
		for k, v := range a.loginAttempts {
			if now.After(v.until) {
				delete(a.loginAttempts, k)
			}
		}
		if len(a.loginAttempts) >= 4096 {
			a.loginMu.Unlock()
			fail(w, 429, "请稍后重试")
			return
		}
	}
	attempt.count++
	a.loginAttempts[key] = attempt
	a.loginMu.Unlock()
	var id int64
	var hash string
	var enabled bool
	err := a.db.QueryRow(r.Context(), "SELECT id,password_hash,enabled FROM merchants WHERE username=$1", in.Username).Scan(&id, &hash, &enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	if err != nil {
		hash = string(a.dummyHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil || !enabled {
		fail(w, 401, "账号、密码错误或商户已停用")
		return
	}
	t := token()
	_, err = a.db.Exec(r.Context(), "INSERT INTO sessions(token_hash,merchant_id,expires_at) VALUES($1,$2,now()+interval '24 hours')", digest(t), id)
	if err != nil {
		a.internal(w, err)
		return
	}
	a.loginMu.Lock()
	delete(a.loginAttempts, key)
	a.loginMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "ane_session", Value: t, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.BaseURL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	ok(w)
}
func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.sameOrigin(w, r) {
			return
		}
		c, err := r.Cookie("ane_session")
		if err != nil || len(c.Value) != 64 {
			fail(w, 401, "请先登录")
			return
		}
		// Authentication needs no QR image; avoid fetching the potentially multi-megabyte data URL on every request.
		m, err := scanMerchant(a.db.QueryRow(r.Context(), "SELECT id,username,name,is_admin,enabled,merchant_key,pool_limit,order_ttl_minutes,'' AS qr_image,payment_note FROM merchants WHERE id=(SELECT merchant_id FROM sessions WHERE token_hash=$1 AND expires_at>now())", digest(c.Value)))
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 401, "会话已过期")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if !m.Enabled {
			fail(w, 401, "商户已停用")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, m)))
	})
}

// A missing selection means own merchant. Explicit "all" is read-only and admin-only.
func (a *App) scope(w http.ResponseWriter, r *http.Request, allowAll bool) (int64, bool) {
	m := actor(r)
	raw := r.URL.Query().Get("merchant_id")
	if raw == "" {
		return m.ID, true
	}
	if raw == "all" && allowAll && m.IsAdmin {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "请选择有效商户")
		return 0, false
	}
	if id != m.ID && !m.IsAdmin {
		fail(w, 403, "无权访问该商户")
		return 0, false
	}
	var exists bool
	err = a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM merchants WHERE id=$1)", id).Scan(&exists)
	if err != nil {
		a.internal(w, err)
		return 0, false
	}
	if !exists {
		fail(w, 404, "商户不存在")
		return 0, false
	}
	return id, true
}
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !actor(r).IsAdmin {
		fail(w, 403, "此操作仅限管理员")
		return false
	}
	return true
}
