package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	app "geo-assist/internal/shared/application"
	"geo-assist/internal/shared/domain"
	"github.com/gin-gonic/gin"
)

type Optional[T any] struct {
	Present, Null bool
	Value         T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Present = true
	o.Null = bytes.Equal(bytes.TrimSpace(b), []byte("null"))
	if o.Null {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(&o.Value)
}
func Fail(c *gin.Context, err error) {
	var e *app.Error
	if !errors.As(err, &e) {
		slog.Error("request failed", "request_id", c.GetString("request_id"), "error", err)
		e = &app.Error{Status: 500, Code: "INTERNAL_ERROR", Message: "Unexpected server error"}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			e = &app.Error{Status: 503, Code: "SERVICE_UNAVAILABLE", Message: "Operation unavailable"}
		}
	}
	details := e.Details
	if details == nil {
		details = []app.Detail{}
	}
	c.AbortWithStatusJSON(e.Status, gin.H{"error": gin.H{"code": e.Code, "message": e.Message, "details": details, "request_id": c.GetString("request_id")}})
}
func Data(c *gin.Context, status int, value any) { c.JSON(status, gin.H{"data": value}) }
func List[T any](c *gin.Context, items []T, p app.Pagination, more bool) {
	if items == nil {
		items = []T{}
	}
	c.JSON(200, gin.H{"data": items, "pagination": gin.H{"limit": p.Limit, "offset": p.Offset, "has_more": more}})
}
func Decode(c *gin.Context, dst any) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		Fail(c, app.E(415, "UNSUPPORTED_MEDIA_TYPE", "Use application/json"))
		return false
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err = dec.Decode(dst); err != nil {
		decodeError(c, err)
		return false
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		decodeError(c, err)
		return false
	}
	return true
}
func decodeError(c *gin.Context, err error) {
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		Fail(c, app.E(413, "PAYLOAD_TOO_LARGE", "Body exceeds 64 KiB"))
		return
	}
	Fail(c, app.E(400, "BAD_REQUEST", "Invalid JSON request"))
}
func NoBody(c *gin.Context) bool {
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		decodeError(c, err)
		return false
	}
	if len(bytes.TrimSpace(b)) > 0 {
		Fail(c, app.E(400, "BAD_REQUEST", "This endpoint does not accept a body"))
		return false
	}
	return true
}
func CheckQuery(c *gin.Context, allowed ...string) bool {
	set := map[string]bool{}
	for _, s := range allowed {
		set[s] = true
	}
	values, err := httpParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		Fail(c, app.E(400, "BAD_REQUEST", "Invalid query string"))
		return false
	}
	for k, v := range values {
		if !set[k] || (len(v) > 1 && k != "tag") {
			Fail(c, app.E(400, "BAD_REQUEST", "Unknown or repeated query parameter: "+k))
			return false
		}
	}
	return true
}
func Actor(c *gin.Context) (app.Actor, bool) {
	v, ok := c.Get("actor")
	if !ok {
		return app.Actor{}, false
	}
	a, ok := v.(app.Actor)
	return a, ok
}
func SetActor(c *gin.Context, a app.Actor) { c.Set("actor", a) }
func ValidateID(c *gin.Context, s string) (domain.ID, bool) {
	if !domain.ValidID(s) {
		Fail(c, app.E(400, "BAD_REQUEST", "Invalid ID"))
		return "", false
	}
	return domain.ID(strings.ToLower(s)), true
}
func ParsePagination(c *gin.Context) (app.Pagination, bool) {
	p := app.Pagination{Limit: 20, Offset: 0}
	for _, key := range []string{"limit", "offset"} {
		s, exists := c.GetQuery(key)
		if !exists {
			if _, ok := c.Request.URL.Query()[key]; !ok {
				continue
			}
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			Fail(c, app.E(400, "BAD_REQUEST", "Invalid "+key))
			return p, false
		}
		if key == "limit" {
			if n < 1 || n > 100 {
				Fail(c, app.Validation(key, "out_of_range"))
				return p, false
			}
			p.Limit = n
		} else {
			if n < 0 || n > 10000 {
				Fail(c, app.Validation(key, "out_of_range"))
				return p, false
			}
			p.Offset = n
		}
	}
	return p, true
}
