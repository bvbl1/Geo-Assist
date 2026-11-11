package main

import (
	"context"
	"fmt"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	"log"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	if cfg.Env != "development" {
		return fmt.Errorf("demo seed is only available in development")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, e := mongostore.Connect(ctx, cfg.MongoURI)
	if e != nil {
		return e
	}
	defer client.Disconnect(context.Background())
	db := client.Database(cfg.Database)
	if e = mongostore.Migrate(ctx, db); e != nil {
		return e
	}
	created, e := mongostore.Seed(ctx, db)
	if e == nil {
		log.Printf("Demo import completed; newly imported: %t", created)
	}
	return e
}
