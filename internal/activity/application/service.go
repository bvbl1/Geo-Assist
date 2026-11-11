package application

import (
	"context"
	"strings"
	"time"

	"geo-assist/internal/activity/domain"
	catalog "geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
)

type PlaceReader interface {
	GetAny(context.Context, shared.ID) (catalog.Place, error)
	GetActive(context.Context, shared.ID) (catalog.Place, error)
}

type Transaction interface {
	GetState(context.Context, shared.ID, shared.ID) (domain.UserPlace, error)
	SaveState(context.Context, domain.UserPlace) error
	AppendEvent(context.Context, domain.Interaction) error
}

type Favorite struct {
	Place       catalog.Place
	FavoritedAt time.Time
	Rating      *int
}

type HistoryQuery struct {
	Pagination app.Pagination
	Type       string
	PlaceID    shared.ID
}

type Repository interface {
	GetState(context.Context, shared.ID, shared.ID) (domain.UserPlace, error)
	WithinTransaction(context.Context, func(Transaction) error) error
	FindEvent(context.Context, shared.ID, string) (domain.Interaction, bool, error)
	InsertEvent(context.Context, domain.Interaction) (domain.Interaction, bool, error)
	Favorites(context.Context, shared.ID, app.Pagination) (app.Page[Favorite], error)
	History(context.Context, shared.ID, HistoryQuery) (app.Page[domain.Interaction], error)
}

type Service struct {
	repo   Repository
	places PlaceReader
	now    func() time.Time
}

func NewService(repo Repository, places PlaceReader) *Service {
	return &Service{repo: repo, places: places, now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }}
}

func checkActor(actor app.Actor) error {
	if !shared.ValidID(string(actor.UserID)) {
		return app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	return nil
}
func checkPlaceID(id shared.ID) error {
	if !shared.ValidID(string(id)) {
		return app.E(400, "BAD_REQUEST", "Invalid place ID")
	}
	return nil
}
func checkPage(p app.Pagination) error {
	if p.Limit < 1 || p.Limit > 100 {
		return app.Validation("limit", "out_of_range")
	}
	if p.Offset < 0 || p.Offset > 10000 {
		return app.Validation("offset", "out_of_range")
	}
	return nil
}

func (s *Service) State(ctx context.Context, actor app.Actor, id shared.ID) (domain.UserPlace, error) {
	if err := checkActor(actor); err != nil {
		return domain.UserPlace{}, err
	}
	if err := checkPlaceID(id); err != nil {
		return domain.UserPlace{}, err
	}
	if _, err := s.places.GetAny(ctx, id); err != nil {
		return domain.UserPlace{}, err
	}
	return s.repo.GetState(ctx, actor.UserID, id)
}

func (s *Service) mutate(ctx context.Context, actor app.Actor, id shared.ID, active bool, change func(*domain.UserPlace, time.Time) (*domain.Interaction, error)) (domain.UserPlace, error) {
	if err := checkActor(actor); err != nil {
		return domain.UserPlace{}, err
	}
	if err := checkPlaceID(id); err != nil {
		return domain.UserPlace{}, err
	}
	var err error
	if active {
		_, err = s.places.GetActive(ctx, id)
	} else {
		_, err = s.places.GetAny(ctx, id)
	}
	if err != nil {
		return domain.UserPlace{}, err
	}
	var result domain.UserPlace
	err = s.repo.WithinTransaction(ctx, func(tx Transaction) error {
		state, e := tx.GetState(ctx, actor.UserID, id)
		if e != nil {
			return e
		}
		now := s.now()
		event, e := change(&state, now)
		if e != nil {
			return e
		}
		if event == nil {
			result = state
			return nil
		}
		if state.ID == "" {
			state.ID = shared.NewID()
			state.CreatedAt = now
		}
		state.UpdatedAt = &now
		event.ID = shared.NewID()
		event.UserID = actor.UserID
		event.PlaceID = id
		event.OccurredAt = now
		event.EventKey = domain.NewEventKey()
		if e = tx.SaveState(ctx, state); e != nil {
			return e
		}
		if e = tx.AppendEvent(ctx, *event); e != nil {
			return e
		}
		result = state
		return nil
	})
	return result, err
}

func (s *Service) SetFavorite(ctx context.Context, actor app.Actor, id shared.ID, value bool) (domain.UserPlace, error) {
	return s.mutate(ctx, actor, id, value, func(state *domain.UserPlace, now time.Time) (*domain.Interaction, error) {
		if state.Favorite == value {
			return nil, nil
		}
		state.Favorite = value
		e := &domain.Interaction{Type: domain.FavoriteRemoved}
		state.FavoritedAt = nil
		if value {
			state.FavoritedAt = &now
			e.Type = domain.FavoriteAdded
		}
		return e, nil
	})
}

func (s *Service) SetRating(ctx context.Context, actor app.Actor, id shared.ID, value *int) (domain.UserPlace, error) {
	if value != nil && (*value < 1 || *value > 5) {
		return domain.UserPlace{}, app.Validation("rating", "out_of_range")
	}
	return s.mutate(ctx, actor, id, value != nil, func(state *domain.UserPlace, _ time.Time) (*domain.Interaction, error) {
		if (state.Rating == nil && value == nil) || (state.Rating != nil && value != nil && *state.Rating == *value) {
			return nil, nil
		}
		e := &domain.Interaction{Type: domain.RatingSet, PreviousRating: domain.CopyRating(state.Rating), Rating: domain.CopyRating(value)}
		if value == nil {
			e.Type = domain.RatingRemoved
		}
		state.Rating = domain.CopyRating(value)
		return e, nil
	})
}

type RecordInput struct {
	PlaceID        shared.ID
	EventKey, Type string
}

func sameEvent(e domain.Interaction, in RecordInput) bool {
	return e.PlaceID == in.PlaceID && e.Type == in.Type
}
func replayResult(e domain.Interaction, in RecordInput) (domain.Interaction, bool, error) {
	if !sameEvent(e, in) {
		return domain.Interaction{}, false, app.E(409, "IDEMPOTENCY_CONFLICT", "Event key already used for another interaction")
	}
	return e, true, nil
}

func (s *Service) Record(ctx context.Context, actor app.Actor, in RecordInput) (domain.Interaction, bool, error) {
	if err := checkActor(actor); err != nil {
		return domain.Interaction{}, false, err
	}
	if err := checkPlaceID(in.PlaceID); err != nil {
		return domain.Interaction{}, false, err
	}
	if !domain.ValidEventKey(in.EventKey) {
		return domain.Interaction{}, false, app.Validation("event_key", "invalid_uuid")
	}
	in.EventKey = strings.ToLower(in.EventKey)
	if in.Type != domain.View && in.Type != domain.Select {
		return domain.Interaction{}, false, app.Validation("type", "invalid_choice")
	}
	e, found, err := s.repo.FindEvent(ctx, actor.UserID, in.EventKey)
	if err != nil {
		return domain.Interaction{}, false, err
	}
	if found {
		return replayResult(e, in)
	}
	if _, err = s.places.GetActive(ctx, in.PlaceID); err != nil {
		return domain.Interaction{}, false, err
	}
	e = domain.Interaction{ID: shared.NewID(), UserID: actor.UserID, PlaceID: in.PlaceID, EventKey: in.EventKey, Type: in.Type, OccurredAt: s.now()}
	e, replayed, err := s.repo.InsertEvent(ctx, e)
	if err != nil {
		return domain.Interaction{}, false, err
	}
	if replayed {
		return replayResult(e, in)
	}
	return e, false, nil
}

func (s *Service) Favorites(ctx context.Context, actor app.Actor, p app.Pagination) (app.Page[Favorite], error) {
	if err := checkActor(actor); err != nil {
		return app.Page[Favorite]{}, err
	}
	if err := checkPage(p); err != nil {
		return app.Page[Favorite]{}, err
	}
	return s.repo.Favorites(ctx, actor.UserID, p)
}
func (s *Service) History(ctx context.Context, actor app.Actor, q HistoryQuery) (app.Page[domain.Interaction], error) {
	if err := checkActor(actor); err != nil {
		return app.Page[domain.Interaction]{}, err
	}
	if err := checkPage(q.Pagination); err != nil {
		return app.Page[domain.Interaction]{}, err
	}
	if q.Type != "" && !domain.ValidEventType(q.Type) {
		return app.Page[domain.Interaction]{}, app.Validation("type", "invalid_choice")
	}
	if q.PlaceID != "" {
		if err := checkPlaceID(q.PlaceID); err != nil {
			return app.Page[domain.Interaction]{}, err
		}
	}
	return s.repo.History(ctx, actor.UserID, q)
}
