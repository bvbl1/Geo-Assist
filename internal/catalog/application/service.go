package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"geo-assist/internal/catalog/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
)

type GeoQuery struct {
	Center  domain.Coordinates
	RadiusM int
}

type SearchQuery struct {
	NamePrefix string
	Category   domain.Category
	Tags       []string
	Geo        *GeoQuery
	Pagination app.Pagination
}

type CreateCommand struct {
	Name, Description, Address string
	Category                   domain.Category
	Tags                       []string
	Location                   domain.Coordinates
}

type UpdateCommand struct {
	ExpectedVersion            int64
	Name, Description, Address *string
	Category                   *domain.Category
	Tags                       *[]string
	Location                   *domain.Coordinates
}

type Repository interface {
	GetAny(context.Context, shared.ID) (domain.Place, error)
	Create(context.Context, domain.Place) error
	Update(context.Context, domain.Place, int64) error
	SoftDelete(context.Context, shared.ID, int64, time.Time) error
	Search(context.Context, SearchQuery) (app.Page[domain.PlaceHit], error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }}
}

func fieldError(err error) error {
	var field *domain.FieldError
	if errors.As(err, &field) {
		return app.Validation(field.Field, field.Reason)
	}
	return err
}

func checkID(id shared.ID) error {
	if !shared.ValidID(string(id)) {
		return app.E(400, "BAD_REQUEST", "Invalid place ID")
	}
	return nil
}

func requireAdmin(actor app.Actor) error {
	if !shared.ValidID(string(actor.UserID)) {
		return app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	if actor.Role != "admin" {
		return app.E(403, "FORBIDDEN", "Administrator role required")
	}
	return nil
}

func (s *Service) GetAny(ctx context.Context, id shared.ID) (domain.Place, error) {
	if err := checkID(id); err != nil {
		return domain.Place{}, err
	}
	return s.repo.GetAny(ctx, id)
}

func (s *Service) GetActive(ctx context.Context, id shared.ID) (domain.Place, error) {
	p, err := s.GetAny(ctx, id)
	if err != nil {
		return domain.Place{}, err
	}
	if p.DeletedAt != nil {
		return domain.Place{}, app.E(404, "NOT_FOUND", "Place not found")
	}
	return p, nil
}

func (s *Service) Create(ctx context.Context, actor app.Actor, cmd CreateCommand) (domain.Place, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Place{}, err
	}
	now := s.now()
	p := domain.Place{ID: shared.NewID(), Name: cmd.Name, Description: cmd.Description, Address: cmd.Address,
		Category: cmd.Category, Tags: cmd.Tags, Location: cmd.Location, CreatedBy: actor.UserID,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := p.NormalizeAndValidate(); err != nil {
		return domain.Place{}, fieldError(err)
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return domain.Place{}, err
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, actor app.Actor, id shared.ID, cmd UpdateCommand) (domain.Place, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Place{}, err
	}
	if cmd.ExpectedVersion < 1 {
		return domain.Place{}, app.Validation("expected_version", "out_of_range")
	}
	if cmd.Name == nil && cmd.Description == nil && cmd.Address == nil && cmd.Category == nil && cmd.Tags == nil && cmd.Location == nil {
		return domain.Place{}, app.Validation("body", "empty_patch")
	}
	p, err := s.GetActive(ctx, id)
	if err != nil {
		return domain.Place{}, err
	}
	if p.Version != cmd.ExpectedVersion {
		return domain.Place{}, app.E(409, "VERSION_CONFLICT", "Place was modified; reload it before retrying")
	}
	if cmd.Name != nil {
		p.Name = *cmd.Name
	}
	if cmd.Description != nil {
		p.Description = *cmd.Description
	}
	if cmd.Address != nil {
		p.Address = *cmd.Address
	}
	if cmd.Category != nil {
		p.Category = *cmd.Category
	}
	if cmd.Tags != nil {
		p.Tags = *cmd.Tags
	}
	if cmd.Location != nil {
		p.Location = *cmd.Location
	}
	if err := p.NormalizeAndValidate(); err != nil {
		return domain.Place{}, fieldError(err)
	}
	p.Version++
	p.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, p, cmd.ExpectedVersion); err != nil {
		return domain.Place{}, err
	}
	return p, nil
}

func (s *Service) Delete(ctx context.Context, actor app.Actor, id shared.ID, version int64) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if err := checkID(id); err != nil {
		return err
	}
	if version < 1 {
		return app.Validation("expected_version", "out_of_range")
	}
	return s.repo.SoftDelete(ctx, id, version, s.now())
}

func (s *Service) Search(ctx context.Context, query SearchQuery) (app.Page[domain.PlaceHit], error) {
	if query.Pagination.Limit < 1 || query.Pagination.Limit > 100 {
		return app.Page[domain.PlaceHit]{}, app.Validation("limit", "out_of_range")
	}
	if query.Pagination.Offset < 0 || query.Pagination.Offset > 10000 {
		return app.Page[domain.PlaceHit]{}, app.Validation("offset", "out_of_range")
	}
	if query.NamePrefix != "" {
		if strings.TrimSpace(query.NamePrefix) == "" || utf8.RuneCountInString(query.NamePrefix) > 120 {
			return app.Page[domain.PlaceHit]{}, app.Validation("q", "invalid_length")
		}
		query.NamePrefix = domain.NormalizeName(query.NamePrefix)
	}
	if query.Category != "" && !domain.ValidCategory(query.Category) {
		return app.Page[domain.PlaceHit]{}, app.Validation("category", "invalid_category")
	}
	tags, err := domain.NormalizeTags(query.Tags)
	if err != nil {
		return app.Page[domain.PlaceHit]{}, fieldError(err)
	}
	query.Tags = tags
	if query.Geo != nil {
		if err := domain.ValidateCoordinates(query.Geo.Center); err != nil {
			return app.Page[domain.PlaceHit]{}, fieldError(err)
		}
		if query.Geo.RadiusM < 1 || query.Geo.RadiusM > 50000 {
			return app.Page[domain.PlaceHit]{}, app.Validation("radius_m", "out_of_range")
		}
	}
	return s.repo.Search(ctx, query)
}
