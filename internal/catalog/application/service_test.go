package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
)

type fakeRepository struct {
	place     domain.Place
	created   bool
	updated   bool
	deleted   bool
	lastQuery SearchQuery
}

func (r *fakeRepository) GetAny(context.Context, shared.ID) (domain.Place, error) {
	return r.place, nil
}
func (r *fakeRepository) Create(_ context.Context, p domain.Place) error {
	r.created = true
	r.place = p
	return nil
}
func (r *fakeRepository) Update(_ context.Context, p domain.Place, version int64) error {
	r.updated = true
	r.place = p
	return nil
}
func (r *fakeRepository) SoftDelete(context.Context, shared.ID, int64, time.Time) error {
	r.deleted = true
	return nil
}
func (r *fakeRepository) Search(_ context.Context, q SearchQuery) (app.Page[domain.PlaceHit], error) {
	r.lastQuery = q
	return app.Page[domain.PlaceHit]{Items: []domain.PlaceHit{}}, nil
}

func actor(role string) app.Actor {
	return app.Actor{UserID: shared.ID("507f1f77bcf86cd799439012"), Role: role}
}
func command() CreateCommand {
	return CreateCommand{Name: "Example", Description: "Demo place", Address: "Astana", Category: domain.Cafe, Location: domain.Coordinates{Longitude: 71.4304, Latitude: 51.1282}}
}
func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *app.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func TestNonAdminCannotMutateCatalog(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)
	ctx := context.Background()
	_, err := s.Create(ctx, actor("user"), command())
	expectCode(t, err, "FORBIDDEN")
	name := "new"
	_, err = s.Update(ctx, actor("user"), "507f1f77bcf86cd799439011", UpdateCommand{ExpectedVersion: 1, Name: &name})
	expectCode(t, err, "FORBIDDEN")
	err = s.Delete(ctx, actor("user"), "507f1f77bcf86cd799439011", 1)
	expectCode(t, err, "FORBIDDEN")
	if r.created || r.updated || r.deleted {
		t.Fatal("unauthorized request reached mutation")
	}
}

func TestPatchPreservesFieldsAndChecksVersion(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)
	ctx := context.Background()
	p, err := s.Create(ctx, actor("admin"), command())
	if err != nil {
		t.Fatal(err)
	}
	name := "Updated"
	_, err = s.Update(ctx, actor("admin"), p.ID, UpdateCommand{ExpectedVersion: 2, Name: &name})
	expectCode(t, err, "VERSION_CONFLICT")
	if r.updated {
		t.Fatal("stale update reached persistence")
	}
	updated, err := s.Update(ctx, actor("admin"), p.ID, UpdateCommand{ExpectedVersion: 1, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Name != name || updated.Description != p.Description || updated.Location != p.Location {
		t.Fatalf("bad partial update: %+v", updated)
	}
}

func TestDeletedPlaceVisibility(t *testing.T) {
	now := time.Now()
	r := &fakeRepository{place: domain.Place{DeletedAt: &now}}
	s := NewService(r)
	_, err := s.GetActive(context.Background(), "507f1f77bcf86cd799439011")
	expectCode(t, err, "NOT_FOUND")
	if _, err := s.GetAny(context.Background(), "507f1f77bcf86cd799439011"); err != nil {
		t.Fatal(err)
	}
}

func TestSearchValidatesBeforeRepository(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)
	q := SearchQuery{NamePrefix: "  EXAMPLE ", Tags: []string{"WiFi"}, Pagination: app.Pagination{Limit: 20}}
	if _, err := s.Search(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if r.lastQuery.NamePrefix != "example" || r.lastQuery.Tags[0] != "wifi" {
		t.Fatalf("query not normalized: %+v", r.lastQuery)
	}
	q.Geo = &GeoQuery{Center: domain.Coordinates{Latitude: 91}, RadiusM: 500}
	_, err := s.Search(context.Background(), q)
	expectCode(t, err, "VALIDATION_ERROR")
}
