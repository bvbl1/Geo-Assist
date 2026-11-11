package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	activitymongo "geo-assist/internal/activity/adapters/mongo"
	activity "geo-assist/internal/activity/application"
	activitydomain "geo-assist/internal/activity/domain"
	catalogmongo "geo-assist/internal/catalog/adapters/mongo"
	catalog "geo-assist/internal/catalog/application"
	catalogdomain "geo-assist/internal/catalog/domain"
	"geo-assist/internal/platform/mongostore"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type activityFixture struct {
	ctx          context.Context
	db           *mongo.Database
	repo         *activitymongo.Repository
	service      *activity.Service
	catalog      *catalog.Service
	actor, admin app.Actor
}

func newActivityFixture(t *testing.T) *activityFixture {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI is unset; requires a real MongoDB replica set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	name := "geoassist_test_activity_" + bson.NewObjectID().Hex()
	db := client.Database(name)
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if !strings.HasPrefix(name, "geoassist_test_activity_") {
			t.Error("refusing to clean non-test database")
			return
		}
		if err := db.Drop(cleanupCtx); err != nil {
			t.Errorf("cleanup test database: %v", err)
		}
		if err := client.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect: %v", err)
		}
	})
	if err = client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err = mongostore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := activitymongo.NewRepository(db)
	places := catalog.NewService(catalogmongo.NewRepository(db))
	return &activityFixture{ctx: ctx, db: db, repo: repo, service: activity.NewService(repo, places), catalog: places, actor: app.Actor{UserID: shared.NewID(), Role: "user"}, admin: app.Actor{UserID: shared.NewID(), Role: "admin"}}
}

func (f *activityFixture) place(t *testing.T, name string) catalogdomain.Place {
	t.Helper()
	p, err := f.catalog.Create(f.ctx, f.admin, catalog.CreateCommand{Name: name, Description: "Integration test place", Address: "Synthetic address in Astana", Category: "cafe", Tags: []string{"wifi"}, Location: catalogdomain.Coordinates{Longitude: 71.4304, Latitude: 51.1282}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type failEventRepository struct{ activity.Repository }

func (r failEventRepository) WithinTransaction(ctx context.Context, fn func(activity.Transaction) error) error {
	return r.Repository.WithinTransaction(ctx, func(tx activity.Transaction) error { return fn(failEventTransaction{tx}) })
}

type failEventTransaction struct{ activity.Transaction }

func (failEventTransaction) AppendEvent(context.Context, activitydomain.Interaction) error {
	return errors.New("injected failure after state save")
}

func TestActivityTransactionRollback(t *testing.T) {
	f := newActivityFixture(t)
	p := f.place(t, "Rollback")
	broken := activity.NewService(failEventRepository{f.repo}, f.catalog)
	if _, err := broken.SetFavorite(f.ctx, f.actor, p.ID, true); err == nil {
		t.Fatal("expected injected write failure")
	}
	count, err := f.db.Collection("user_places").CountDocuments(f.ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("state persisted after rollback: %d", count)
	}
	count, err = f.db.Collection("interactions").CountDocuments(f.ctx, bson.M{})
	if err != nil || count != 0 {
		t.Fatalf("events after rollback: %d, %v", count, err)
	}
}

func TestActivityConcurrentFavoriteAndRatingNoop(t *testing.T) {
	f := newActivityFixture(t)
	p := f.place(t, "Concurrent")
	const workers = 12
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.service.SetFavorite(f.ctx, f.actor, p.ID, true); errorsCh <- err }()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Error(err)
		}
	}
	state, err := f.service.State(f.ctx, f.actor, p.ID)
	if err != nil || !state.Favorite {
		t.Fatalf("state: %+v %v", state, err)
	}
	states, err := f.db.Collection("user_places").CountDocuments(f.ctx, bson.M{})
	if err != nil || states != 1 {
		t.Fatalf("duplicate states: %d %v", states, err)
	}
	events, err := f.db.Collection("interactions").CountDocuments(f.ctx, bson.M{"type": "favorite_added"})
	if err != nil || events != 1 {
		t.Fatalf("duplicate events: %d %v", events, err)
	}
	rating := 5
	for i := 0; i < 3; i++ {
		if _, err := f.service.SetRating(f.ctx, f.actor, p.ID, &rating); err != nil {
			t.Fatal(err)
		}
	}
	events, err = f.db.Collection("interactions").CountDocuments(f.ctx, bson.M{"type": "rating_set"})
	if err != nil || events != 1 {
		t.Fatalf("rating no-op created events: %d %v", events, err)
	}
}

func TestActivityReplayConflictAndDeletedPlace(t *testing.T) {
	f := newActivityFixture(t)
	p := f.place(t, "Replay")
	input := activity.RecordInput{PlaceID: p.ID, EventKey: activitydomain.NewEventKey(), Type: activitydomain.View}
	first, replay, err := f.service.Record(f.ctx, f.actor, input)
	if err != nil || replay {
		t.Fatalf("first: %v %v", replay, err)
	}
	if err = f.catalog.Delete(f.ctx, f.admin, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	repeated, replay, err := f.service.Record(f.ctx, f.actor, input)
	if err != nil || !replay || repeated.ID != first.ID || !repeated.OccurredAt.Equal(first.OccurredAt) {
		t.Fatalf("replay after deletion: %v %v", replay, err)
	}
	input.Type = activitydomain.Select
	_, _, err = f.service.Record(f.ctx, f.actor, input)
	var ae *app.Error
	if !errors.As(err, &ae) || ae.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected conflict, got %v", err)
	}
	count, err := f.db.Collection("interactions").CountDocuments(f.ctx, bson.M{})
	if err != nil || count != 1 {
		t.Fatalf("event count: %d %v", count, err)
	}
}

func TestActivityFavoritesFilterDeletedBeforePagination(t *testing.T) {
	f := newActivityFixture(t)
	first := f.place(t, "First active")
	second := f.place(t, "Second active")
	deleted := f.place(t, "Most recent deleted")
	for _, p := range []catalogdomain.Place{first, second, deleted} {
		if _, err := f.service.SetFavorite(f.ctx, f.actor, p.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	// Explicit timestamps make the pagination check independent of clock precision.
	for i, p := range []catalogdomain.Place{first, second, deleted} {
		pid, _ := bson.ObjectIDFromHex(string(p.ID))
		_, err := f.db.Collection("user_places").UpdateOne(f.ctx, bson.M{"place_id": pid}, bson.M{"$set": bson.M{"favorited_at": time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.catalog.Delete(f.ctx, f.admin, deleted.ID, deleted.Version); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.Favorites(f.ctx, f.actor, app.Pagination{Limit: 1, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Place.ID != second.ID || !page.HasMore {
		t.Fatalf("first page incorrectly filtered: %+v", page)
	}
	page, err = f.service.Favorites(f.ctx, f.actor, app.Pagination{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Place.ID != first.ID || page.HasMore {
		t.Fatalf("second page incorrect: %+v", page)
	}
	if _, err = f.service.SetFavorite(f.ctx, f.actor, deleted.ID, false); err != nil {
		t.Fatal("cannot undo deleted-place preference", err)
	}
}

func TestActivityNoOpPreservesResponseTimestamps(t *testing.T) {
	f := newActivityFixture(t)
	p := f.place(t, "Timestamp precision")
	first, err := f.service.SetFavorite(f.ctx, f.actor, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := f.service.SetFavorite(f.ctx, f.actor, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.UpdatedAt.Equal(*repeated.UpdatedAt) || !first.FavoritedAt.Equal(*repeated.FavoritedAt) {
		t.Fatalf("no-op changed response timestamps: first=%+v repeated=%+v", first, repeated)
	}
}
