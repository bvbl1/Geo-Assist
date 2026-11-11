package mongostore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
	"strings"
)

const DemoAdminEmail = "admin@geoassist.local"
const DemoAdminPassword = "Admin-demo-2026!"
const DemoUserPassword = "Student-demo-2026!"
const SeedVersion = "astana-demo-v1"

func SeedID(kind string, n int) bson.ObjectID {
	sum := sha256.Sum256([]byte(fmt.Sprintf("geoassist:%s:%d", kind, n)))
	var id bson.ObjectID
	copy(id[:], sum[:12])
	return id
}
func eventKey(key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("geoassist:"+key)).String()
}

// Seed installs a complete synthetic Astana dataset exactly once. Existing data is never overwritten.
// A metadata marker and all demo records commit atomically, including across interrupted startups.
func Seed(ctx context.Context, db *mongo.Database) (bool, error) {
	meta := db.Collection("app_meta")
	if err := meta.FindOne(ctx, bson.M{"_id": SeedVersion}).Err(); err == nil {
		return false, nil
	} else if err != mongo.ErrNoDocuments {
		return false, err
	}
	// Create the metadata collection outside the transaction (safe on repeat).
	err := db.CreateCollection(ctx, "app_meta")
	if err != nil {
		var cmd mongo.CommandError
		if !errors.As(err, &cmd) || cmd.Code != 48 {
			return false, err
		}
	}
	adminHash, err := bcrypt.GenerateFromPassword([]byte(DemoAdminPassword), 12)
	if err != nil {
		return false, err
	}
	userHash, err := bcrypt.GenerateFromPassword([]byte(DemoUserPassword), 12)
	if err != nil {
		return false, err
	}
	users, places, states, events := demoDocuments(string(adminHash), string(userHash))
	session, err := db.Client().StartSession()
	if err != nil {
		return false, err
	}
	defer session.EndSession(ctx)
	result, err := session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		if err := meta.FindOne(tx, bson.M{"_id": SeedVersion}).Err(); err == nil {
			return false, nil
		} else if err != mongo.ErrNoDocuments {
			return nil, err
		}
		for _, name := range []string{"users", "places", "user_places", "interactions"} {
			n, e := db.Collection(name).CountDocuments(tx, bson.M{}, options.Count().SetLimit(1))
			if e != nil {
				return nil, e
			}
			if n != 0 {
				return nil, fmt.Errorf("auto seed requires an empty demo database (found %s); disable AUTO_SEED to use existing data", name)
			}
		}
		for _, batch := range []struct {
			name string
			docs []any
		}{{"users", users}, {"places", places}, {"user_places", states}, {"interactions", events}} {
			if _, e := db.Collection(batch.name).InsertMany(tx, batch.docs); e != nil {
				return nil, fmt.Errorf("seed %s: %w", batch.name, e)
			}
		}
		_, e := meta.InsertOne(tx, bson.M{"_id": SeedVersion, "created_at": time.Now().UTC(), "city": "Astana", "synthetic": true})
		return true, e
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	if err != nil {
		return false, err
	}
	created, _ := result.(bool)
	if created {
		slog.Info("demo seed ready", "city", "Astana", "users", 51, "places", 100, "states", len(states), "events", len(events))
	}
	return created, nil
}

func demoDocuments(adminHash, userHash string) (users, places, states, events []any) {
	at := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	cats := []string{"cafe", "park", "library", "coworking", "attraction"}
	names := []string{"Кофейня", "Парк", "Библиотека", "Коворкинг", "Достопримечательность"}
	admin := SeedID("user", 0)
	for i := 0; i <= 50; i++ {
		email := fmt.Sprintf("student%02d@geoassist.local", i)
		name := fmt.Sprintf("Demo Student %02d", i)
		role := "user"
		hash := userHash
		prefs := []string{cats[(i+4)%5]}
		if i == 0 {
			email = DemoAdminEmail
			name = "Demo Administrator"
			role = "admin"
			hash = adminHash
			prefs = []string{}
		}
		users = append(users, bson.M{"_id": SeedID("user", i), "schema_version": int32(1), "email": email, "password_hash": hash, "display_name": name, "role": role, "preferred_categories": prefs, "created_at": at, "updated_at": at})
	}
	for i := 0; i < 100; i++ {
		category := i % 5
		name := fmt.Sprintf("Астана — %s %02d (демо)", names[category], i+1)
		tags := []string{"demo"}
		if category == 0 || category == 2 || category == 3 {
			tags = append(tags, "quiet", "wifi")
		} else {
			tags = append(tags, "outdoor")
		}
		lng := 71.4304 + float64(i%10-5)*0.004
		lat := 51.1282 + float64(i/10-5)*0.003
		places = append(places, bson.M{"_id": SeedID("place", i), "schema_version": int32(1), "created_by": admin, "name": name, "name_search": strings.ToLower(norm.NFC.String(name)), "description": "Синтетическое учебное место в Астане. Название и характеристики вымышлены.", "category": cats[category], "tags": tags, "address": fmt.Sprintf("Астана, учебная точка %02d (не реальный адрес)", i+1), "location": bson.M{"type": "Point", "coordinates": []float64{lng, lat}}, "version": int64(1), "created_at": at, "updated_at": at, "deleted_at": nil})
	}
	count := 0
	for u := 1; u <= 50; u++ {
		group := (u - 1) % 5
		for j := 0; j < 8; j++ {
			p := ((u/5+j)%20)*5 + group
			stamp := at.Add(time.Duration(count+1) * time.Minute)
			r := int32(4 + ((u + j) % 2))
			userID, placeID := SeedID("user", u), SeedID("place", p)
			states = append(states, bson.M{"_id": SeedID("state", count), "schema_version": int32(1), "user_id": userID, "place_id": placeID, "favorite": true, "favorited_at": stamp, "rating": r, "created_at": stamp, "updated_at": stamp.Add(time.Second)})
			for k, kind := range []string{"view", "favorite_added", "rating_set"} {
				var rating any
				if kind == "rating_set" {
					rating = r
				}
				events = append(events, bson.M{"_id": SeedID("event", count*3+k), "schema_version": int32(1), "user_id": userID, "place_id": placeID, "event_key": eventKey(hex.EncodeToString(userID[:]) + fmt.Sprintf(":%d:%s", p, kind)), "type": kind, "previous_rating": nil, "rating": rating, "occurred_at": stamp.Add(time.Duration(k-1) * time.Second)})
			}
			count++
		}
	}
	return
}
