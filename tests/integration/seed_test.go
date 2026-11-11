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
