package mongoadapter

import (
	"context"
	"errors"
	identity "geo-assist/internal/identity/application"
	"geo-assist/internal/identity/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"time"
)

type Repository struct{ db *mongo.Database }

func NewRepository(db *mongo.Database) *Repository { return &Repository{db: db} }

type userDocument struct {
	ID                  bson.ObjectID `bson:"_id"`
	SchemaVersion       int32         `bson:"schema_version"`
	Email               string        `bson:"email"`
	PasswordHash        string        `bson:"password_hash"`
	DisplayName         string        `bson:"display_name"`
	Role                string        `bson:"role"`
	PreferredCategories []string      `bson:"preferred_categories"`
	CreatedAt           time.Time     `bson:"created_at"`
	UpdatedAt           time.Time     `bson:"updated_at"`
}
type sessionDocument struct {
	ID            bson.ObjectID `bson:"_id"`
	UserID        bson.ObjectID `bson:"user_id"`
	SchemaVersion int32         `bson:"schema_version"`
	TokenHash     string        `bson:"token_hash"`
	CreatedAt     time.Time     `bson:"created_at"`
	ExpiresAt     time.Time     `bson:"expires_at"`
}

func unavailable(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return identity.ErrNotFound
	}
	return app.E(503, "SERVICE_UNAVAILABLE", "Identity storage is unavailable")
}
func objectID(id shared.ID) (bson.ObjectID, error) { return bson.ObjectIDFromHex(string(id)) }
func (d userDocument) domain() domain.User {
	if d.PreferredCategories == nil {
		d.PreferredCategories = []string{}
	}
	return domain.User{ID: shared.ID(d.ID.Hex()), Email: d.Email, PasswordHash: d.PasswordHash, DisplayName: d.DisplayName, Role: d.Role, PreferredCategories: d.PreferredCategories, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}
func (r *Repository) CreateUser(ctx context.Context, u domain.User) error {
	id, err := objectID(u.ID)
	if err != nil {
		return err
	}
	categories := u.PreferredCategories
	if categories == nil {
		categories = []string{}
	}
	d := userDocument{ID: id, SchemaVersion: 1, Email: u.Email, PasswordHash: u.PasswordHash, DisplayName: u.DisplayName, Role: u.Role, PreferredCategories: categories, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
	_, err = r.db.Collection("users").InsertOne(ctx, d)
	if mongo.IsDuplicateKeyError(err) {
		return identity.ErrEmailExists
	}
	return unavailable(err)
}
func (r *Repository) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var d userDocument
	err := r.db.Collection("users").FindOne(ctx, bson.M{"email": email}).Decode(&d)
	return d.domain(), unavailable(err)
}
func (r *Repository) UserByID(ctx context.Context, id shared.ID) (domain.User, error) {
	oid, err := objectID(id)
	if err != nil {
		return domain.User{}, identity.ErrNotFound
	}
	var d userDocument
	err = r.db.Collection("users").FindOne(ctx, bson.M{"_id": oid}).Decode(&d)
	return d.domain(), unavailable(err)
}
func (r *Repository) UpdateProfile(ctx context.Context, id shared.ID, in identity.UpdateProfileInput, at time.Time) (domain.User, error) {
	oid, err := objectID(id)
	if err != nil {
		return domain.User{}, identity.ErrNotFound
	}
	set := bson.M{"updated_at": at}
	if in.DisplayName != nil {
		set["display_name"] = *in.DisplayName
	}
	if in.PreferredCategories != nil {
		categories := *in.PreferredCategories
		if categories == nil {
			categories = []string{}
		}
		set["preferred_categories"] = categories
	}
	var d userDocument
	err = r.db.Collection("users").FindOneAndUpdate(ctx, bson.M{"_id": oid}, bson.M{"$set": set}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	return d.domain(), unavailable(err)
}
func (r *Repository) RotateSession(ctx context.Context, oldHash string, s domain.Session) error {
	id, err := objectID(s.ID)
	if err != nil {
		return err
	}
	uid, err := objectID(s.UserID)
	if err != nil {
		return err
	}
	d := sessionDocument{ID: id, UserID: uid, SchemaVersion: 1, TokenHash: s.TokenHash, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
	session, err := r.db.Client().StartSession()
	if err != nil {
		return unavailable(err)
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		if oldHash != "" {
			if _, err := r.db.Collection("sessions").DeleteOne(tx, bson.M{"token_hash": oldHash}); err != nil {
				return nil, err
			}
		}
		_, err := r.db.Collection("sessions").InsertOne(tx, d)
		return nil, err
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return unavailable(err)
}
func (r *Repository) SessionByHash(ctx context.Context, hash string) (domain.Session, error) {
	var d sessionDocument
	err := r.db.Collection("sessions").FindOne(ctx, bson.M{"token_hash": hash}).Decode(&d)
	return domain.Session{ID: shared.ID(d.ID.Hex()), UserID: shared.ID(d.UserID.Hex()), TokenHash: d.TokenHash, CreatedAt: d.CreatedAt, ExpiresAt: d.ExpiresAt}, unavailable(err)
}
func (r *Repository) DeleteSession(ctx context.Context, hash string) error {
	_, err := r.db.Collection("sessions").DeleteOne(ctx, bson.M{"token_hash": hash})
	return unavailable(err)
}
