package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	app "geo-assist/internal/shared/application"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func httpParseQuery(s string) (url.Values, error) { return url.ParseQuery(s) }

func Middleware(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		id := uuid.NewString()
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic recovered", "request_id", id, "panic", p)
				Fail(c, app.E(500, "INTERNAL_ERROR", "Unexpected server error"))
			}
			slog.Info("http", "request_id", id, "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
		}()
		switch c.Request.Method {
		case "POST", "PUT", "PATCH", "DELETE":
			if c.GetHeader("Origin") != origin || c.GetHeader("X-GeoAssist-CSRF") != "1" {
				Fail(c, app.E(403, "CSRF_REJECTED", "Origin or CSRF header rejected"))
				return
			}
		}
		c.Next()
	}
}
