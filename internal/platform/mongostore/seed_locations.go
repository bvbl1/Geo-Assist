package mongostore

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

//go:embed astana-demo-locations.json
var demoLocationJSON []byte

type demoStreetLocation struct {
	Street      string     `json:"street"`
	Coordinates [2]float64 `json:"coordinates"`
	Anchor      [2]float64 `json:"street_anchor"`
}

var demoStreetLocations = func() []demoStreetLocation {
	var data struct {
		Locations []demoStreetLocation `json:"locations"`
	}
	if err := json.Unmarshal(demoLocationJSON, &data); err != nil {
		panic(err)
	}
	if len(data.Locations) != 100 {
		panic("expected 100 embedded Astana demo locations")
	}
	return data.Locations
}()

func demoAddress(i int) string {
	return fmt.Sprintf("Астана, возле %s; учебная точка %02d (не реальный адрес)", demoStreetLocations[i].Street, i+1)
}

// upgradeDemoLocations only moves untouched legacy grid records. The seed marker
// and updates commit together, so retries are safe and user edits are preserved.
func upgradeDemoLocations(ctx context.Context, db *mongo.Database) error {
	session, err := db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	changed := int64(0)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		changed = 0
		var marker struct {
			Version int `bson:"locations_version"`
		}
		if err := db.Collection("app_meta").FindOne(tx, bson.M{"_id": SeedVersion}).Decode(&marker); err != nil {
			return nil, err
		}
		if marker.Version >= 2 {
			return nil, nil
		}
		now := time.Now().UTC()
		for i, location := range demoStreetLocations {
			old := []float64{71.4304 + float64(i%10-5)*.004, 51.1282 + float64(i/10-5)*.003}
			filter := bson.M{"_id": SeedID("place", i), "created_by": SeedID("user", 0), "version": int64(1), "deleted_at": nil, "tags": "demo", "location.coordinates": old}
			result, err := db.Collection("places").UpdateOne(tx, filter, bson.M{"$set": bson.M{"location.coordinates": location.Coordinates, "address": demoAddress(i), "updated_at": now}, "$inc": bson.M{"version": int64(1)}})
			if err != nil {
				return nil, err
			}
			changed += result.ModifiedCount
		}
		_, err := db.Collection("app_meta").UpdateOne(tx, bson.M{"_id": SeedVersion}, bson.M{"$set": bson.M{"locations_version": 2, "locations_updated_at": now}})
		return nil, err
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	if err == nil && changed > 0 {
		slog.Info("demo coordinates moved to Astana streets", "places", changed)
	}
	return err
}
