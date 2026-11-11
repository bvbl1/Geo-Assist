package httpadapter

import (
	"strings"
	"sync"
	"time"

	"geo-assist/internal/activity/application"
	"geo-assist/internal/activity/domain"
	cataloghttp "geo-assist/internal/catalog/adapters/http"
	"geo-assist/internal/platform/httpserver"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *application.Service
	mu      sync.Mutex
	windows map[shared.ID]rateWindow
}
type rateWindow struct {
	Start time.Time
	Count int
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service, windows: map[shared.ID]rateWindow{}}
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup, auth gin.HandlerFunc) {
	r := group.Group("")
	r.Use(auth, func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	r.GET("/me/places/:id/state", h.state)
	r.PUT("/me/favorites/:id", h.favorite(true))
	r.DELETE("/me/favorites/:id", h.favorite(false))
	r.GET("/me/favorites", h.favorites)
	r.PUT("/me/ratings/:id", h.rating)
	r.DELETE("/me/ratings/:id", h.removeRating)
	r.POST("/interactions", h.record)
	r.GET("/me/interactions", h.history)
}

type StateResponse struct {
	PlaceID     shared.ID  `json:"place_id"`
	Favorite    bool       `json:"favorite"`
	FavoritedAt *time.Time `json:"favorited_at"`
	Rating      *int       `json:"rating"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func stateResponse(s domain.UserPlace) StateResponse {
	return StateResponse{PlaceID: s.PlaceID, Favorite: s.Favorite, FavoritedAt: s.FavoritedAt, Rating: s.Rating, UpdatedAt: s.UpdatedAt}
}

type InteractionResponse struct {
	ID             shared.ID `json:"id"`
	EventKey       string    `json:"event_key"`
	PlaceID        shared.ID `json:"place_id"`
	Type           string    `json:"type"`
	PreviousRating *int      `json:"previous_rating"`
	Rating         *int      `json:"rating"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func eventResponse(e domain.Interaction) InteractionResponse {
	return InteractionResponse{ID: e.ID, EventKey: e.EventKey, PlaceID: e.PlaceID, Type: e.Type, PreviousRating: e.PreviousRating, Rating: e.Rating, OccurredAt: e.OccurredAt}
}

func actor(c *gin.Context) (app.Actor, bool) {
	a, ok := httpserver.Actor(c)
	if !ok {
		httpserver.Fail(c, app.E(401, "UNAUTHENTICATED", "Authentication required"))
	}
	return a, ok
}
func pathInput(c *gin.Context) (app.Actor, shared.ID, bool) {
	a, ok := actor(c)
	if !ok {
		return a, "", false
	}
	if !httpserver.CheckQuery(c) {
		return a, "", false
	}
	id, ok := httpserver.ValidateID(c, c.Param("id"))
	return a, id, ok
}
func (h *Handler) state(c *gin.Context) {
	a, id, ok := pathInput(c)
	if !ok || !httpserver.NoBody(c) {
		return
	}
	s, err := h.service.State(c.Request.Context(), a, id)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.Data(c, 200, stateResponse(s))
}
func (h *Handler) favorite(value bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, id, ok := pathInput(c)
		if !ok || !httpserver.NoBody(c) {
			return
		}
		s, err := h.service.SetFavorite(c.Request.Context(), a, id, value)
		if err != nil {
			httpserver.Fail(c, err)
			return
		}
		httpserver.Data(c, 200, stateResponse(s))
	}
}
func (h *Handler) rating(c *gin.Context) {
	a, id, ok := pathInput(c)
	if !ok {
		return
	}
	var dto struct {
		Rating httpserver.Optional[int] `json:"rating"`
	}
	if !httpserver.Decode(c, &dto) {
		return
	}
	if !dto.Rating.Present || dto.Rating.Null {
		httpserver.Fail(c, app.Validation("rating", "required"))
		return
	}
	s, err := h.service.SetRating(c.Request.Context(), a, id, &dto.Rating.Value)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.Data(c, 200, stateResponse(s))
}
func (h *Handler) removeRating(c *gin.Context) {
	a, id, ok := pathInput(c)
	if !ok || !httpserver.NoBody(c) {
		return
	}
	s, err := h.service.SetRating(c.Request.Context(), a, id, nil)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.Data(c, 200, stateResponse(s))
}
func (h *Handler) favorites(c *gin.Context) {
	a, ok := actor(c)
	if !ok || !httpserver.CheckQuery(c, "limit", "offset") || !httpserver.NoBody(c) {
		return
	}
	p, ok := httpserver.ParsePagination(c)
	if !ok {
		return
	}
	result, err := h.service.Favorites(c.Request.Context(), a, p)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	type favoriteResponse struct {
		Place       cataloghttp.PlaceResponse `json:"place"`
		FavoritedAt time.Time                 `json:"favorited_at"`
		Rating      *int                      `json:"rating"`
	}
	items := make([]favoriteResponse, 0, len(result.Items))
	for _, v := range result.Items {
		items = append(items, favoriteResponse{Place: cataloghttp.ToResponse(v.Place, nil), FavoritedAt: v.FavoritedAt, Rating: v.Rating})
	}
	httpserver.List(c, items, p, result.HasMore)
}
func (h *Handler) allowEvent(user shared.ID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for k, v := range h.windows {
		if now.Sub(v.Start) >= time.Minute {
			delete(h.windows, k)
		}
	}
	w := h.windows[user]
	if w.Start.IsZero() {
		w.Start = now
	}
	if w.Count >= 60 {
		return false
	}
	w.Count++
	h.windows[user] = w
	return true
}
func (h *Handler) record(c *gin.Context) {
	a, ok := actor(c)
	if !ok || !httpserver.CheckQuery(c) {
		return
	}
	if !h.allowEvent(a.UserID) {
		c.Header("Retry-After", "60")
		httpserver.Fail(c, app.E(429, "RATE_LIMITED", "Too many interactions"))
		return
	}
	var dto struct {
		EventKey string `json:"event_key"`
		PlaceID  string `json:"place_id"`
		Type     string `json:"type"`
	}
	if !httpserver.Decode(c, &dto) {
		return
	}
	id, ok := httpserver.ValidateID(c, dto.PlaceID)
	if !ok {
		return
	}
	e, replay, err := h.service.Record(c.Request.Context(), a, application.RecordInput{PlaceID: id, EventKey: dto.EventKey, Type: dto.Type})
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	status := 201
	if replay {
		status = 200
	}
	httpserver.Data(c, status, eventResponse(e))
}
func (h *Handler) history(c *gin.Context) {
	a, ok := actor(c)
	if !ok || !httpserver.CheckQuery(c, "type", "place_id", "limit", "offset") || !httpserver.NoBody(c) {
		return
	}
	p, ok := httpserver.ParsePagination(c)
	if !ok {
		return
	}
	q := application.HistoryQuery{Pagination: p, Type: c.Query("type")}
	if _, exists := c.Request.URL.Query()["type"]; exists && q.Type == "" {
		httpserver.Fail(c, app.Validation("type", "invalid_choice"))
		return
	}
	if v, exists := c.Request.URL.Query()["place_id"]; exists {
		id, valid := httpserver.ValidateID(c, strings.Join(v, ""))
		if !valid {
			return
		}
		q.PlaceID = id
	}
	result, err := h.service.History(c.Request.Context(), a, q)
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	items := make([]InteractionResponse, 0, len(result.Items))
	for _, v := range result.Items {
		items = append(items, eventResponse(v))
	}
	httpserver.List(c, items, p, result.HasMore)
}
