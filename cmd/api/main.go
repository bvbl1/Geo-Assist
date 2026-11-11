package main

import (
	"context"
	"errors"
	"geo-assist/internal/bootstrap"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client, err := mongostore.Connect(startup, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() {
		closing, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = client.Disconnect(closing)
	}()
	db := client.Database(cfg.Database)
	if err = mongostore.Migrate(startup, db); err != nil {
		return err
	}
	if cfg.AutoSeed {
		if _, err = mongostore.Seed(startup, db); err != nil {
			return err
		}
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: bootstrap.Router(cfg, db), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	result := make(chan error, 1)
	go func() {
		slog.Info("api ready", "address", cfg.HTTPAddr, "origin", cfg.Origin)
		result <- server.ListenAndServe()
	}()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		closing, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		return server.Shutdown(closing)
	}
}
