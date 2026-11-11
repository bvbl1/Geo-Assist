package main

import (
	"context"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, e := mongostore.Connect(ctx, cfg.MongoURI)
	if e != nil {
		return e
	}
	defer client.Disconnect(context.Background())
	return mongostore.Migrate(ctx, client.Database(cfg.Database))
}
