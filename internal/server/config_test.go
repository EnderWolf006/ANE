package server

import "testing"

func TestConfig(t *testing.T) {
	t.Setenv("PG_URI", "postgres://ane@localhost/ane")
	t.Setenv("PUBLIC_BASE_URL", "https://ane.example.com")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "secure-admin-password")
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ k, v string }{{"PG_URI", ""}, {"PG_URI", "mysql://localhost/test"}, {"ADMIN_PASSWORD", "short"}, {"PUBLIC_BASE_URL", "https://example.com/path"}, {"ADMIN_USERNAME", "bad name"}} {
		t.Run(c.k+c.v, func(t *testing.T) {
			t.Setenv(c.k, c.v)
			if _, err := LoadConfig(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
