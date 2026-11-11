package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	catalogmongo "geo-assist/internal/catalog/adapters/mongo"
	catalog "geo-assist/internal/catalog/application"
	catalogdomain "geo-assist/internal/catalog/domain"
	"geo-assist/internal/platform/mongostore"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type catalogFixture struct {
	ctx     context.Context
	db      *mongo.Database
	repo    *catalogmongo.Repository
	service *catalog.Service
	admin   app.Actor
}

func newCatalogFixture(t *testing.T) *catalogFixture {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI is unset; requires a real MongoDB replica set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	client, err := mongostore.Connect(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	name := "geoassist_test_catalog_" + bson.NewObjectID().Hex()
	db := client.Database(name)
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if !strings.HasPrefix(name, "geoassist_test_catalog_") {
			t.Error("refusing to clean non-test database")
			return
		}
		if err := db.Drop(cleanupCtx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if err := client.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect: %v", err)
		}
	})
	if err := mongostore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := catalogmongo.NewRepository(db)
	return &catalogFixture{ctx: ctx, db: db, repo: repo, service: catalog.NewService(repo), admin: app.Actor{UserID: shared.NewID(), Role: "admin"}}
}

func (f *catalogFixture) place(t *testing.T, name string, category catalogdomain.Category, tags []string, longitude, latitude float64) catalogdomain.Place {
	t.Helper()
	p, err := f.service.Create(f.ctx, f.admin, catalog.CreateCommand{Name: name, Description: "Synthetic integration place", Address: "Astana test address", Category: category, Tags: tags, Location: catalogdomain.Coordinates{Longitude: longitude, Latitude: latitude}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCatalogGeoPrefixAndCombinedFilters(t *testing.T) {
	f := newCatalogFixture(t)
	near := f.place(t, "Cafe.* Literal", catalogdomain.Cafe, []string{"wifi", "quiet"}, 71.4304, 51.1282)
	f.place(t, "CafeZZ", catalogdomain.Cafe, []string{"wifi"}, 71.4314, 51.1282)
	f.place(t, "Cafe.* Park", catalogdomain.Park, []string{"wifi", "quiet"}, 71.4304, 51.1292)
	f.place(t, "Cafe.* Far", catalogdomain.Cafe, []string{"wifi", "quiet"}, 71.4304, 51.2282)
	query := catalog.SearchQuery{NamePrefix: "CAFE.*", Category: catalogdomain.Cafe, Tags: []string{"quiet", "wifi"}, Geo: &catalog.GeoQuery{Center: catalogdomain.Coordinates{Longitude: 71.4304, Latitude: 51.1282}, RadiusM: 500}, Pagination: app.Pagination{Limit: 20}}
	page, err := f.service.Search(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Place.ID != near.ID {
		t.Fatalf("AND/radius/literal-prefix filter failed: %+v", page.Items)
	}
	if page.Items[0].DistanceM == nil || *page.Items[0].DistanceM > 1 {
		t.Fatalf("center distance invalid: %+v", page.Items[0])
	}
	query.NamePrefix = ""
	query.Category = ""
	query.Tags = nil
	page, err = f.service.Search(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected three inside and one outside radius; got %d", len(page.Items))
	}
	for i := 1; i < len(page.Items); i++ {
		if *page.Items[i].DistanceM < *page.Items[i-1].DistanceM {
			t.Fatal("distance order is not ascending")
		}
	}
	query.Geo = nil
	query.NamePrefix = "cafe.*"
	query.Pagination.Limit = 2
	page, err = f.service.Search(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("expected limit+1 pagination: %+v", page)
	}
	for _, hit := range page.Items {
		if hit.DistanceM != nil {
			t.Fatal("non-geo search must have null distance")
		}
	}
}

func TestCatalogConcurrentVersionAndSoftDelete(t *testing.T) {
	f := newCatalogFixture(t)
	p := f.place(t, "Version test", catalogdomain.Cafe, nil, 71.4304, 51.1282)
	// Bypass the preliminary service read to exercise atomic compare-and-set directly.
	first, second := p, p
	first.Name = "First update"
	second.Name = "Second update"
	first.Version = 2
	second.Version = 2
	var wg sync.WaitGroup
	errorsOut := make(chan error, 2)
	for _, candidate := range []catalogdomain.Place{first, second} {
		wg.Add(1)
		go func(next catalogdomain.Place) { defer wg.Done(); errorsOut <- f.repo.Update(f.ctx, next, 1) }(candidate)
	}
	wg.Wait()
	close(errorsOut)
	success, conflicts := 0, 0
	for err := range errorsOut {
		if err == nil {
			success++
			continue
		}
		var apiErr *app.Error
		if errors.As(err, &apiErr) && apiErr.Code == "VERSION_CONFLICT" {
			conflicts++
		} else {
			t.Fatalf("unexpected update error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("wanted one success/one conflict, got %d/%d", success, conflicts)
	}
	if err := f.service.Delete(f.ctx, f.admin, p.ID, 1); err == nil {
		t.Fatal("stale delete should fail")
	}
	if err := f.service.Delete(f.ctx, f.admin, p.ID, 2); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.GetActive(f.ctx, p.ID)
	var apiErr *app.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "NOT_FOUND" {
		t.Fatalf("deleted place visible: %v", err)
	}
	stored, err := f.service.GetAny(f.ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DeletedAt == nil || stored.Version != 3 {
		t.Fatalf("soft delete failed: %+v", stored)
	}
	page, err := f.service.Search(f.ctx, catalog.SearchQuery{Pagination: app.Pagination{Limit: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatal("deleted place remained in catalog")
	}
}

func TestCatalogMongoValidatorsRejectInvalidDocuments(t *testing.T) {
	f := newCatalogFixture(t)
	p := f.place(t, "Validator source", catalogdomain.Cafe, []string{"wifi"}, 71.4304, 51.1282)
	doc, err := catalogmongo.ToDocument(p)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(bson.M){
		"missing_required":   func(d bson.M) { delete(d, "name") },
		"unknown_field":      func(d bson.M) { d["unexpected"] = true },
		"unknown_category":   func(d bson.M) { d["category"] = "restaurant" },
		"wrong_version_type": func(d bson.M) { d["version"] = int32(1) },
		"duplicate_tags":     func(d bson.M) { d["tags"] = []string{"wifi", "wifi"} },
	} {
		t.Run(name, func(t *testing.T) {
			var candidate bson.M
			if err := bson.Unmarshal(encoded, &candidate); err != nil {
				t.Fatal(err)
			}
			candidate["_id"] = bson.NewObjectID()
			mutate(candidate)
			_, err := f.db.Collection("places").InsertOne(f.ctx, candidate)
			var server mongo.ServerError
			if !errors.As(err, &server) || !server.HasErrorCode(121) {
				t.Fatalf("expected MongoDB document validation code 121, got %v", err)
			}
		})
	}
	if err := mongostore.Migrate(f.ctx, f.db); err != nil {
		t.Fatalf("repeat migration failed: %v", err)
	}
}
