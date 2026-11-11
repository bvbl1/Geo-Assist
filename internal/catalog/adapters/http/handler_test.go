package httpadapter

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geo-assist/internal/catalog/application"
	"geo-assist/internal/catalog/domain"
	httpx "geo-assist/internal/platform/httpserver"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"github.com/gin-gonic/gin"
)

type noWriteRepo struct{ writes int }

func (r *noWriteRepo) GetAny(context.Context, shared.ID) (domain.Place, error) {
	return domain.Place{}, app.E(404, "NOT_FOUND", "Place not found")
}
func (r *noWriteRepo) Create(context.Context, domain.Place) error        { r.writes++; return nil }
func (r *noWriteRepo) Update(context.Context, domain.Place, int64) error { r.writes++; return nil }
func (r *noWriteRepo) SoftDelete(context.Context, shared.ID, int64, time.Time) error {
	r.writes++
	return nil
}
func (r *noWriteRepo) Search(context.Context, application.SearchQuery) (app.Page[domain.PlaceHit], error) {
	return app.Page[domain.PlaceHit]{Items: []domain.PlaceHit{}}, nil
}

func TestCreateRejectsMissingCoordinatesNullAndUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"missing_longitude": `{"name":"Cafe","description":"Demo","category":"cafe","address":"Astana","location":{"latitude":51.12}}`,
		"null_location":     `{"name":"Cafe","description":"Demo","category":"cafe","address":"Astana","location":null}`,
		"unknown_nested":    `{"name":"Cafe","description":"Demo","category":"cafe","address":"Astana","location":{"longitude":71.43,"latitude":51.12,"extra":true}}`,
		"null_tags":         `{"name":"Cafe","description":"Demo","category":"cafe","address":"Astana","location":{"longitude":71.43,"latitude":51.12},"tags":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &noWriteRepo{}
			router := gin.New()
			h := NewHandler(application.NewService(repo))
			h.RegisterRoutes(router.Group("/api/v1"), func(c *gin.Context) { httpx.SetActor(c, app.Actor{UserID: "507f1f77bcf86cd799439012", Role: "admin"}) })
			request := httptest.NewRequest("POST", "/api/v1/places", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			want := 422
			if name == "unknown_nested" {
				want = 400
			}
			if response.Code != want {
				t.Fatalf("want %d, got %d: %s", want, response.Code, response.Body.String())
			}
			if repo.writes != 0 {
				t.Fatal("invalid request reached write")
			}
		})
	}
}

func TestPartialGeoAndRepeatedQueryAreRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewHandler(application.NewService(&noWriteRepo{}))
	h.RegisterRoutes(router.Group("/api/v1"), func(c *gin.Context) {})
	for path, want := range map[string]int{
		"/api/v1/places?longitude=71.43&latitude=51.12":             422,
		"/api/v1/places?limit=10&limit=20":                          400,
		"/api/v1/places?q=":                                         422,
		"/api/v1/places?longitude=NaN&latitude=51.12&radius_m=1000": 422,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != want {
			t.Fatalf("%s: want %d got %d", path, want, response.Code)
		}
	}
}
