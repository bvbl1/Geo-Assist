package mongoadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	catalogmongo "geo-assist/internal/catalog/adapters/mongo"
	catalog "geo-assist/internal/catalog/application"
	place "geo-assist/internal/catalog/domain"
	rec "geo-assist/internal/recommendation/application"
	"geo-assist/internal/recommendation/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"sort"
	"time"
)

type Repository struct{ db *mongo.Database }

// Cleanup runs while holding the build lease. Retain active + newest three ready
// versions, and all versions younger than 24 hours. Delete only explicit IDs.
func (r *Repository) Cleanup(ctx context.Context, owner string) error {
	if err := r.Renew(ctx, owner); err != nil {
		return err
	}
	meta, err := r.Meta(ctx)
	if err != nil {
		return err
	}
	keep := map[bson.ObjectID]bool{oid(meta.ActiveID): true, oid(owner): true}
	cur, err := r.c("recommendation_models").Find(ctx, bson.M{"status": "ready"}, options.Find().SetSort(bson.D{{Key: "finished_at", Value: -1}}).SetLimit(3))
	if err != nil {
		return err
	}
	for cur.Next(ctx) {
		var d struct {
			ID bson.ObjectID `bson:"_id"`
		}
		if err = cur.Decode(&d); err != nil {
			cur.Close(ctx)
			return err
		}
		keep[d.ID] = true
	}
	err = cur.Err()
	cur.Close(ctx)
	if err != nil {
		return err
	}
	// Interrupted attempts from a dead process cannot publish with an expired lease.
	_, err = r.c("recommendation_models").UpdateMany(ctx, bson.M{"status": "building", "_id": bson.M{"$ne": oid(owner)}, "started_at": bson.M{"$lt": time.Now().Add(-60 * time.Second)}}, bson.M{"$set": bson.M{"status": "failed", "finished_at": time.Now(), "error_summary": "Build interrupted; lease expired."}})
	if err != nil {
		return err
	}
	old, err := r.c("recommendation_models").Find(ctx, bson.M{"status": bson.M{"$ne": "building"}, "finished_at": bson.M{"$lt": time.Now().Add(-24 * time.Hour)}}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(100))
	if err != nil {
		return err
	}
	defer old.Close(ctx)
	for old.Next(ctx) {
		var d struct {
			ID bson.ObjectID `bson:"_id"`
		}
		if err = old.Decode(&d); err != nil {
			return err
		}
		if keep[d.ID] {
			continue
		}
		// Fence the deletion in a transaction against concurrent lease takeover.
		session, e := r.db.Client().StartSession()
		if e != nil {
			return e
		}
		_, e = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
			if err := r.Renew(tx, owner); err != nil {
				return nil, err
			}
			if _, err := r.c("recommendation_items").DeleteMany(tx, bson.M{"model_id": d.ID}); err != nil {
				return nil, err
			}
			_, err := r.c("recommendation_models").DeleteOne(tx, bson.M{"_id": d.ID})
			return nil, err
		}, options.Transaction().SetWriteConcern(writeconcern.Majority()))
		session.EndSession(ctx)
		if e != nil {
			return e
		}
	}
	return old.Err()
}

func New(db *mongo.Database) *Repository { return &Repository{db: db} }
func oid(s string) bson.ObjectID         { id, _ := bson.ObjectIDFromHex(s); return id }
func (r *Repository) c(name string) *mongo.Collection {
	return r.db.Collection(name, options.Collection().SetWriteConcern(writeconcern.Majority()))
}
func (r *Repository) Init(ctx context.Context) error {
	for name, validator := range map[string]bson.M{
		"recommendation_models": {"$jsonSchema": bson.M{"bsonType": "object", "required": bson.A{"_id", "schema_version", "algorithm", "status", "started_at"}, "properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "schema_version": bson.M{"enum": bson.A{int32(1)}}, "status": bson.M{"enum": bson.A{"building", "ready", "failed", "skipped"}}}}},
		"recommendation_items":  {"$jsonSchema": bson.M{"bsonType": "object", "required": bson.A{"_id", "schema_version", "model_id", "place_id", "favorite_count", "neighbors"}, "properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "schema_version": bson.M{"enum": bson.A{int32(1)}}, "model_id": bson.M{"bsonType": "objectId"}, "place_id": bson.M{"bsonType": "objectId"}, "favorite_count": bson.M{"bsonType": "long", "minimum": 0}, "neighbors": bson.M{"bsonType": "array"}}}},
	} {
		err := r.db.CreateCollection(ctx, name, options.CreateCollection().SetValidator(validator))
		var cmd mongo.CommandError
		if err != nil && (!errors.As(err, &cmd) || cmd.Code != 48) {
			return err
		}
		if err != nil {
			if err = r.db.RunCommand(ctx, bson.D{{Key: "collMod", Value: name}, {Key: "validator", Value: validator}}).Err(); err != nil {
				return err
			}
		}
	}
	if _, err := r.c("recommendation_items").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "model_id", Value: 1}, {Key: "place_id", Value: 1}}, Options: options.Index().SetName("model_place_unique").SetUnique(true)}); err != nil {
		return err
	}
	if _, err := r.c("recommendation_models").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "finished_at", Value: -1}}, Options: options.Index().SetName("model_status_finished")}); err != nil {
		return err
	}
	_, err := r.c("app_meta").UpdateOne(ctx, bson.M{"_id": "recommendations"}, bson.M{"$setOnInsert": bson.M{"active_model_id": "", "last_attempt_id": "", "verified_model_id": "", "lease": bson.M{"owner": "", "expires_at": time.Unix(0, 0)}}}, options.UpdateOne().SetUpsert(true))
	return err
}
func (r *Repository) Meta(ctx context.Context) (rec.Meta, error) {
	var m rec.Meta
	err := r.c("app_meta").FindOne(ctx, bson.M{"_id": "recommendations"}).Decode(&m)
	return m, err
}
func attemptDoc(a rec.Attempt) bson.M {
	data, _ := bson.Marshal(a)
	var d bson.M
	_ = bson.Unmarshal(data, &d)
	d["_id"] = oid(a.ID)
	return d
}
func (r *Repository) Attempt(ctx context.Context, id string) (*rec.Attempt, error) {
	if id == "" {
		return nil, nil
	}
	var d bson.M
	err := r.c("recommendation_models").FindOne(ctx, bson.M{"_id": oid(id)}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d["_id"] = id
	raw, err := bson.Marshal(d)
	if err != nil {
		return nil, err
	}
	var a rec.Attempt
	err = bson.Unmarshal(raw, &a)
	return &a, err
}
func leaseFilter(owner string) bson.M {
	return bson.M{"_id": "recommendations", "lease.owner": owner, "$expr": bson.M{"$gt": bson.A{"$lease.expires_at", "$$NOW"}}}
}
func leaseUpdate(owner string) bson.M {
	return bson.M{"lease.owner": owner, "lease.expires_at": bson.M{"$dateAdd": bson.M{"startDate": "$$NOW", "unit": "second", "amount": 60}}}
}
func (r *Repository) Acquire(ctx context.Context, a rec.Attempt) error {
	filter := bson.M{"_id": "recommendations", "$expr": bson.M{"$lte": bson.A{"$lease.expires_at", "$$NOW"}}}
	// A regular timer must not bypass the retry deadline after a failure.
	if a.Trigger == "scheduled" {
		filter["$or"] = bson.A{bson.M{"next_attempt_at": nil}, bson.M{"$expr": bson.M{"$lte": bson.A{"$next_attempt_at", "$$NOW"}}}}
	}
	if a.Trigger == "admin" {
		filter["$or"] = bson.A{bson.M{"last_manual_started_at": nil}, bson.M{"$expr": bson.M{"$lte": bson.A{"$last_manual_started_at", bson.M{"$dateSubtract": bson.M{"startDate": "$$NOW", "unit": "second", "amount": 60}}}}}}
	}
	update := leaseUpdate(a.ID)
	update["last_attempt_id"] = a.ID
	update["next_attempt_at"] = nil
	if a.Trigger == "admin" {
		update["last_manual_started_at"] = "$$NOW"
	}
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		res, err := r.c("app_meta").UpdateOne(tx, filter, mongo.Pipeline{{{Key: "$set", Value: update}}})
		if err != nil {
			return nil, err
		}
		if res.MatchedCount != 1 {
			m, e := r.Meta(tx)
			if e != nil {
				return nil, e
			}
			if m.Lease.Owner != "" && m.Lease.Expires.After(time.Now()) {
				return nil, app.E(409, "MODEL_BUILD_IN_PROGRESS", "A model build is already running")
			}
			return nil, app.E(429, "MODEL_BUILD_COOLDOWN", "Wait before requesting another build")
		}
		_, err = r.c("recommendation_models").InsertOne(tx, attemptDoc(a))
		return nil, err
	}, options.Transaction().SetWriteConcern(writeconcern.Majority()))
	return err
}
func (r *Repository) Renew(ctx context.Context, owner string) error {
	res, err := r.c("app_meta").UpdateOne(ctx, leaseFilter(owner), mongo.Pipeline{{{Key: "$set", Value: leaseUpdate(owner)}}})
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return fmt.Errorf("model lease lost")
	}
	return nil
}
func (r *Repository) Snapshot(ctx context.Context) (domain.Source, error) {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return domain.Source{}, err
	}
	defer session.EndSession(ctx)
	var source domain.Source
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		source = domain.Source{Places: []string{}, Edges: []domain.Edge{}}
		valid := map[bson.ObjectID]bool{}
		cur, err := r.c("places").Find(tx, bson.M{"deleted_at": nil}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(10001))
		if err != nil {
			return nil, err
		}
		defer cur.Close(tx)
		for cur.Next(tx) {
			var d struct {
				ID bson.ObjectID `bson:"_id"`
			}
			if err = cur.Decode(&d); err != nil {
				return nil, err
			}
			valid[d.ID] = true
			source.Places = append(source.Places, d.ID.Hex())
		}
		if err = cur.Err(); err != nil {
			return nil, err
		}
		if len(source.Places) > 10000 {
			return nil, fmt.Errorf("place limit exceeded")
		}
		pipeline := mongo.Pipeline{{{Key: "$match", Value: bson.M{"favorite": true}}}, {{Key: "$lookup", Value: bson.M{"from": "users", "localField": "user_id", "foreignField": "_id", "as": "user", "pipeline": bson.A{bson.M{"$project": bson.M{"_id": 1}}}}}}, {{Key: "$match", Value: bson.M{"user.0": bson.M{"$exists": true}}}}, {{Key: "$project", Value: bson.M{"user_id": 1, "place_id": 1}}}, {{Key: "$limit", Value: 200001}}}
		edges, err := r.c("user_places").Aggregate(tx, pipeline)
		if err != nil {
			return nil, err
		}
		defer edges.Close(tx)
		count := 0
		for edges.Next(tx) {
			count++
			if count > 200000 {
				return nil, fmt.Errorf("favorite edge limit exceeded")
			}
			var e struct {
				User  bson.ObjectID `bson:"user_id"`
				Place bson.ObjectID `bson:"place_id"`
			}
			if err = edges.Decode(&e); err != nil {
				return nil, err
			}
			if valid[e.Place] {
				source.Edges = append(source.Edges, domain.Edge{User: e.User.Hex(), Place: e.Place.Hex()})
			}
		}
		return nil, edges.Err()
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return source, err
}

type neighborDoc struct {
	PlaceID    bson.ObjectID `bson:"place_id"`
	Similarity float64       `bson:"similarity"`
	Count      int64         `bson:"cofavorite_count"`
}
type itemDoc struct {
	ID        bson.ObjectID `bson:"_id"`
	Schema    int32         `bson:"schema_version"`
	ModelID   bson.ObjectID `bson:"model_id"`
	PlaceID   bson.ObjectID `bson:"place_id"`
	Count     int64         `bson:"favorite_count"`
	Neighbors []neighborDoc `bson:"neighbors"`
}

func digest(items []domain.Item) string {
	sort.Slice(items, func(i, j int) bool { return items[i].PlaceID < items[j].PlaceID })
	h := sha256.New()
	for _, item := range items {
		raw, _ := bson.Marshal(item)
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (r *Repository) Save(ctx context.Context, a *rec.Attempt, m *domain.Model) error {
	items := make([]domain.Item, 0, len(m.Items))
	for _, item := range m.Items {
		items = append(items, item)
	}
	a.Checksum = digest(items)
	batch := []any{}
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := r.c("recommendation_items").InsertMany(ctx, batch)
		batch = nil
		return err
	}
	for _, item := range items {
		doc := itemDoc{ID: bson.NewObjectID(), Schema: 1, ModelID: oid(a.ID), PlaceID: oid(item.PlaceID), Count: item.FavoriteCount, Neighbors: []neighborDoc{}}
		for _, n := range item.Neighbors {
			doc.Neighbors = append(doc.Neighbors, neighborDoc{oid(n.PlaceID), n.Similarity, n.CofavoriteCount})
		}
		raw, err := bson.Marshal(doc)
		if err != nil {
			return err
		}
		if len(raw) > 8*1024*1024 {
			return fmt.Errorf("item size limit exceeded")
		}
		a.ModelBytes += len(raw)
		if a.ModelBytes > 256*1024*1024 {
			return fmt.Errorf("model size limit exceeded")
		}
		batch = append(batch, doc)
		if len(batch) == 100 {
			if err = flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	stored, err := r.readItems(ctx, a.ID)
	if err != nil {
		return err
	}
	if len(stored) != len(items) || digest(stored) != a.Checksum {
		return fmt.Errorf("model verification failed")
	}
	return nil
}
func (r *Repository) Finish(ctx context.Context, a rec.Attempt, publish bool) error {
	now := time.Now().UTC()
	a.FinishedAt = &now
	a.DurationMS = now.Sub(a.StartedAt).Milliseconds()
	if publish {
		a.BuiltAt = &now
	}
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		set := bson.M{"next_attempt_at": nil, "failure_count": 0, "last_checked_at": now, "verified_model_id": a.ReusedModelID, "lease.owner": "", "lease.expires_at": time.Unix(0, 0)}
		if publish {
			set["active_model_id"] = a.ID
			set["active_published_at"] = now
			set["verified_model_id"] = a.ID
		}
		res, err := r.c("app_meta").UpdateOne(tx, leaseFilter(a.ID), bson.M{"$set": set})
		if err != nil {
			return nil, err
		}
		if res.MatchedCount != 1 {
			return nil, fmt.Errorf("publication lease lost")
		}
		_, err = r.c("recommendation_models").ReplaceOne(tx, bson.M{"_id": oid(a.ID), "status": "building"}, attemptDoc(a))
		return nil, err
	}, options.Transaction().SetWriteConcern(writeconcern.Majority()))
	return err
}
func (r *Repository) Fail(ctx context.Context, a rec.Attempt) error {
	// A lost acknowledgement must never turn an already published version into a failed one.
	now := time.Now().UTC()
	a.FinishedAt = &now
	a.DurationMS = now.Sub(a.StartedAt).Milliseconds()
	_, err := r.c("recommendation_models").ReplaceOne(ctx, bson.M{"_id": oid(a.ID), "status": "building"}, attemptDoc(a))
	if err != nil {
		return err
	}
	exponent := bson.M{"$min": bson.A{4, bson.M{"$subtract": bson.A{"$failure_count", 1}}}}
	delay := bson.M{"$toInt": bson.M{"$min": bson.A{300, bson.M{"$multiply": bson.A{30, bson.M{"$pow": bson.A{2, exponent}}}}}}}
	next := bson.M{"$dateAdd": bson.M{"startDate": "$$NOW", "unit": "second", "amount": delay}}
	_, err = r.c("app_meta").UpdateOne(ctx, bson.M{"_id": "recommendations", "lease.owner": a.ID}, mongo.Pipeline{
		{{Key: "$set", Value: bson.M{"failure_count": bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$failure_count", 0}}, 1}}, "lease.owner": "", "lease.expires_at": time.Unix(0, 0)}}},
		{{Key: "$set", Value: bson.M{"next_attempt_at": next}}},
	})
	return err
}
func (r *Repository) readItems(ctx context.Context, id string) ([]domain.Item, error) {
	cur, err := r.c("recommendation_items").Find(ctx, bson.M{"model_id": oid(id)}, options.Find().SetLimit(10001))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := []domain.Item{}
	size := 0
	for cur.Next(ctx) {
		size += len(cur.Current)
		if size > 256*1024*1024 || len(items) >= 10000 {
			return nil, fmt.Errorf("stored model size limit exceeded")
		}
		var d itemDoc
		if err = cur.Decode(&d); err != nil {
			return nil, err
		}
		item := domain.Item{PlaceID: d.PlaceID.Hex(), FavoriteCount: d.Count, Neighbors: []domain.Neighbor{}}
		for _, n := range d.Neighbors {
			item.Neighbors = append(item.Neighbors, domain.Neighbor{PlaceID: n.PlaceID.Hex(), Similarity: n.Similarity, CofavoriteCount: n.Count})
		}
		items = append(items, item)
	}
	return items, cur.Err()
}
func (r *Repository) Load(ctx context.Context, id string) (*rec.Loaded, error) {
	a, err := r.Attempt(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil || a.Status != "ready" {
		return nil, fmt.Errorf("active model is not ready")
	}
	items, err := r.readItems(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(items) != a.ItemCount || digest(items) != a.Checksum {
		return nil, fmt.Errorf("active model checksum mismatch")
	}
	m := &domain.Model{Items: map[string]domain.Item{}, Fingerprint: a.Fingerprint, PairCount: a.PairCount}
	for _, item := range items {
		m.Items[item.PlaceID] = item
	}
	return &rec.Loaded{Model: m, Attempt: *a}, nil
}
func (r *Repository) Candidates(ctx context.Context, q catalog.SearchQuery) ([]place.PlaceHit, error) {
	filter := bson.M{"deleted_at": nil}
	if q.Category != "" {
		filter["category"] = q.Category
	}
	if len(q.Tags) > 0 {
		filter["tags"] = bson.M{"$all": q.Tags}
	}
	pipeline := mongo.Pipeline{}
	if q.Geo != nil {
		pipeline = append(pipeline, bson.D{{Key: "$geoNear", Value: bson.M{"near": bson.M{"type": "Point", "coordinates": bson.A{q.Geo.Center.Longitude, q.Geo.Center.Latitude}}, "distanceField": "distance_m", "maxDistance": q.Geo.RadiusM, "spherical": true, "query": filter}}})
	} else {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: filter}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$limit", Value: 10001}})
	cur, err := r.c("places").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	hits := []place.PlaceHit{}
	for cur.Next(ctx) {
		if len(hits) >= 10000 {
			return nil, app.E(503, "MODEL_CAPACITY_EXCEEDED", "Candidate limit exceeded")
		}
		var d struct {
			catalogmongo.Document `bson:",inline"`
			Distance              *float64 `bson:"distance_m"`
		}
		if err = cur.Decode(&d); err != nil {
			return nil, err
		}
		hits = append(hits, place.PlaceHit{Place: catalogmongo.FromDocument(d.Document), DistanceM: d.Distance})
	}
	return hits, cur.Err()
}
func (r *Repository) Favorites(ctx context.Context, user shared.ID) ([]string, error) {
	cur, err := r.c("user_places").Aggregate(ctx, mongo.Pipeline{{{Key: "$match", Value: bson.M{"user_id": oid(string(user)), "favorite": true}}}, {{Key: "$lookup", Value: bson.M{"from": "places", "localField": "place_id", "foreignField": "_id", "as": "place"}}}, {{Key: "$match", Value: bson.M{"place.0": bson.M{"$exists": true}, "place.deleted_at": nil}}}, {{Key: "$project", Value: bson.M{"place_id": 1}}}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	ids := []string{}
	for cur.Next(ctx) {
		var d struct {
			ID bson.ObjectID `bson:"place_id"`
		}
		if err = cur.Decode(&d); err != nil {
			return nil, err
		}
		ids = append(ids, d.ID.Hex())
	}
	return ids, cur.Err()
}
func (r *Repository) Popularity(ctx context.Context) (map[string]float64, error) {
	cur, err := r.c("user_places").Aggregate(ctx, mongo.Pipeline{{{Key: "$match", Value: bson.M{"favorite": true}}}, {{Key: "$lookup", Value: bson.M{"from": "users", "localField": "user_id", "foreignField": "_id", "as": "user", "pipeline": bson.A{bson.M{"$project": bson.M{"_id": 1}}}}}}, {{Key: "$match", Value: bson.M{"user.0": bson.M{"$exists": true}}}}, {{Key: "$group", Value: bson.M{"_id": "$place_id", "count": bson.M{"$sum": 1}}}}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	scores := map[string]float64{}
	for cur.Next(ctx) {
		var d struct {
			ID    bson.ObjectID `bson:"_id"`
			Count float64       `bson:"count"`
		}
		if err = cur.Decode(&d); err != nil {
			return nil, err
		}
		scores[d.ID.Hex()] = d.Count
	}
	return scores, cur.Err()
}
