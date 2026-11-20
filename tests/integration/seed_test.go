package integration

import (
	"context"
	"testing"
	"time"

	"geo-assist/internal/platform/mongostore"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSeedOncePreservesChanges(t *testing.T) {
	f := newActivityFixture(t)
	created, err := mongostore.Seed(f.ctx, f.db)
	if err != nil || !created {
		t.Fatalf("first seed created=%v err=%v", created, err)
	}
	for name, want := range map[string]int64{"users": 51, "places": 100, "user_places": 400, "interactions": 1200, "app_meta": 1} {
		n, e := f.db.Collection(name).CountDocuments(f.ctx, bson.M{})
		if e != nil || n != want {
			t.Fatalf("%s count=%d want=%d err=%v", name, n, want, e)
		}
	}
	placeID := mongostore.SeedID("place", 0)
	if _, err = f.db.Collection("places").UpdateOne(f.ctx, bson.M{"_id": placeID}, bson.M{"$set": bson.M{"name": "User edited place", "name_search": "user edited place"}, "$inc": bson.M{"version": int64(1)}}); err != nil {
		t.Fatal(err)
	}
	if err = mongostore.Migrate(f.ctx, f.db); err != nil {
		t.Fatal(err)
	}
	created, err = mongostore.Seed(f.ctx, f.db)
	if err != nil || created {
		t.Fatalf("repeat seed created=%v err=%v", created, err)
	}
	var p struct {
		Name    string `bson:"name"`
		Version int64  `bson:"version"`
	}
	if err = f.db.Collection("places").FindOne(f.ctx, bson.M{"_id": placeID}).Decode(&p); err != nil || p.Name != "User edited place" || p.Version != 2 {
		t.Fatalf("seed overwrote changes: %+v %v", p, err)
	}
}

func TestSeedRollsBackEntireImport(t *testing.T) {
	f := newActivityFixture(t)
	// Force the final batch to fail after users, places and states were inserted.
	if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "interactions"}, {Key: "validator", Value: bson.M{"type": "never_allowed"}}, {Key: "validationLevel", Value: "strict"}, {Key: "validationAction", Value: "error"}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := mongostore.Seed(f.ctx, f.db); err == nil {
		t.Fatal("expected validator rejection")
	}
	for _, name := range []string{"users", "places", "user_places", "interactions", "app_meta"} {
		n, err := f.db.Collection(name).CountDocuments(f.ctx, bson.M{})
		if err != nil || n != 0 {
			t.Fatalf("partial seed %s=%d err=%v", name, n, err)
		}
	}
	// A corrected startup can retry successfully after the interrupted import.
	if err := mongostore.Migrate(f.ctx, f.db); err != nil {
		t.Fatal(err)
	}
	retry, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	if created, err := mongostore.Seed(retry, f.db); err != nil || !created {
		t.Fatalf("retry created=%v err=%v", created, err)
	}
}

func TestSeedUpgradesLegacyLocationsWithoutChangingUserData(t *testing.T) {
	f := newActivityFixture(t)
	if _, err := mongostore.Seed(f.ctx, f.db); err != nil {
		t.Fatal(err)
	}
	// Reconstruct an old installation, with one edited and one deleted place.
	for i := 0; i < 100; i++ {
		old := []float64{71.4304 + float64(i%10-5)*.004, 51.1282 + float64(i/10-5)*.003}
		set := bson.M{"location.coordinates": old}
		if i == 0 {
			set["name"] = "User edited place"
			set["version"] = int64(2)
		}
		if i == 1 {
			set["deleted_at"] = time.Now()
			set["version"] = int64(2)
		}
		if _, err := f.db.Collection("places").UpdateOne(f.ctx, bson.M{"_id": mongostore.SeedID("place", i)}, bson.M{"$set": set}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Collection("app_meta").UpdateOne(f.ctx, bson.M{"_id": mongostore.SeedVersion}, bson.M{"$unset": bson.M{"locations_version": ""}}); err != nil {
		t.Fatal(err)
	}
	if created, err := mongostore.Seed(f.ctx, f.db); err != nil || created {
		t.Fatalf("upgrade: created=%v err=%v", created, err)
	}
	for i := 0; i < 100; i++ {
		var p struct {
			Location struct {
				Coordinates []float64 `bson:"coordinates"`
			} `bson:"location"`
			Version int64  `bson:"version"`
			Name    string `bson:"name"`
		}
		if err := f.db.Collection("places").FindOne(f.ctx, bson.M{"_id": mongostore.SeedID("place", i)}).Decode(&p); err != nil {
			t.Fatal(err)
		}
		oldLon := 71.4304 + float64(i%10-5)*.004
		if i < 2 && p.Location.Coordinates[0] != oldLon {
			t.Fatal("changed user-edited place")
		}
		if i >= 2 && (p.Location.Coordinates[0] == oldLon || p.Version != 2) {
			t.Fatalf("legacy place %d not upgraded", i)
		}
		if i == 0 && p.Name != "User edited place" {
			t.Fatal("overwrote user name")
		}
	}
	if _, err := mongostore.Seed(f.ctx, f.db); err != nil {
		t.Fatal(err)
	}
	n, err := f.db.Collection("places").CountDocuments(f.ctx, bson.M{"version": int64(2)})
	if err != nil || n != 100 {
		t.Fatal("upgrade was not idempotent")
	}
	for collection, want := range map[string]int64{"users": 51, "user_places": 400, "interactions": 1200} {
		n, e := f.db.Collection(collection).CountDocuments(f.ctx, bson.M{})
		if e != nil || n != want {
			t.Fatalf("changed %s count", collection)
		}
	}
}
