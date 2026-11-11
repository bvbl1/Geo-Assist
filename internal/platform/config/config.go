package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Env, HTTPAddr, Origin, MongoURI, Database string
	CookieSecure, AutoSeed                    bool
}

func Load() (Config, error) {
	c := Config{Env: get("APP_ENV", "development"), HTTPAddr: get("HTTP_ADDR", ":8080"), Origin: get("APP_ORIGIN", "http://localhost:8080"), MongoURI: get("MONGO_URI", "mongodb://localhost:27018/?replicaSet=rs0&directConnection=true"), Database: get("MONGO_DATABASE", "geoassist")}
	var err error
	c.CookieSecure, err = strconv.ParseBool(get("COOKIE_SECURE", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid COOKIE_SECURE")
	}
	c.AutoSeed, err = strconv.ParseBool(get("AUTO_SEED", "true"))
	if err != nil {
		return c, fmt.Errorf("invalid AUTO_SEED")
	}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return c, fmt.Errorf("APP_ORIGIN must be an exact http(s) origin without path")
	}
	if c.Env != "development" && c.Env != "production" {
		return c, fmt.Errorf("APP_ENV must be development or production")
	}
	if c.Env == "production" && (!c.CookieSecure || u.Scheme != "https" || c.AutoSeed) {
		return c, fmt.Errorf("production requires HTTPS, COOKIE_SECURE=true and AUTO_SEED=false")
	}
	if strings.ContainsAny(c.Database, " /\\.\"$") || c.Database == "" {
		return c, fmt.Errorf("invalid MONGO_DATABASE")
	}
	return c, nil
}
func get(k, fallback string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return fallback
}
