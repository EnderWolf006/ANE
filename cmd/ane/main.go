package main

import (
	"context"
	"github.com/ane/ane/internal/server"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := server.LoadConfig()
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := server.New(ctx, cfg)
	if err != nil {
		slog.Error("startup", "error", err)
		os.Exit(1)
	}
	defer app.Close()
	srv := &http.Server{Addr: cfg.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	go app.RunCallbacks(ctx)
	go app.RunMaintenance(ctx)
	go app.RunMail(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("ANE listening", "address", cfg.Listen, "public_url", cfg.BaseURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http", "error", err)
		os.Exit(1)
	}
}
