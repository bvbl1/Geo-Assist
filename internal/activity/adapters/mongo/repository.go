package mongoadapter

import (
	"context"
	"errors"
	"time"

	"geo-assist/internal/activity/application"
	"geo-assist/internal/activity/domain"
	catalog "geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

type Repository struct {
	db             *mongo.Database
	states, events *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{db: db, states: db.Collection("user_places"), events: db.Collection("interactions")}
}

type stateDocument struct {
	ID            bson.ObjectID `bson:"_id"`
	SchemaVersion int32         `bson:"schema_version"`
	UserID        bson.ObjectID `bson:"user_id"`
	PlaceID       bson.ObjectID `bson:"place_id"`
	Favorite      bool          `bson:"favorite"`
	FavoritedAt   *time.Time    `bson:"favorited_at"`
	Rating        *int32        `bson:"rating"`
	CreatedAt     time.Time     `bson:"created_at"`
	UpdatedAt     time.Time     `bson:"updated_at"`
}
type eventDocument struct {
	ID             bson.ObjectID `bson:"_id"`
	SchemaVersion  int32         `bson:"schema_version"`
	UserID         bson.ObjectID `bson:"user_id"`
	PlaceID        bson.ObjectID `bson:"place_id"`
	EventKey       string        `bson:"event_key"`
	Type           string        `bson:"type"`
	PreviousRating *int32        `bson:"previous_rating"`
	Rating         *int32        `bson:"rating"`
	OccurredAt     time.Time     `bson:"occurred_at"`
}

func oid(id shared.ID) bson.ObjectID { v, _ := bson.ObjectIDFromHex(string(id)); return v }
func toInt32(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}
func toInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}
func (s stateDocument) model() domain.UserPlace {
	return domain.UserPlace{ID: shared.ID(s.ID.Hex()), UserID: shared.ID(s.UserID.Hex()), PlaceID: shared.ID(s.PlaceID.Hex()), Favorite: s.Favorite, FavoritedAt: s.FavoritedAt, Rating: toInt(s.Rating), CreatedAt: s.CreatedAt, UpdatedAt: &s.UpdatedAt}
}
func (e eventDocument) model() domain.Interaction {
	return domain.Interaction{ID: shared.ID(e.ID.Hex()), UserID: shared.ID(e.UserID.Hex()), PlaceID: shared.ID(e.PlaceID.Hex()), EventKey: e.EventKey, Type: e.Type, PreviousRating: toInt(e.PreviousRating), Rating: toInt(e.Rating), OccurredAt: e.OccurredAt}
}
func eventDoc(e domain.Interaction) eventDocument {
	return eventDocument{ID: oid(e.ID), SchemaVersion: 1, UserID: oid(e.UserID), PlaceID: oid(e.PlaceID), EventKey: e.EventKey, Type: e.Type, PreviousRating: toInt32(e.PreviousRating), Rating: toInt32(e.Rating), OccurredAt: e.OccurredAt}
}
func dbError(err error) error {
	if err == nil {
		return nil
	}
	var ae *app.Error
	if errors.As(err, &ae) {
		return err
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && ce.Code == 121 {
		return app.E(500, "INTERNAL_ERROR", "Data persistence validation failed")
	}
	return app.E(503, "SERVICE_UNAVAILABLE", "Activity storage unavailable")
}

func (r *Repository) getState(ctx context.Context, user, place shared.ID) (domain.UserPlace, error) {
	var doc stateDocument
	err := r.states.FindOne(ctx, bson.M{"user_id": oid(user), "place_id": oid(place)}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.UserPlace{UserID: user, PlaceID: place}, nil
	}
	if err != nil {
		return domain.UserPlace{}, err
	}
	return doc.model(), nil
}
func (r *Repository) GetState(ctx context.Context, user, place shared.ID) (domain.UserPlace, error) {
	s, err := r.getState(ctx, user, place)
	return s, dbError(err)
}

type transaction struct {
	repo *Repository
	ctx  context.Context
}

func (t *transaction) GetState(_ context.Context, u, p shared.ID) (domain.UserPlace, error) {
	return t.repo.getState(t.ctx, u, p)
}
func (t *transaction) SaveState(_ context.Context, s domain.UserPlace) error {
	doc := stateDocument{ID: oid(s.ID), SchemaVersion: 1, UserID: oid(s.UserID), PlaceID: oid(s.PlaceID), Favorite: s.Favorite, FavoritedAt: s.FavoritedAt, Rating: toInt32(s.Rating), CreatedAt: s.CreatedAt, UpdatedAt: *s.UpdatedAt}
	_, err := t.repo.states.ReplaceOne(t.ctx, bson.M{"user_id": doc.UserID, "place_id": doc.PlaceID}, doc, options.Replace().SetUpsert(true))
	return err
}
func (t *transaction) AppendEvent(_ context.Context, e domain.Interaction) error {
	_, err := t.repo.events.InsertOne(t.ctx, eventDoc(e))
	return err
}

func (r *Repository) WithinTransaction(ctx context.Context, fn func(application.Transaction) error) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for attempt := 0; attempt < 5; attempt++ {
		session, err := r.db.Client().StartSession()
		if err != nil {
			return dbError(err)
		}
		_, err = session.WithTransaction(ctx, func(txCtx context.Context) (any, error) { return nil, fn(&transaction{repo: r, ctx: txCtx}) }, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
		session.EndSession(ctx)
		if err == nil {
			return nil
		}
		// A competing first insert of the user/place pair may return duplicate key
		// rather than a transient transaction label. Retry the entire callback.
		if !mongo.IsDuplicateKeyError(err) {
			return dbError(err)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return dbError(ctx.Err())
		case <-timer.C:
		}
	}
	return app.E(503, "SERVICE_UNAVAILABLE", "Concurrent activity update; retry the request")
}

func (r *Repository) FindEvent(ctx context.Context, user shared.ID, key string) (domain.Interaction, bool, error) {
	var doc eventDocument
	err := r.events.FindOne(ctx, bson.M{"user_id": oid(user), "event_key": key}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Interaction{}, false, nil
	}
	if err != nil {
		return domain.Interaction{}, false, dbError(err)
	}
	return doc.model(), true, nil
}
func (r *Repository) InsertEvent(ctx context.Context, e domain.Interaction) (domain.Interaction, bool, error) {
	_, err := r.events.InsertOne(ctx, eventDoc(e))
	if err == nil {
		return e, false, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		existing, found, findErr := r.FindEvent(ctx, e.UserID, e.EventKey)
		if findErr != nil {
			return domain.Interaction{}, false, findErr
		}
		if found {
			return existing, true, nil
		}
	}
	return domain.Interaction{}, false, dbError(err)
}

type placeSnapshot struct {
	ID          bson.ObjectID `bson:"_id"`
	Name        string        `bson:"name"`
	Description string        `bson:"description"`
	Category    string        `bson:"category"`
	Tags        []string      `bson:"tags"`
	Address     string        `bson:"address"`
	Location    struct {
		Coordinates []float64 `bson:"coordinates"`
	} `bson:"location"`
	CreatedBy bson.ObjectID `bson:"created_by"`
	Version   int64         `bson:"version"`
	CreatedAt time.Time     `bson:"created_at"`
	UpdatedAt time.Time     `bson:"updated_at"`
	DeletedAt *time.Time    `bson:"deleted_at"`
}

func (p placeSnapshot) model() catalog.Place {
	c := catalog.Coordinates{}
	if len(p.Location.Coordinates) == 2 {
		c.Longitude = p.Location.Coordinates[0]
		c.Latitude = p.Location.Coordinates[1]
	}
	return catalog.Place{ID: shared.ID(p.ID.Hex()), Name: p.Name, Description: p.Description, Category: catalog.Category(p.Category), Tags: p.Tags, Address: p.Address, Location: c, CreatedBy: shared.ID(p.CreatedBy.Hex()), Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeletedAt: p.DeletedAt}
}

func (r *Repository) Favorites(ctx context.Context, user shared.ID, p app.Pagination) (app.Page[application.Favorite], error) {
	result := app.Page[application.Favorite]{Items: []application.Favorite{}}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": oid(user), "favorite": true}}},
		{{Key: "$lookup", Value: bson.M{"from": "places", "localField": "place_id", "foreignField": "_id", "as": "place"}}},
		{{Key: "$unwind", Value: "$place"}},
		{{Key: "$match", Value: bson.M{"place.deleted_at": nil}}},
		{{Key: "$sort", Value: bson.D{{Key: "favorited_at", Value: -1}, {Key: "_id", Value: -1}}}},
		{{Key: "$skip", Value: int64(p.Offset)}},
		{{Key: "$limit", Value: int64(p.Limit + 1)}},
	}
	cur, err := r.states.Aggregate(ctx, pipeline)
	if err != nil {
		return result, dbError(err)
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			FavoritedAt time.Time     `bson:"favorited_at"`
			Rating      *int32        `bson:"rating"`
			Place       placeSnapshot `bson:"place"`
		}
		if err = cur.Decode(&row); err != nil {
			return result, dbError(err)
		}
		result.Items = append(result.Items, application.Favorite{Place: row.Place.model(), FavoritedAt: row.FavoritedAt, Rating: toInt(row.Rating)})
	}
	if err = cur.Err(); err != nil {
		return result, dbError(err)
	}
	if len(result.Items) > p.Limit {
		result.HasMore = true
		result.Items = result.Items[:p.Limit]
	}
	return result, nil
}

func (r *Repository) History(ctx context.Context, user shared.ID, q application.HistoryQuery) (app.Page[domain.Interaction], error) {
	result := app.Page[domain.Interaction]{Items: []domain.Interaction{}}
	filter := bson.M{"user_id": oid(user)}
	if q.Type != "" {
		filter["type"] = q.Type
	}
	if q.PlaceID != "" {
		filter["place_id"] = oid(q.PlaceID)
	}
	cur, err := r.events.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "occurred_at", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(int64(q.Pagination.Offset)).SetLimit(int64(q.Pagination.Limit+1)))
	if err != nil {
		return result, dbError(err)
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc eventDocument
		if err = cur.Decode(&doc); err != nil {
			return result, dbError(err)
		}
		result.Items = append(result.Items, doc.model())
	}
	if err = cur.Err(); err != nil {
		return result, dbError(err)
	}
	if len(result.Items) > q.Pagination.Limit {
		result.HasMore = true
		result.Items = result.Items[:q.Pagination.Limit]
	}
	return result, nil
}

var _ application.Repository = (*Repository)(nil)
