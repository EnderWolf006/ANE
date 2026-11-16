package server

import (
	"errors"
	"github.com/ane/ane/internal/core"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL, Listen, BaseURL, AdminUser, AdminPassword string
	AllowPrivateCallbacks                                  bool
}

func LoadConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("PG_URI"), Listen: env("LISTEN_ADDR", ":8080"), BaseURL: strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"), AdminUser: env("ADMIN_USERNAME", "admin"), AdminPassword: os.Getenv("ADMIN_PASSWORD"), AllowPrivateCallbacks: os.Getenv("ALLOW_PRIVATE_CALLBACKS") == "true"}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return c, errors.New("PG_URI must be a PostgreSQL connection URI")
	}
	if err = core.ValidateURL(c.BaseURL); err != nil {
		return c, errors.New("PUBLIC_BASE_URL must be a valid HTTP(S) origin")
	}
	u, _ = url.Parse(c.BaseURL)
	if u.Path != "" || u.RawQuery != "" {
		return c, errors.New("PUBLIC_BASE_URL must not contain a path or query")
	}
	if !validUsername(c.AdminUser) {
		return c, errors.New("ADMIN_USERNAME must contain 3..40 lowercase letters, digits, underscores or hyphens")
	}
	if !validPassword(c.AdminPassword) || strings.HasPrefix(c.AdminPassword, "replace-") {
		return c, errors.New("ADMIN_PASSWORD must contain 16..72 bytes and replace the example")
	}
	return c, nil
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
