package integration

import (
	"context"
	"encoding/json"
	"geo-assist/internal/bootstrap"
	"geo-assist/internal/platform/config"
	"geo-assist/internal/platform/mongostore"
	recmongo "geo-assist/internal/recommendation/adapters/mongo"
	rec "geo-assist/internal/recommendation/application"
	"geo-assist/internal/recommendation/domain"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http/httptest"
	"testing"
	"time"
)

func waitModel(t *testing.T, ctx context.Context, repo *recmongo.Repository, id string) *rec.Attempt {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		a, err := repo.Attempt(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if a != nil && a.Status != "building" {
			if a.Status == "failed" {
				t.Fatalf("build failed: %+v", a)
			}
			return a
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("model build timed out")
	return nil
}
func TestRecommendationMongoLifecycleAndHTTP(t *testing.T) {
	f := newAuthFixture(t)
	if _, err := mongostore.Seed(f.ctx, f.db); err != nil {
		t.Fatal(err)
	}
	repo := recmongo.New(f.db)
	cfg := rec.DefaultConfig()
	cfg.AutoRebuild = false
	cfg.PollInterval = time.Second
	service := rec.New(repo, cfg)
	if err := service.Start(f.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	f.server.Close()
	f.server = httptest.NewServer(bootstrap.Router(config.Config{Origin: "http://localhost:8080"}, f.db, service))
	t.Cleanup(f.server.Close)
	guest, user, admin := authClient(t), authClient(t), authClient(t)
	authExpect(t, f.call(t, guest, "GET", "/api/v1/me/recommendations", nil, nil), 401)
	authExpect(t, f.call(t, user, "POST", "/api/v1/auth/login", map[string]any{"email": "student01@geoassist.local", "password": "Student-demo-2026!"}, nil), 200)
	authExpect(t, f.call(t, admin, "POST", "/api/v1/auth/login", map[string]any{"email": "admin@geoassist.local", "password": "Admin-demo-2026!"}, nil), 200)
	authExpect(t, f.call(t, user, "GET", "/api/v1/me/recommendations", nil, nil), 503)
	authExpect(t, f.call(t, user, "POST", "/api/v1/admin/recommendations/rebuild", nil, nil), 403)
	authExpect(t, f.call(t, user, "GET", "/api/v1/admin/recommendations/status", nil, nil), 403)
	for _, query := range []string{"limit=0", "limit=51", "longitude=71", "category=unknown", "latitude=NaN&longitude=71&radius_m=5000", "tag=bad%20tag", "limit=1&limit=2", "user_id=abc"} {
		r := f.call(t, user, "GET", "/api/v1/me/recommendations?"+query, nil, nil)
		if r.status != 400 && r.status != 422 {
			t.Fatalf("invalid query %s: %d", query, r.status)
		}
	}
	r := f.call(t, admin, "POST", "/api/v1/admin/recommendations/rebuild", nil, nil)
	authExpect(t, r, 202)
	id := authData(t, r)["build_id"].(string)
	a := waitModel(t, f.ctx, repo, id)
	if a.Status != "ready" || a.ItemCount != 100 || a.PairCount == 0 {
		t.Fatalf("unexpected model %+v", a)
	}
	if err := service.Refresh(f.ctx); err != nil {
		t.Fatal(err)
	}
	r = f.call(t, user, "GET", "/api/v1/me/recommendations?limit=5&category=cafe&longitude=71.4304&latitude=51.1282&radius_m=50000", nil, nil)
	authExpect(t, r, 200)
	var result struct {
		Data []struct {
			Place struct {
				ID, Category string
				Distance     float64 `json:"distance_m"`
			}
			Score float64
		}
		Meta rec.ResultMeta
	}
	if err := json.Unmarshal(r.body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) == 0 || len(result.Data) > 5 {
		t.Fatalf("empty/oversized response %s", r.body)
	}
	for _, item := range result.Data {
		if item.Place.Category != "cafe" || item.Place.Distance > 50000 || item.Score <= 0 {
			t.Fatalf("bad filter: %s", r.body)
		}
	}
	authExpect(t, f.call(t, admin, "POST", "/api/v1/admin/recommendations/rebuild", nil, nil), 429)
	cold := f.call(t, admin, "GET", "/api/v1/me/recommendations", nil, nil)
	authExpect(t, cold, 200)
	var coldResult struct{ Meta rec.ResultMeta }
	_ = json.Unmarshal(cold.body, &coldResult)
	if !coldResult.Meta.Fallback {
		t.Fatal("admin without favorites should use popularity")
	}
	// A new service loads the same persisted version without rebuilding.
	restarted := rec.New(recmongo.New(f.db), cfg)
	if err := restarted.Start(f.ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	status, err := restarted.Status(f.ctx, "")
	if err != nil || status["loaded_model_id"] != id {
		t.Fatalf("restart did not load active version: %v %v", status, err)
	}
	skipped, err := service.RequestBuild("cli", false)
	if err != nil {
		t.Fatal(err)
	}
	if waitModel(t, f.ctx, repo, skipped.ID).Status != "skipped" {
		t.Fatal("unchanged source should skip")
	}
	forced, err := service.RequestBuild("cli", true)
	if err != nil {
		t.Fatal(err)
	}
	if waitModel(t, f.ctx, repo, forced.ID).Status != "ready" {
		t.Fatal("forced rebuild should publish")
	}
	if err = restarted.Refresh(f.ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = restarted.Status(f.ctx, "")
	if status["loaded_model_id"] != forced.ID {
		t.Fatal("external publication not loaded")
	}

	// Changing current favorites must rebuild without --force, and deleted places
	// must disappear immediately even while the previous model remains loaded.
	sourceBefore, err := repo.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	edge := sourceBefore.Edges[0]
	userID, _ := bson.ObjectIDFromHex(edge.User)
	placeID, _ := bson.ObjectIDFromHex(edge.Place)
	_, err = f.db.Collection("user_places").UpdateOne(f.ctx, bson.M{"user_id": userID, "place_id": placeID}, bson.M{"$set": bson.M{"favorite": false, "favorited_at": nil}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := service.RequestBuild("cli", false)
	if err != nil {
		t.Fatal(err)
	}
	changedModel := waitModel(t, f.ctx, repo, changed.ID)
	if changedModel.Status != "ready" || changedModel.Fingerprint == a.Fingerprint || changedModel.EdgeCount != a.EdgeCount-1 {
		t.Fatalf("favorite change was not rebuilt: %+v", changedModel)
	}
	forced = changed
	selectedID := result.Data[0].Place.ID
	selectedOID, _ := bson.ObjectIDFromHex(selectedID)
	_, err = f.db.Collection("places").UpdateOne(f.ctx, bson.M{"_id": selectedOID}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	latest := f.call(t, user, "GET", "/api/v1/me/recommendations?limit=50", nil, nil)
	authExpect(t, latest, 200)
	var liveResult struct {
		Data []struct{ Place struct{ ID string } }
	}
	if err = json.Unmarshal(latest.body, &liveResult); err != nil {
		t.Fatal(err)
	}
	for _, item := range liveResult.Data {
		if item.Place.ID == selectedID {
			t.Fatal("stale model recommended deleted place")
		}
	}
	// Lease fencing: a previous owner cannot publish after a replacement acquired the lease.
	old := rec.Attempt{ID: string(shared.NewID()), SchemaVersion: 1, Algorithm: domain.Algorithm, Status: "building", Trigger: "cli", StartedAt: time.Now()}
	if err = repo.Acquire(f.ctx, old); err != nil {
		t.Fatal(err)
	}
	other := old
	other.ID = string(shared.NewID())
	if err = repo.Acquire(f.ctx, other); err == nil {
		t.Fatal("concurrent lease accepted")
	}
	_, err = f.db.Collection("app_meta").UpdateOne(f.ctx, bson.M{"_id": "recommendations"}, bson.M{"$set": bson.M{"lease.expires_at": time.Unix(0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Acquire(f.ctx, other); err != nil {
		t.Fatal(err)
	}
	old.Status = "ready"
	if err = repo.Finish(f.ctx, old, true); err == nil {
		t.Fatal("stale owner published")
	}
	other.Status = "failed"
	if err = repo.Fail(f.ctx, other); err != nil {
		t.Fatal(err)
	}
	meta, _ := repo.Meta(f.ctx)
	if meta.NextAttempt == nil || !meta.NextAttempt.After(time.Now()) {
		t.Fatal("failed build did not persist retry deadline")
	}
	scheduled := other
	scheduled.ID = string(shared.NewID())
	scheduled.Status = "building"
	scheduled.Trigger = "scheduled"
	if err = repo.Acquire(f.ctx, scheduled); err == nil {
		t.Fatal("scheduled build bypassed retry backoff")
	}
	if meta.ActiveID != forced.ID {
		t.Fatal("failed build displaced active model")
	}
}
