package mongoadapter

import (
	"context"
	"errors"
	"regexp"
	"time"

	"geo-assist/internal/catalog/application"
	"geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type GeoPoint struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}

// Document is the persistence representation, also used for joined read models.
type Document struct {
	ID            bson.ObjectID `bson:"_id"`
	SchemaVersion int32         `bson:"schema_version"`
	Name          string        `bson:"name"`
	NameSearch    string        `bson:"name_search"`
	Description   string        `bson:"description"`
	Category      string        `bson:"category"`
	Tags          []string      `bson:"tags"`
	Address       string        `bson:"address"`
	Location      GeoPoint      `bson:"location"`
	CreatedBy     bson.ObjectID `bson:"created_by"`
	Version       int64         `bson:"version"`
	CreatedAt     time.Time     `bson:"created_at"`
	UpdatedAt     time.Time     `bson:"updated_at"`
	DeletedAt     *time.Time    `bson:"deleted_at"`
}

func FromDocument(d Document) domain.Place {
	location := domain.Coordinates{}
	if len(d.Location.Coordinates) == 2 {
		location.Longitude, location.Latitude = d.Location.Coordinates[0], d.Location.Coordinates[1]
	}
	tags := append([]string{}, d.Tags...)
	return domain.Place{ID: shared.ID(d.ID.Hex()), Name: d.Name, Description: d.Description,
		Category: domain.Category(d.Category), Tags: tags, Address: d.Address, Location: location,
		CreatedBy: shared.ID(d.CreatedBy.Hex()), Version: d.Version, CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt, DeletedAt: d.DeletedAt}
}

func ToDocument(p domain.Place) (Document, error) {
	id, err := bson.ObjectIDFromHex(string(p.ID))
	if err != nil {
		return Document{}, err
	}
	createdBy, err := bson.ObjectIDFromHex(string(p.CreatedBy))
	if err != nil {
		return Document{}, err
	}
	return Document{ID: id, SchemaVersion: 1, Name: p.Name, NameSearch: domain.NormalizeName(p.Name),
		Description: p.Description, Category: string(p.Category), Tags: append([]string{}, p.Tags...), Address: p.Address,
		Location:  GeoPoint{Type: "Point", Coordinates: []float64{p.Location.Longitude, p.Location.Latitude}},
		CreatedBy: createdBy, Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeletedAt: p.DeletedAt}, nil
}

type Repository struct{ places *mongo.Collection }

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{places: db.Collection("places")}
}

func dbError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return app.E(404, "NOT_FOUND", "Place not found")
	}
	return errors.Join(app.E(503, "SERVICE_UNAVAILABLE", "Database operation unavailable"), err)
}

func objectID(id shared.ID) (bson.ObjectID, error) {
	value, err := bson.ObjectIDFromHex(string(id))
	if err != nil {
		return bson.NilObjectID, app.E(400, "BAD_REQUEST", "Invalid place ID")
	}
	return value, nil
}

func (r *Repository) GetAny(ctx context.Context, id shared.ID) (domain.Place, error) {
	value, err := objectID(id)
	if err != nil {
		return domain.Place{}, err
	}
	var doc Document
	if err := r.places.FindOne(ctx, bson.M{"_id": value}).Decode(&doc); err != nil {
		return domain.Place{}, dbError(err)
	}
	return FromDocument(doc), nil
}

func (r *Repository) Create(ctx context.Context, place domain.Place) error {
	doc, err := ToDocument(place)
	if err != nil {
		return errors.Join(app.E(500, "INTERNAL_ERROR", "Invalid persistence identifier"), err)
	}
	_, err = r.places.InsertOne(ctx, doc)
	return dbError(err)
}

func (r *Repository) Update(ctx context.Context, place domain.Place, expectedVersion int64) error {
	doc, err := ToDocument(place)
	if err != nil {
		return err
	}
	result, err := r.places.UpdateOne(ctx, bson.M{"_id": doc.ID, "deleted_at": nil, "version": expectedVersion},
		bson.M{"$set": bson.M{"name": doc.Name, "name_search": doc.NameSearch, "description": doc.Description,
			"category": doc.Category, "tags": doc.Tags, "address": doc.Address, "location": doc.Location,
			"updated_at": doc.UpdatedAt}, "$inc": bson.M{"version": int64(1)}})
	if err != nil {
		return dbError(err)
	}
	if result.MatchedCount == 0 {
		return r.versionFailure(ctx, place.ID)
	}
	return nil
}

func (r *Repository) SoftDelete(ctx context.Context, id shared.ID, expectedVersion int64, at time.Time) error {
	value, err := objectID(id)
	if err != nil {
		return err
	}
	result, err := r.places.UpdateOne(ctx, bson.M{"_id": value, "deleted_at": nil, "version": expectedVersion},
		bson.M{"$set": bson.M{"deleted_at": at, "updated_at": at}, "$inc": bson.M{"version": int64(1)}})
	if err != nil {
		return dbError(err)
	}
	if result.MatchedCount == 0 {
		return r.versionFailure(ctx, id)
	}
	return nil
}

func (r *Repository) versionFailure(ctx context.Context, id shared.ID) error {
	p, err := r.GetAny(ctx, id)
	if err != nil {
		return err
	}
	if p.DeletedAt != nil {
		return app.E(404, "NOT_FOUND", "Place not found")
	}
	return app.E(409, "VERSION_CONFLICT", "Place was modified; reload it before retrying")
}

func searchFilter(query application.SearchQuery) bson.M {
	filter := bson.M{"deleted_at": nil}
	if query.NamePrefix != "" {
		filter["name_search"] = bson.M{"$regex": "^" + regexp.QuoteMeta(domain.NormalizeName(query.NamePrefix))}
	}
	if query.Category != "" {
		filter["category"] = string(query.Category)
	}
	if len(query.Tags) > 0 {
		filter["tags"] = bson.M{"$all": query.Tags}
	}
	return filter
}

func (r *Repository) Search(ctx context.Context, query application.SearchQuery) (app.Page[domain.PlaceHit], error) {
	filter := searchFilter(query)
	var cursor *mongo.Cursor
	var err error
	if query.Geo == nil {
		cursor, err = r.places.Find(ctx, filter, options.Find().
			SetSort(bson.D{{Key: "name_search", Value: 1}, {Key: "_id", Value: 1}}).
			SetSkip(int64(query.Pagination.Offset)).SetLimit(int64(query.Pagination.Limit+1)))
	} else {
		center := query.Geo.Center
		pipeline := mongo.Pipeline{
			bson.D{{Key: "$geoNear", Value: bson.M{"key": "location", "near": GeoPoint{"Point", []float64{center.Longitude, center.Latitude}},
				"spherical": true, "maxDistance": query.Geo.RadiusM, "distanceField": "distance_m", "query": filter}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "distance_m", Value: 1}, {Key: "_id", Value: 1}}}},
			bson.D{{Key: "$skip", Value: int64(query.Pagination.Offset)}},
			bson.D{{Key: "$limit", Value: int64(query.Pagination.Limit + 1)}},
		}
		cursor, err = r.places.Aggregate(ctx, pipeline)
	}
	if err != nil {
		return app.Page[domain.PlaceHit]{}, dbError(err)
	}
	defer cursor.Close(ctx)
	items := make([]domain.PlaceHit, 0, query.Pagination.Limit+1)
	for cursor.Next(ctx) {
		var hit struct {
			Document  `bson:",inline"`
			DistanceM *float64 `bson:"distance_m"`
		}
		if err := cursor.Decode(&hit); err != nil {
			return app.Page[domain.PlaceHit]{}, dbError(err)
		}
		items = append(items, domain.PlaceHit{Place: FromDocument(hit.Document), DistanceM: hit.DistanceM})
	}
	if err := cursor.Err(); err != nil {
		return app.Page[domain.PlaceHit]{}, dbError(err)
	}
	hasMore := len(items) > query.Pagination.Limit
	if hasMore {
		items = items[:query.Pagination.Limit]
	}
	return app.Page[domain.PlaceHit]{Items: items, HasMore: hasMore}, nil
}

var _ application.Repository = (*Repository)(nil)
