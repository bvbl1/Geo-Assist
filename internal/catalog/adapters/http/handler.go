package httpadapter

import (
	"strconv"
	"time"

	"geo-assist/internal/catalog/application"
	"geo-assist/internal/catalog/domain"
	httpx "geo-assist/internal/platform/httpserver"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"

	"github.com/gin-gonic/gin"
)

type LocationResponse struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type PlaceResponse struct {
	ID          shared.ID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Category    domain.Category  `json:"category"`
	Tags        []string         `json:"tags"`
	Address     string           `json:"address"`
	Location    LocationResponse `json:"location"`
	Version     int64            `json:"version"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DistanceM   *float64         `json:"distance_m"`
}

func ToResponse(place domain.Place, distance *float64) PlaceResponse {
	return PlaceResponse{ID: place.ID, Name: place.Name, Description: place.Description, Category: place.Category,
		Tags: append([]string{}, place.Tags...), Address: place.Address,
		Location: LocationResponse{Longitude: place.Location.Longitude, Latitude: place.Location.Latitude},
		Version:  place.Version, CreatedAt: place.CreatedAt, UpdatedAt: place.UpdatedAt, DistanceM: distance}
}

type locationRequest struct {
	Longitude httpx.Optional[float64] `json:"longitude"`
	Latitude  httpx.Optional[float64] `json:"latitude"`
}

type createRequest struct {
	Name        httpx.Optional[string]          `json:"name"`
	Description httpx.Optional[string]          `json:"description"`
	Category    httpx.Optional[domain.Category] `json:"category"`
	Tags        httpx.Optional[[]string]        `json:"tags"`
	Address     httpx.Optional[string]          `json:"address"`
	Location    httpx.Optional[locationRequest] `json:"location"`
}

type updateRequest struct {
	ExpectedVersion httpx.Optional[int64]           `json:"expected_version"`
	Name            httpx.Optional[string]          `json:"name"`
	Description     httpx.Optional[string]          `json:"description"`
	Category        httpx.Optional[domain.Category] `json:"category"`
	Tags            httpx.Optional[[]string]        `json:"tags"`
	Address         httpx.Optional[string]          `json:"address"`
	Location        httpx.Optional[locationRequest] `json:"location"`
}

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(group *gin.RouterGroup, auth gin.HandlerFunc) {
	group.GET("/categories", h.Categories)
	group.GET("/places", h.Search)
	group.GET("/places/:id", h.Get)
	group.POST("/places", auth, h.Create)
	group.PATCH("/places/:id", auth, h.Update)
	group.DELETE("/places/:id", auth, h.Delete)
}

func require[T any](c *gin.Context, field string, value httpx.Optional[T]) bool {
	if !value.Present {
		httpx.Fail(c, app.Validation(field, "required"))
		return false
	}
	return notNull(c, field, value)
}

func notNull[T any](c *gin.Context, field string, value httpx.Optional[T]) bool {
	if value.Null {
		httpx.Fail(c, app.Validation(field, "null_not_allowed"))
		return false
	}
	return true
}

func coordinates(c *gin.Context, value locationRequest) (domain.Coordinates, bool) {
	if !require(c, "location.longitude", value.Longitude) || !require(c, "location.latitude", value.Latitude) {
		return domain.Coordinates{}, false
	}
	return domain.Coordinates{Longitude: value.Longitude.Value, Latitude: value.Latitude.Value}, true
}

func routeID(c *gin.Context) (shared.ID, bool) {
	id := c.Param("id")
	if !shared.ValidID(id) {
		httpx.Fail(c, app.E(400, "BAD_REQUEST", "Invalid place ID"))
		return "", false
	}
	return shared.ID(id), true
}

func (h *Handler) Categories(c *gin.Context) {
	if !httpx.CheckQuery(c) || !httpx.NoBody(c) {
		return
	}
	httpx.Data(c, 200, []gin.H{
		{"code": "cafe", "name": "Кафе"}, {"code": "park", "name": "Парк"},
		{"code": "library", "name": "Библиотека"}, {"code": "coworking", "name": "Коворкинг"},
		{"code": "attraction", "name": "Достопримечательность"},
	})
}

func (h *Handler) Get(c *gin.Context) {
	if !httpx.CheckQuery(c) || !httpx.NoBody(c) {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	place, err := h.service.GetActive(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Data(c, 200, ToResponse(place, nil))
}

func (h *Handler) Create(c *gin.Context) {
	actor, ok := httpx.Actor(c)
	if !ok {
		return
	}
	if !httpx.CheckQuery(c) {
		return
	}
	var req createRequest
	if !httpx.Decode(c, &req) {
		return
	}
	if !require(c, "name", req.Name) || !require(c, "description", req.Description) || !require(c, "category", req.Category) ||
		!require(c, "address", req.Address) || !require(c, "location", req.Location) || !notNull(c, "tags", req.Tags) {
		return
	}
	location, ok := coordinates(c, req.Location.Value)
	if !ok {
		return
	}
	place, err := h.service.Create(c.Request.Context(), actor, application.CreateCommand{Name: req.Name.Value,
		Description: req.Description.Value, Address: req.Address.Value, Category: req.Category.Value,
		Tags: req.Tags.Value, Location: location})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Location", "/api/v1/places/"+string(place.ID))
	httpx.Data(c, 201, ToResponse(place, nil))
}

func (h *Handler) Update(c *gin.Context) {
	actor, ok := httpx.Actor(c)
	if !ok {
		return
	}
	if !httpx.CheckQuery(c) {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req updateRequest
	if !httpx.Decode(c, &req) {
		return
	}
	if !require(c, "expected_version", req.ExpectedVersion) || !notNull(c, "name", req.Name) || !notNull(c, "description", req.Description) ||
		!notNull(c, "category", req.Category) || !notNull(c, "tags", req.Tags) || !notNull(c, "address", req.Address) || !notNull(c, "location", req.Location) {
		return
	}
	cmd := application.UpdateCommand{ExpectedVersion: req.ExpectedVersion.Value}
	if req.Name.Present {
		cmd.Name = &req.Name.Value
	}
	if req.Description.Present {
		cmd.Description = &req.Description.Value
	}
	if req.Category.Present {
		cmd.Category = &req.Category.Value
	}
	if req.Tags.Present {
		cmd.Tags = &req.Tags.Value
	}
	if req.Address.Present {
		cmd.Address = &req.Address.Value
	}
	if req.Location.Present {
		location, ok := coordinates(c, req.Location.Value)
		if !ok {
			return
		}
		cmd.Location = &location
	}
	place, err := h.service.Update(c.Request.Context(), actor, id, cmd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Data(c, 200, ToResponse(place, nil))
}

func (h *Handler) Delete(c *gin.Context) {
	actor, ok := httpx.Actor(c)
	if !ok {
		return
	}
	if !httpx.CheckQuery(c, "expected_version") || !httpx.NoBody(c) {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	value, present := c.GetQuery("expected_version")
	if !present {
		httpx.Fail(c, app.Validation("expected_version", "required"))
		return
	}
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		httpx.Fail(c, app.E(400, "BAD_REQUEST", "Invalid expected_version"))
		return
	}
	if err := h.service.Delete(c.Request.Context(), actor, id, version); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(204)
}

func (h *Handler) Search(c *gin.Context) {
	if !httpx.CheckQuery(c, "q", "category", "tag", "longitude", "latitude", "radius_m", "limit", "offset") || !httpx.NoBody(c) {
		return
	}
	pagination, ok := httpx.ParsePagination(c)
	if !ok {
		return
	}
	query := application.SearchQuery{Pagination: pagination, Tags: c.QueryArray("tag")}
	values := c.Request.URL.Query()
	if values.Has("q") {
		query.NamePrefix = c.Query("q")
		if query.NamePrefix == "" {
			httpx.Fail(c, app.Validation("q", "invalid_length"))
			return
		}
	}
	if values.Has("category") {
		query.Category = domain.Category(c.Query("category"))
		if query.Category == "" {
			httpx.Fail(c, app.Validation("category", "invalid_category"))
			return
		}
	}
	geoCount := 0
	for _, key := range []string{"longitude", "latitude", "radius_m"} {
		if values.Has(key) {
			geoCount++
		}
	}
	if geoCount != 0 && geoCount != 3 {
		httpx.Fail(c, app.Validation("location", "all_geo_parameters_required"))
		return
	}
	if geoCount == 3 {
		longitude, err := strconv.ParseFloat(c.Query("longitude"), 64)
		if err != nil {
			httpx.Fail(c, app.E(400, "BAD_REQUEST", "Invalid longitude"))
			return
		}
		latitude, err := strconv.ParseFloat(c.Query("latitude"), 64)
		if err != nil {
			httpx.Fail(c, app.E(400, "BAD_REQUEST", "Invalid latitude"))
			return
		}
		radius, err := strconv.Atoi(c.Query("radius_m"))
		if err != nil {
			httpx.Fail(c, app.E(400, "BAD_REQUEST", "Invalid radius_m"))
			return
		}
		query.Geo = &application.GeoQuery{Center: domain.Coordinates{Longitude: longitude, Latitude: latitude}, RadiusM: radius}
	}
	page, err := h.service.Search(c.Request.Context(), query)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items := make([]PlaceResponse, 0, len(page.Items))
	for _, hit := range page.Items {
		items = append(items, ToResponse(hit.Place, hit.DistanceM))
	}
	httpx.List(c, items, pagination, page.HasMore)
}
