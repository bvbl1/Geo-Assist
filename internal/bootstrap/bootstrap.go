// Package bootstrap wires concrete adapters to application use cases.
package bootstrap

import (
	"context"
	activityhttp "geo-assist/internal/activity/adapters/http"
	activitymongo "geo-assist/internal/activity/adapters/mongo"
	activityapp "geo-assist/internal/activity/application"
	cataloghttp "geo-assist/internal/catalog/adapters/http"
	catalogmongo "geo-assist/internal/catalog/adapters/mongo"
	catalogapp "geo-assist/internal/catalog/application"
	identityhttp "geo-assist/internal/identity/adapters/http"
	identitymongo "geo-assist/internal/identity/adapters/mongo"
	identityapp "geo-assist/internal/identity/application"
	"geo-assist/internal/platform/config"
	web "geo-assist/internal/platform/httpserver"
	"geo-assist/internal/platform/security"
	app "geo-assist/internal/shared/application"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"time"
)

func Router(cfg config.Config, db *mongo.Database) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.HandleMethodNotAllowed = true
	r.Use(web.Middleware(cfg.Origin))
	r.NoRoute(func(c *gin.Context) { web.Fail(c, app.E(404, "NOT_FOUND", "Route not found")) })
	r.NoMethod(func(c *gin.Context) { web.Fail(c, app.E(405, "METHOD_NOT_ALLOWED", "Method not allowed")) })
	r.GET("/health/live", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.Client().Ping(ctx, readpref.Primary()); err != nil {
			c.JSON(503, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})
	identity := identityapp.NewService(identitymongo.NewRepository(db), security.BcryptHasher{}, security.CryptoTokens{})
	ih := identityhttp.NewHandler(identity, cfg.CookieSecure)
	catalog := catalogapp.NewService(catalogmongo.NewRepository(db))
	activity := activityapp.NewService(activitymongo.NewRepository(db), catalog)
	v1 := r.Group("/api/v1")
	ih.RegisterRoutes(v1)
	cataloghttp.NewHandler(catalog).RegisterRoutes(v1, ih.RequireAuth())
	activityhttp.NewHandler(activity).RegisterRoutes(v1, ih.RequireAuth())
	return r
}
