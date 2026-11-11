package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"geo-assist/internal/bootstrap"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	"geo-assist/internal/platform/security"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type authFixture struct {
	ctx    context.Context
	db     *mongo.Database
	server *httptest.Server
}
type authHTTPResult struct {
	status  int
	header  http.Header
	body    []byte
	cookies []*http.Cookie
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI is unset; requires a real MongoDB replica set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	name := "geoassist_test_auth_" + bson.NewObjectID().Hex()
	db := client.Database(name)
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if !strings.HasPrefix(name, "geoassist_test_auth_") {
			t.Error("refusing to drop non-test database")
			return
		}
		if err := db.Drop(clean); err != nil {
			t.Errorf("drop auth fixture database: %v", err)
		}
		if err := client.Disconnect(clean); err != nil {
			t.Errorf("disconnect auth fixture: %v", err)
		}
	})
	if err = client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err = mongostore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(bootstrap.Router(config.Config{Origin: "http://localhost:8080", CookieSecure: false}, db))
	t.Cleanup(server.Close)
	return &authFixture{ctx: ctx, db: db, server: server}
}
func authClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 15 * time.Second}
}
func (f *authFixture) call(t *testing.T, client *http.Client, method, path string, body any, headers map[string]string) authHTTPResult {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	request, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		request.Header.Set("Origin", "http://localhost:8080")
		request.Header.Set("X-GeoAssist-CSRF", "1")
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	b, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"password_hash", "token_hash", "Auth-test-password-2026!"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatalf("credential data exposed by %s %s", method, path)
		}
	}
	if response.Header.Get("X-Request-ID") == "" {
		t.Errorf("request ID missing on %s %s", method, path)
	}
	return authHTTPResult{status: response.StatusCode, header: response.Header, body: b, cookies: response.Cookies()}
}
func authExpect(t *testing.T, r authHTTPResult, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("expected HTTP %d, got %d: %s", status, r.status, r.body)
	}
}
func authData(t *testing.T, r authHTTPResult) map[string]any {
	t.Helper()
	var v struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatal(err)
	}
	return v.Data
}
func authCookie(t *testing.T, client *http.Client, server string) *http.Cookie {
	t.Helper()
	u, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "geoassist_session" {
			return c
		}
	}
	t.Fatal("session cookie was not stored")
	return nil
}
func TestAuthLifecycleAgainstMongoAndRouter(t *testing.T) {
	f := newAuthFixture(t)
	browser := authClient(t)
	otherDevice := authClient(t)
	credentials := map[string]any{"email": "auth@example.com", "password": "Auth-test-password-2026!"}
	signup := map[string]any{"email": " AUTH@example.com ", "password": "Auth-test-password-2026!", "display_name": " Auth Student ", "preferred_categories": []string{"cafe"}}
	r := f.call(t, browser, "POST", "/api/v1/auth/register", signup, nil)
	authExpect(t, r, 201)
	user := authData(t, r)
	if user["email"] != "auth@example.com" || user["display_name"] != "Auth Student" || user["role"] != "user" {
		t.Fatalf("incorrect registration response: %v", user)
	}
	if len(r.cookies) != 0 {
		t.Fatal("register unexpectedly authenticated user")
	}
	authExpect(t, f.call(t, browser, "GET", "/api/v1/me", nil, nil), 401)
	authExpect(t, f.call(t, browser, "POST", "/api/v1/auth/register", signup, nil), 409)
	// The same generic error must cover unknown email and wrong password.
	wrong := f.call(t, browser, "POST", "/api/v1/auth/login", map[string]any{"email": "auth@example.com", "password": "incorrect-password"}, nil)
	missing := f.call(t, browser, "POST", "/api/v1/auth/login", map[string]any{"email": "missing@example.com", "password": "incorrect-password"}, nil)
	authExpect(t, wrong, 401)
	authExpect(t, missing, 401)
	for _, result := range []authHTTPResult{wrong, missing} {
		if !bytes.Contains(result.body, []byte("INVALID_CREDENTIALS")) {
			t.Fatalf("wrong credentials response: %s", result.body)
		}
	}
	r = f.call(t, browser, "POST", "/api/v1/auth/login", credentials, nil)
	authExpect(t, r, 200)
	if r.header.Get("Cache-Control") != "no-store" {
		t.Fatal("login response permits caching")
	}
	var sessionCookie *http.Cookie
	for _, c := range r.cookies {
		if c.Name == "geoassist_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.Path != "/" || sessionCookie.Domain != "" || sessionCookie.MaxAge != 604800 {
		t.Fatalf("incorrect session cookie: %+v", sessionCookie)
	}
	firstCookie := *authCookie(t, browser, f.server.URL)
	authExpect(t, f.call(t, browser, "GET", "/api/v1/me", nil, nil), 200)
	r = f.call(t, browser, "PATCH", "/api/v1/me", map[string]any{"display_name": "Updated Name"}, nil)
	authExpect(t, r, 200)
	profile := authData(t, r)
	if profile["display_name"] != "Updated Name" || profile["email"] != "auth@example.com" || len(profile["preferred_categories"].([]any)) != 1 {
		t.Fatalf("partial profile update lost fields: %v", profile)
	}
	authExpect(t, f.call(t, browser, "PATCH", "/api/v1/me", map[string]any{"display_name": nil}, nil), 422)
	authExpect(t, f.call(t, browser, "PATCH", "/api/v1/me", map[string]any{"role": "admin"}, nil), 400)
	authExpect(t, f.call(t, browser, "PATCH", "/api/v1/me", map[string]any{"preferred_categories": []string{}}, nil), 200)
	// Public authentication endpoints and private mutations both enforce CSRF.
	authExpect(t, f.call(t, browser, "POST", "/api/v1/auth/login", credentials, map[string]string{"Origin": "https://attacker.example"}), 403)
	authExpect(t, f.call(t, browser, "PATCH", "/api/v1/me", map[string]any{"display_name": "CSRF overwrite"}, map[string]string{"X-GeoAssist-CSRF": ""}), 403)
	place := map[string]any{"name": "Unauthorized place", "description": "Must not be created", "category": "cafe", "tags": []string{}, "address": "Synthetic address", "location": map[string]float64{"longitude": 71.4304, "latitude": 51.1282}}
	authExpect(t, f.call(t, browser, "POST", "/api/v1/places", place, nil), 403)
	// A second device is independent of rotation in the first browser.
	authExpect(t, f.call(t, otherDevice, "POST", "/api/v1/auth/login", credentials, nil), 200)
	secondDeviceCookie := *authCookie(t, otherDevice, f.server.URL)
	authExpect(t, f.call(t, browser, "POST", "/api/v1/auth/login", credentials, nil), 200)
	rotatedCookie := authCookie(t, browser, f.server.URL)
	if rotatedCookie.Value == firstCookie.Value {
		t.Fatal("login reused the old session token")
	}
	rawClient := &http.Client{Timeout: 15 * time.Second}
	authExpect(t, f.call(t, rawClient, "GET", "/api/v1/me", nil, map[string]string{"Cookie": firstCookie.String()}), 401)
	authExpect(t, f.call(t, otherDevice, "GET", "/api/v1/me", nil, nil), 200)
	// Force expiry without waiting for TTL cleanup; application must reject it.
	digest := security.CryptoTokens{}.Hash(rotatedCookie.Value)
	update, err := f.db.Collection("sessions").UpdateOne(f.ctx, bson.M{"token_hash": digest}, bson.M{"$set": bson.M{"expires_at": time.Now().UTC().Add(-time.Second)}})
	if err != nil || update.MatchedCount != 1 {
		t.Fatalf("expire session: %v, result: %+v", err, update)
	}
	authExpect(t, f.call(t, browser, "GET", "/api/v1/me", nil, nil), 401)
	// Logout revokes the remaining device token, even if manually replayed.
	r = f.call(t, otherDevice, "POST", "/api/v1/auth/logout", nil, nil)
	authExpect(t, r, 204)
	authExpect(t, f.call(t, otherDevice, "GET", "/api/v1/me", nil, nil), 401)
	authExpect(t, f.call(t, rawClient, "GET", "/api/v1/me", nil, map[string]string{"Cookie": secondDeviceCookie.String()}), 401)
	authExpect(t, f.call(t, otherDevice, "POST", "/api/v1/auth/logout", nil, nil), 204)
	count, err := f.db.Collection("places").CountDocuments(f.ctx, bson.M{})
	if err != nil || count != 0 {
		t.Fatalf("ordinary user created catalog data: count=%d err=%v", count, err)
	}
	var stored bson.M
	if err = f.db.Collection("users").FindOne(f.ctx, bson.M{"email": "auth@example.com"}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	hash, ok := stored["password_hash"].(string)
	if !ok || !strings.HasPrefix(hash, "$2") || hash == credentials["password"] {
		t.Fatal("password was not stored as a bcrypt hash")
	}
}
