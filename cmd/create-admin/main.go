package main

import (
	"context"
	"flag"
	"fmt"
	identitymongo "geo-assist/internal/identity/adapters/mongo"
	identity "geo-assist/internal/identity/application"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	"geo-assist/internal/platform/security"
	"golang.org/x/term"
	"log"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	email := flag.String("email", "", "Administrator email")
	name := flag.String("name", "Administrator", "Display name")
	flag.Parse()
	if *email == "" {
		return fmt.Errorf("-email is required")
	}
	fmt.Fprint(os.Stderr, "Password: ")
	password, e := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if e != nil {
		return e
	}
	defer clear(password)
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
	db := client.Database(cfg.Database)
	if e = mongostore.Migrate(ctx, db); e != nil {
		return e
	}
	svc := identity.NewService(identitymongo.NewRepository(db), security.BcryptHasher{}, security.CryptoTokens{})
	u, e := svc.CreateAdmin(ctx, identity.RegisterInput{Email: *email, Password: string(password), DisplayName: *name})
	if e != nil {
		return e
	}
	fmt.Println("Administrator created:", u.Email)
	return nil
}
