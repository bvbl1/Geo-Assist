package httpadapter

import (
	identity "geo-assist/internal/identity/application"
	"geo-assist/internal/identity/domain"
	web "geo-assist/internal/platform/httpserver"
	app "geo-assist/internal/shared/application"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const CookieName = "geoassist_session"

type Handler struct {
	service      *identity.Service
	cookieSecure bool
	limits       *limiter
}

func NewHandler(service *identity.Service, cookieSecure bool) *Handler {
	return &Handler{service: service, cookieSecure: cookieSecure, limits: &limiter{entries: map[string]bucket{}, now: time.Now}}
}
func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/auth/register", h.register)
	group.POST("/auth/login", h.login)
	group.POST("/auth/logout", h.logout)
	group.GET("/me", h.RequireAuth(), h.me)
	group.PATCH("/me", h.RequireAuth(), h.update)
}
func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie(CookieName)
		actor, err := h.service.Authenticate(c.Request.Context(), token)
		if err != nil {
			web.Fail(c, err)
			c.Abort()
			return
		}
		web.SetActor(c, actor)
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

type UserResponse struct {
	ID                  string    `json:"id"`
	Email               string    `json:"email"`
	DisplayName         string    `json:"display_name"`
	Role                string    `json:"role"`
	PreferredCategories []string  `json:"preferred_categories"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func response(u domain.User) UserResponse {
	categories := u.PreferredCategories
	if categories == nil {
		categories = []string{}
	}
	return UserResponse{ID: string(u.ID), Email: u.Email, DisplayName: u.DisplayName, Role: u.Role, PreferredCategories: categories, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}
func (h *Handler) register(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !web.CheckQuery(c) {
		return
	}
	if !h.allow(c, "register:ip:"+c.ClientIP(), 5, time.Hour) {
		return
	}
	var in struct {
		Email               string                 `json:"email"`
		Password            string                 `json:"password"`
		DisplayName         string                 `json:"display_name"`
		PreferredCategories web.Optional[[]string] `json:"preferred_categories"`
	}
	if !web.Decode(c, &in) {
		return
	}
	if in.PreferredCategories.Null {
		web.Fail(c, app.Validation("preferred_categories", "null_not_allowed"))
		return
	}
	u, err := h.service.Register(c.Request.Context(), identity.RegisterInput{Email: in.Email, Password: in.Password, DisplayName: in.DisplayName, PreferredCategories: in.PreferredCategories.Value})
	if err != nil {
		web.Fail(c, err)
		return
	}
	c.Header("Location", "/api/v1/me")
	web.Data(c, 201, response(u))
}
func (h *Handler) login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !web.CheckQuery(c) {
		return
	}
	if !h.allow(c, "login:ip:"+c.ClientIP(), 10, time.Minute) {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !web.Decode(c, &in) {
		return
	}
	if in.Email == "" || in.Password == "" {
		web.Fail(c, app.Validation("credentials", "email_and_password_required"))
		return
	}
	if !h.allow(c, "login:email:"+identity.NormalizeEmail(in.Email), 10, time.Minute) {
		return
	}
	old, _ := c.Cookie(CookieName)
	u, token, err := h.service.Login(c.Request.Context(), in.Email, in.Password, old)
	if err != nil {
		web.Fail(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(identity.SessionTTL.Seconds()), Expires: time.Now().UTC().Add(identity.SessionTTL)})
	web.Data(c, 200, response(u))
}
func (h *Handler) logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !web.CheckQuery(c) || !web.NoBody(c) {
		return
	}
	token, _ := c.Cookie(CookieName)
	if err := h.service.Logout(c.Request.Context(), token); err != nil {
		web.Fail(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
	c.Status(204)
}
func (h *Handler) me(c *gin.Context) {
	if !web.CheckQuery(c) || !web.NoBody(c) {
		return
	}
	actor, ok := web.Actor(c)
	if !ok {
		return
	}
	u, err := h.service.GetProfile(c.Request.Context(), actor)
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Data(c, 200, response(u))
}
func (h *Handler) update(c *gin.Context) {
	if !web.CheckQuery(c) {
		return
	}
	actor, ok := web.Actor(c)
	if !ok {
		return
	}
	var in struct {
		DisplayName         web.Optional[string]   `json:"display_name"`
		PreferredCategories web.Optional[[]string] `json:"preferred_categories"`
	}
	if !web.Decode(c, &in) {
		return
	}
	if in.DisplayName.Null {
		web.Fail(c, app.Validation("display_name", "null_not_allowed"))
		return
	}
	if in.PreferredCategories.Null {
		web.Fail(c, app.Validation("preferred_categories", "null_not_allowed"))
		return
	}
	input := identity.UpdateProfileInput{}
	if in.DisplayName.Present {
		input.DisplayName = &in.DisplayName.Value
	}
	if in.PreferredCategories.Present {
		input.PreferredCategories = &in.PreferredCategories.Value
	}
	u, err := h.service.UpdateProfile(c.Request.Context(), actor, input)
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Data(c, 200, response(u))
}
func (h *Handler) allow(c *gin.Context, key string, max int, window time.Duration) bool {
	wait := h.limits.take(key, max, window)
	if wait <= 0 {
		return true
	}
	c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	web.Fail(c, app.E(429, "RATE_LIMITED", "Too many requests"))
	return false
}

type bucket struct {
	count   int
	expires time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]bucket
	now     func() time.Time
}

func (l *limiter) take(key string, max int, window time.Duration) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	// Bound memory and evict expired entries when nearing capacity.
	if len(l.entries) >= 10000 {
		for k, b := range l.entries {
			if !b.expires.After(now) {
				delete(l.entries, k)
			}
		}
	}
	b, ok := l.entries[key]
	if !ok || !b.expires.After(now) {
		if !ok && len(l.entries) >= 10000 {
			return time.Minute
		}
		b = bucket{expires: now.Add(window)}
	}
	if b.count >= max {
		return b.expires.Sub(now)
	}
	b.count++
	l.entries[key] = b
	return 0
}
