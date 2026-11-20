package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	recmongo "geo-assist/internal/recommendation/adapters/mongo"
	rec "geo-assist/internal/recommendation/application"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	force := flag.Bool("force", false, "Rebuild even when the source is unchanged")
	status := flag.Bool("status", false, "Print model status without building")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rcfg, err := rec.ConfigFromEnv()
	if err != nil {
		return err
	}
	rcfg.AutoRebuild = false
	ctx, cancel := context.WithTimeout(context.Background(), rcfg.BuildTimeout+20*time.Second)
	defer cancel()
	client, err := mongostore.Connect(ctx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())
	repo := recmongo.New(client.Database(cfg.Database))
	service := rec.New(repo, rcfg)
	if err = service.Start(ctx); err != nil {
		return err
	}
	defer service.Close()
	if *status {
		data, err := service.Status(ctx, "")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(data)
	}
	a, err := service.RequestBuild("cli", *force)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			attempt, err := repo.Attempt(ctx, a.ID)
			if err != nil {
				return err
			}
			if attempt != nil && attempt.Status != "building" {
				if err = json.NewEncoder(os.Stdout).Encode(attempt); err != nil {
					return err
				}
				if attempt.Status == "failed" {
					return fmt.Errorf("model build failed")
				}
				return nil
			}
		}
	}
}
