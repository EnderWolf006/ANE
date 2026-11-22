package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

//go:embed schema.sql web
var files embed.FS

const databaseSchemaVersion = 7

type App struct {
	cfg            Config
	db             *pgxpool.Pool
	dummyHash      []byte
	callbackClient *http.Client
	loginMu        sync.Mutex
	loginAttempts  map[string]loginAttempt
	sendMail       func(context.Context, SMTPConfig, string, string, string) error
}
type loginAttempt struct {
	count int
	until time.Time
}

func New(ctx context.Context, c Config) (*App, error) {
	pool, err := pgxpool.New(ctx, c.DatabaseURL)
	if err != nil {
		return nil, err
	}
	var tx pgx.Tx
	failNew := func(err error) (*App, error) {
		if tx != nil {
			_ = tx.Rollback(context.Background())
		}
		pool.Close()
		return nil, err
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		return failNew(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(89763211)"); err != nil {
		return failNew(err)
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass(format('%I.schema_version', current_schema())) IS NOT NULL").Scan(&exists); err != nil {
		return failNew(err)
	}
	if exists {
		var supported bool
		if err = tx.QueryRow(ctx, "SELECT count(*)=1 AND coalesce(min(version)=$1,false) FROM schema_version", databaseSchemaVersion).Scan(&supported); err != nil {
			return failNew(err)
		}
		if !supported {
			return failNew(fmt.Errorf("unsupported ANE database schema (expected %d); use an empty database for a fresh install", databaseSchemaVersion))
		}
	} else {
		var nonempty bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class
 WHERE relnamespace=current_schema()::regnamespace AND relkind IN ('r','p','v','m','f'))`).Scan(&nonempty); err != nil {
			return failNew(err)
		}
		if nonempty {
			return failNew(errors.New("ANE requires an empty schema for a fresh install"))
		}
		schema, e := files.ReadFile("schema.sql")
		if e != nil {
			return failNew(e)
		}
		if _, err = tx.Exec(ctx, string(schema)); err != nil {
			return failNew(err)
		}
	}
	var id int64
	var username, hash string
	err = tx.QueryRow(ctx, "SELECT id,username,password_hash FROM merchants WHERE is_admin").Scan(&id, &username, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		h, e := bcrypt.GenerateFromPassword([]byte(c.AdminPassword), bcrypt.DefaultCost)
		if e != nil {
			return failNew(e)
		}
		_, err = tx.Exec(ctx, `WITH m AS (
 INSERT INTO merchants(username,name,password_hash,is_admin,merchant_key)
 VALUES($1,'管理员',$2,true,$3) RETURNING id,merchant_key,payment_note
) INSERT INTO merchant_apps(merchant_id,name,merchant_key,payment_note)
 SELECT id,'default',merchant_key,payment_note FROM m`, c.AdminUser, string(h), token()+"_default")
	} else if err == nil && (username != c.AdminUser || bcrypt.CompareHashAndPassword([]byte(hash), []byte(c.AdminPassword)) != nil) {
		h, e := bcrypt.GenerateFromPassword([]byte(c.AdminPassword), bcrypt.DefaultCost)
		if e != nil {
			return failNew(e)
		}
		_, err = tx.Exec(ctx, "UPDATE merchants SET username=$1,password_hash=$2,enabled=true WHERE id=$3", c.AdminUser, string(h), id)
		if err == nil {
			_, err = tx.Exec(ctx, "DELETE FROM sessions WHERE merchant_id=$1", id)
		}
	}
	if err != nil {
		return failNew(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return failNew(err)
	}
	dummy, _ := bcrypt.GenerateFromPassword([]byte(token()), bcrypt.DefaultCost)
	a := &App{cfg: c, db: pool, dummyHash: dummy, loginAttempts: map[string]loginAttempt{}, sendMail: sendSMTP}
	a.callbackClient = a.newCallbackClient()
	return a, nil
}
func (a *App) Close() { a.callbackClient.CloseIdleConnections(); a.db.Close() }
func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]any{"code": -1, "msg": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "请求 JSON 无效")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "请求 JSON 包含多余数据")
		return false
	}
	return true
}
func (a *App) internal(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	fail(w, 500, "服务器暂时无法处理请求")
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.db.Ping(ctx); err != nil {
			fail(w, 503, "database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok", "version": "2.4.4"})
	})
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.Handle("/api/manage/", a.auth(http.HandlerFunc(a.manageAPI)))
	mux.HandleFunc("GET /submit.php", a.submit)
	mux.HandleFunc("POST /submit.php", a.submit)
	mux.HandleFunc("GET /mapi.php", a.mapi)
	mux.HandleFunc("POST /mapi.php", a.mapi)
	mux.HandleFunc("GET /api.php", a.epayQuery)
	mux.HandleFunc("GET /api/pay/{trade}", a.payStatus)
	mux.HandleFunc("GET /api/pay/{trade}/qr", a.collectionQR)
	mux.HandleFunc("POST /api/device/heartbeat", a.heartbeat)
	mux.HandleFunc("POST /api/device/events", a.deviceEvent)
	webFS, _ := fs.Sub(files, "web")
	mux.Handle("GET /assets/", http.FileServer(http.FS(webFS)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fail(w, 405, "不支持此方法")
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/admin" && !strings.HasPrefix(r.URL.Path, "/admin/") && !strings.HasPrefix(r.URL.Path, "/pay/") {
			http.NotFound(w, r)
			return
		}
		b, err := files.ReadFile("web/index.html")
		if err != nil {
			fail(w, 503, "请先运行 Web 构建")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", fmt.Sprint(v))
				fail(w, 500, "服务器错误")
			}
		}()
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a *App) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" {
		return true
	}
	if r.Header.Get("Origin") != a.cfg.BaseURL {
		fail(w, 403, "请从配置的 PUBLIC_BASE_URL 访问管理面板")
		return false
	}
	return true
}
func readParams(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		fail(w, 400, "请求参数无效")
		return nil, false
	}
	for _, vs := range r.Form {
		if len(vs) != 1 {
			fail(w, 400, "参数不能重复")
			return nil, false
		}
	}
	return r.Form, true
}
func (a *App) jsonList(w http.ResponseWriter, r *http.Request, sql string, args ...any) {
	rows, err := a.db.Query(r.Context(), sql, args...)
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
	respond(w, 200, map[string]any{"code": 1, "data": list})
}
func ok(w http.ResponseWriter) { respond(w, 200, map[string]int{"code": 1}) }
