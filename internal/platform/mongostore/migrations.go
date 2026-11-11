package mongostore

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func field(t any) bson.M { return bson.M{"bsonType": t} }
func text(min, max int) bson.M {
	return bson.M{"bsonType": "string", "minLength": min, "maxLength": max}
}
func enum(values ...string) bson.M { return bson.M{"bsonType": "string", "enum": values} }
func categories() bson.M           { return enum("cafe", "park", "library", "coworking", "attraction") }
func rating() bson.M               { return bson.M{"bsonType": bson.A{"int", "null"}, "minimum": 1, "maximum": 5} }
func schema(properties bson.M) bson.M {
	properties["_id"] = field("objectId")
	properties["schema_version"] = bson.M{"bsonType": "int", "enum": bson.A{int32(1)}}
	required := []string{}
	for key := range properties {
		required = append(required, key)
	}
	return bson.M{"$jsonSchema": bson.M{"bsonType": "object", "required": required, "additionalProperties": false, "properties": properties}}
}

// Migrate installs the v1 schema without deleting or rewriting existing records.
func Migrate(ctx context.Context, db *mongo.Database) error {
	nullableDate := field(bson.A{"date", "null"})
	validators := map[string]bson.M{
		"users":        schema(bson.M{"email": text(3, 254), "password_hash": text(1, 200), "display_name": text(1, 80), "role": enum("user", "admin"), "preferred_categories": bson.M{"bsonType": "array", "maxItems": 5, "uniqueItems": true, "items": categories()}, "created_at": field("date"), "updated_at": field("date")}),
		"sessions":     schema(bson.M{"user_id": field("objectId"), "token_hash": bson.M{"bsonType": "string", "pattern": "^[a-f0-9]{64}$"}, "created_at": field("date"), "expires_at": field("date")}),
		"places":       schema(bson.M{"created_by": field("objectId"), "name": text(1, 120), "name_search": text(1, 120), "description": text(1, 2000), "address": text(1, 300), "category": categories(), "tags": bson.M{"bsonType": "array", "maxItems": 20, "uniqueItems": true, "items": bson.M{"bsonType": "string", "pattern": "^[a-z0-9][a-z0-9_-]{0,31}$"}}, "location": bson.M{"bsonType": "object", "required": bson.A{"type", "coordinates"}, "additionalProperties": false, "properties": bson.M{"type": enum("Point"), "coordinates": bson.M{"bsonType": "array", "minItems": 2, "maxItems": 2, "items": field("double")}}}, "version": bson.M{"bsonType": "long", "minimum": 1}, "created_at": field("date"), "updated_at": field("date"), "deleted_at": nullableDate}),
		"user_places":  schema(bson.M{"user_id": field("objectId"), "place_id": field("objectId"), "favorite": field("bool"), "favorited_at": nullableDate, "rating": rating(), "created_at": field("date"), "updated_at": field("date")}),
		"interactions": schema(bson.M{"user_id": field("objectId"), "place_id": field("objectId"), "event_key": bson.M{"bsonType": "string", "pattern": "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"}, "type": enum("view", "select", "favorite_added", "favorite_removed", "rating_set", "rating_removed"), "previous_rating": rating(), "rating": rating(), "occurred_at": field("date")}),
	}
	validators["user_places"]["$or"] = bson.A{bson.M{"favorite": true, "favorited_at": bson.M{"$type": "date"}}, bson.M{"favorite": false, "favorited_at": nil}}
	validators["interactions"]["$or"] = bson.A{
		bson.M{"type": "rating_set", "rating": bson.M{"$type": "int"}},
		bson.M{"type": "rating_removed", "previous_rating": bson.M{"$type": "int"}, "rating": nil},
		bson.M{"type": bson.M{"$in": bson.A{"view", "select", "favorite_added", "favorite_removed"}}, "rating": nil, "previous_rating": nil},
	}
	for _, name := range []string{"users", "sessions", "places", "user_places", "interactions"} {
		err := db.CreateCollection(ctx, name, options.CreateCollection().SetValidator(validators[name]).SetValidationLevel("strict").SetValidationAction("error"))
		if err != nil {
			var cmd mongo.CommandError
			if !errors.As(err, &cmd) || cmd.Code != 48 {
				return fmt.Errorf("create %s: %w", name, err)
			}
			if err = db.RunCommand(ctx, bson.D{{Key: "collMod", Value: name}, {Key: "validator", Value: validators[name]}, {Key: "validationLevel", Value: "strict"}, {Key: "validationAction", Value: "error"}}).Err(); err != nil {
				return fmt.Errorf("validate %s: %w", name, err)
			}
		}
	}
	indexes := map[string][]mongo.IndexModel{
		"users":        {idx("users_email", bson.D{{Key: "email", Value: 1}}, true)},
		"sessions":     {idx("sessions_token", bson.D{{Key: "token_hash", Value: 1}}, true), {Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetName("sessions_expiry").SetExpireAfterSeconds(0)}},
		"places":       {idx("places_geo", bson.D{{Key: "location", Value: "2dsphere"}}, false), idx("places_name", bson.D{{Key: "deleted_at", Value: 1}, {Key: "name_search", Value: 1}, {Key: "_id", Value: 1}}, false), idx("places_category", bson.D{{Key: "deleted_at", Value: 1}, {Key: "category", Value: 1}, {Key: "name_search", Value: 1}, {Key: "_id", Value: 1}}, false)},
		"user_places":  {idx("user_place_unique", bson.D{{Key: "user_id", Value: 1}, {Key: "place_id", Value: 1}}, true), idx("user_favorites", bson.D{{Key: "user_id", Value: 1}, {Key: "favorite", Value: 1}, {Key: "favorited_at", Value: -1}, {Key: "_id", Value: -1}}, false), idx("favorite_users", bson.D{{Key: "favorite", Value: 1}, {Key: "place_id", Value: 1}, {Key: "user_id", Value: 1}}, false)},
		"interactions": {idx("event_idempotency", bson.D{{Key: "user_id", Value: 1}, {Key: "event_key", Value: 1}}, true), idx("user_history", bson.D{{Key: "user_id", Value: 1}, {Key: "occurred_at", Value: -1}, {Key: "_id", Value: -1}}, false)},
	}
	for name, models := range indexes {
		if _, err := db.Collection(name).Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("indexes %s: %w", name, err)
		}
	}
	return nil
}
func idx(name string, keys bson.D, unique bool) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys, Options: options.Index().SetName(name).SetUnique(unique)}
}
