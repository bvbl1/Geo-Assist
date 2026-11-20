package httpadapter

import (
	"context"
	cataloghttp "geo-assist/internal/catalog/adapters/http"
	catalog "geo-assist/internal/catalog/application"
	place "geo-assist/internal/catalog/domain"
	web "geo-assist/internal/platform/httpserver"
	rec "geo-assist/internal/recommendation/application"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func Register(g *gin.RouterGroup, auth gin.HandlerFunc, s *rec.Service) {
	g.GET("/me/recommendations", auth, func(c *gin.Context) {
		if !web.CheckQuery(c, "limit", "category", "tag", "longitude", "latitude", "radius_m") || !web.NoBody(c) {
			return
		}
		actor, ok := web.Actor(c)
		if !ok {
			return
		}
		limit := 10
		var err error
		if c.Request.URL.Query().Has("limit") {
			limit, err = strconv.Atoi(c.Query("limit"))
		}
		if err != nil || limit < 1 || limit > 50 {
			web.Fail(c, app.Validation("limit", "out_of_range"))
			return
		}
		q := catalog.SearchQuery{Category: place.Category(c.Query("category")), Tags: c.QueryArray("tag")}
		if c.Request.URL.Query().Has("category") && !place.ValidCategory(q.Category) {
			web.Fail(c, app.Validation("category", "invalid_category"))
			return
		}
		q.Tags, err = place.NormalizeTags(q.Tags)
		if err != nil {
			web.Fail(c, app.Validation("tag", "invalid_tags"))
			return
		}
		n := 0
		for _, key := range []string{"longitude", "latitude", "radius_m"} {
			if c.Request.URL.Query().Has(key) {
				n++
			}
		}
		if n != 0 && n != 3 {
			web.Fail(c, app.Validation("location", "all_geo_parameters_required"))
			return
		}
		if n == 3 {
			lon, e1 := strconv.ParseFloat(c.Query("longitude"), 64)
			lat, e2 := strconv.ParseFloat(c.Query("latitude"), 64)
			radius, e3 := strconv.Atoi(c.Query("radius_m"))
			center := place.Coordinates{Longitude: lon, Latitude: lat}
			if e1 != nil || e2 != nil || e3 != nil || place.ValidateCoordinates(center) != nil || radius < 1 || radius > 50000 {
				web.Fail(c, app.Validation("location", "invalid_geo_parameters"))
				return
			}
			q.Geo = &catalog.GeoQuery{Center: center, RadiusM: radius}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		items, meta, err := s.Recommend(ctx, actor.UserID, q, limit)
		if err != nil {
			c.Header("Retry-After", "5")
			web.Fail(c, err)
			return
		}
		data := make([]gin.H, 0, len(items))
		for _, item := range items {
			data = append(data, gin.H{"place": cataloghttp.ToResponse(item.Place.Place, item.Place.DistanceM), "score": item.Score, "reason": item.Reason})
		}
		c.JSON(200, gin.H{"data": data, "meta": meta})
	})
	admin := func(c *gin.Context) bool {
		actor, ok := web.Actor(c)
		if !ok {
			return false
		}
		if actor.Role != "admin" {
			web.Fail(c, app.E(403, "FORBIDDEN", "Administrator role required"))
			return false
		}
		return true
	}
	g.GET("/admin/recommendations/status", auth, func(c *gin.Context) {
		if !admin(c) || !web.CheckQuery(c, "build_id") || !web.NoBody(c) {
			return
		}
		id := c.Query("build_id")
		if c.Request.URL.Query().Has("build_id") && !shared.ValidID(id) {
			web.Fail(c, app.Validation("build_id", "invalid_id"))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		status, err := s.Status(ctx, id)
		if err != nil {
			web.Fail(c, err)
			return
		}
		web.Data(c, 200, status)
	})
	g.POST("/admin/recommendations/rebuild", auth, func(c *gin.Context) {
		if !admin(c) || !web.CheckQuery(c) || !web.NoBody(c) {
			return
		}
		attempt, err := s.RequestBuild("admin", false)
		if err != nil {
			c.Header("Retry-After", "60")
			web.Fail(c, err)
			return
		}
		web.Data(c, 202, gin.H{"build_id": attempt.ID, "status": attempt.Status})
	})
}
