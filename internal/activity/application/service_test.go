package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"geo-assist/internal/activity/domain"
	catalog "geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
)

type memoryRepo struct {
	state      domain.UserPlace
	events     []domain.Interaction
	failAppend bool
}

func (r *memoryRepo) GetState(_ context.Context, u, p shared.ID) (domain.UserPlace, error) {
	if r.state.ID == "" {
		return domain.UserPlace{UserID: u, PlaceID: p}, nil
	}
	return r.state, nil
}
func (r *memoryRepo) SaveState(_ context.Context, s domain.UserPlace) error { r.state = s; return nil }
func (r *memoryRepo) AppendEvent(_ context.Context, e domain.Interaction) error {
	if r.failAppend {
		return errors.New("injected event write failure")
	}
	r.events = append(r.events, e)
	return nil
}
func (r *memoryRepo) WithinTransaction(_ context.Context, fn func(Transaction) error) error {
	copyRepo := *r
	copyRepo.events = append([]domain.Interaction(nil), r.events...)
	if err := fn(&copyRepo); err != nil {
		return err
	}
	*r = copyRepo
	return nil
}
func (r *memoryRepo) FindEvent(_ context.Context, u shared.ID, k string) (domain.Interaction, bool, error) {
	for _, e := range r.events {
		if e.UserID == u && e.EventKey == k {
			return e, true, nil
		}
	}
	return domain.Interaction{}, false, nil
}
func (r *memoryRepo) InsertEvent(ctx context.Context, e domain.Interaction) (domain.Interaction, bool, error) {
	old, ok, _ := r.FindEvent(ctx, e.UserID, e.EventKey)
	if ok {
		return old, true, nil
	}
	r.events = append(r.events, e)
	return e, false, nil
}
func (r *memoryRepo) Favorites(context.Context, shared.ID, app.Pagination) (app.Page[Favorite], error) {
	return app.Page[Favorite]{}, nil
}
func (r *memoryRepo) History(context.Context, shared.ID, HistoryQuery) (app.Page[domain.Interaction], error) {
	return app.Page[domain.Interaction]{Items: r.events}, nil
}

type placeReader struct{ place catalog.Place }

func (p *placeReader) GetAny(_ context.Context, id shared.ID) (catalog.Place, error) {
	if id != p.place.ID {
		return catalog.Place{}, app.E(404, "NOT_FOUND", "Place not found")
	}
	return p.place, nil
}
func (p *placeReader) GetActive(ctx context.Context, id shared.ID) (catalog.Place, error) {
	v, e := p.GetAny(ctx, id)
	if e != nil {
		return v, e
	}
	if v.DeletedAt != nil {
		return catalog.Place{}, app.E(404, "NOT_FOUND", "Place not found")
	}
	return v, nil
}
func fixture() (*Service, *memoryRepo, *placeReader, app.Actor, shared.ID) {
	r := &memoryRepo{}
	p := &placeReader{place: catalog.Place{ID: shared.NewID()}}
	s := NewService(r, p)
	a := app.Actor{UserID: shared.NewID(), Role: "user"}
	return s, r, p, a, p.place.ID
}

func TestFavoriteNoopAndTransitions(t *testing.T) {
	s, r, _, a, id := fixture()
	ctx := context.Background()
	initial, err := s.State(ctx, a, id)
	if err != nil || initial.Favorite || initial.UpdatedAt != nil {
		t.Fatalf("virtual state: %+v %v", initial, err)
	}
	first, err := s.SetFavorite(ctx, a, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Favorite || first.FavoritedAt == nil || len(r.events) != 1 {
		t.Fatalf("first favorite: %+v", first)
	}
	repeated, err := s.SetFavorite(ctx, a, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.events) != 1 || !repeated.UpdatedAt.Equal(*first.UpdatedAt) {
		t.Fatal("duplicate PUT changed state or history")
	}
	removed, err := s.SetFavorite(ctx, a, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Favorite || removed.FavoritedAt != nil || len(r.events) != 2 || r.events[1].Type != domain.FavoriteRemoved {
		t.Fatalf("remove: %+v", removed)
	}
	if _, err = s.SetFavorite(ctx, a, id, false); err != nil || len(r.events) != 2 {
		t.Fatal("duplicate DELETE wrote event")
	}
}

func TestRatingTransitionsPreservePreviousValue(t *testing.T) {
	s, r, _, a, id := fixture()
	ctx := context.Background()
	rating := 4
	if _, err := s.SetRating(ctx, a, id, &rating); err != nil {
		t.Fatal(err)
	}
	rating = 5
	if _, err := s.SetRating(ctx, a, id, &rating); err != nil {
		t.Fatal(err)
	}
	if len(r.events) != 2 || *r.events[1].PreviousRating != 4 || *r.events[1].Rating != 5 {
		t.Fatalf("incorrect transitions: %+v", r.events)
	}
	if *r.events[0].Rating != 4 {
		t.Fatal("caller mutation changed previous event")
	}
	if _, err := s.SetRating(ctx, a, id, &rating); err != nil || len(r.events) != 2 {
		t.Fatal("rating no-op wrote event")
	}
	state, err := s.SetRating(ctx, a, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Rating != nil || r.events[2].Type != domain.RatingRemoved || *r.events[2].PreviousRating != 5 || r.events[2].Rating != nil {
		t.Fatal("incorrect removal")
	}
	bad := 6
	if _, err = s.SetRating(ctx, a, id, &bad); err == nil {
		t.Fatal("invalid rating accepted")
	}
}

func TestRollbackOnEventFailure(t *testing.T) {
	s, r, _, a, id := fixture()
	r.failAppend = true
	if _, err := s.SetFavorite(context.Background(), a, id, true); err == nil {
		t.Fatal("expected write failure")
	}
	if r.state.ID != "" || len(r.events) != 0 {
		t.Fatal("state survived failed transaction")
	}
}

func TestDeletedPlaceAllowsUndoButNotNewPreference(t *testing.T) {
	s, _, p, a, id := fixture()
	ctx := context.Background()
	if _, err := s.SetFavorite(ctx, a, id, true); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	p.place.DeletedAt = &now
	if _, err := s.SetFavorite(ctx, a, id, true); err == nil {
		t.Fatal("accepted favorite on deleted place")
	}
	if _, err := s.SetFavorite(ctx, a, id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.State(ctx, a, id); err != nil {
		t.Fatal("deleted state unavailable", err)
	}
}

func TestRecordIdempotencyAndReplayAfterDeletion(t *testing.T) {
	s, r, p, a, id := fixture()
	ctx := context.Background()
	input := RecordInput{PlaceID: id, EventKey: domain.NewEventKey(), Type: domain.View}
	first, replay, err := s.Record(ctx, a, input)
	if err != nil || replay {
		t.Fatalf("first: %v %v", replay, err)
	}
	now := time.Now()
	p.place.DeletedAt = &now
	second, replay, err := s.Record(ctx, a, input)
	if err != nil || !replay || first.ID != second.ID || len(r.events) != 1 {
		t.Fatalf("replay: %v %v", replay, err)
	}
	input.Type = domain.Select
	_, _, err = s.Record(ctx, a, input)
	var e *app.Error
	if !errors.As(err, &e) || e.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestClientCannotForgeStateEvents(t *testing.T) {
	s, r, _, a, id := fixture()
	_, _, err := s.Record(context.Background(), a, RecordInput{PlaceID: id, EventKey: domain.NewEventKey(), Type: domain.FavoriteAdded})
	if err == nil || len(r.events) != 0 {
		t.Fatal("forged state event accepted")
	}
	if _, err = s.SetFavorite(context.Background(), app.Actor{}, id, true); err == nil {
		t.Fatal("anonymous state mutation accepted")
	}
}
