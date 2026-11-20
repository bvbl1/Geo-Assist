package application

import (
	"context"
	catalog "geo-assist/internal/catalog/application"
	place "geo-assist/internal/catalog/domain"
	"geo-assist/internal/recommendation/domain"
	shared "geo-assist/internal/shared/domain"
	"testing"
)

type fixtureRepo struct {
	Repository
	favs     []string
	hits     []place.PlaceHit
	popular  map[string]float64
	popCalls int
}

func (r *fixtureRepo) Meta(context.Context) (Meta, error)                     { return Meta{}, nil }
func (r *fixtureRepo) Favorites(context.Context, shared.ID) ([]string, error) { return r.favs, nil }
func (r *fixtureRepo) Candidates(context.Context, catalog.SearchQuery) ([]place.PlaceHit, error) {
	return r.hits, nil
}
func (r *fixtureRepo) Popularity(context.Context) (map[string]float64, error) {
	r.popCalls++
	return r.popular, nil
}
func hit(id string) place.PlaceHit { return place.PlaceHit{Place: place.Place{ID: shared.ID(id)}} }
func TestFilterBeforeLimitAndLiveFavorites(t *testing.T) {
	m, _ := domain.Build(context.Background(), domain.Source{Places: []string{"A", "B", "C", "D"}, Edges: []domain.Edge{{User: "1", Place: "A"}, {User: "1", Place: "B"}, {User: "2", Place: "A"}, {User: "2", Place: "C"}, {User: "3", Place: "A"}, {User: "3", Place: "C"}, {User: "4", Place: "B"}, {User: "4", Place: "D"}}})
	repo := &fixtureRepo{favs: []string{"A", "B"}, hits: []place.PlaceHit{hit("D")}}
	s := New(repo, DefaultConfig())
	s.loaded.Store(&Loaded{Model: m, Attempt: Attempt{ID: "version1"}})
	results, meta, err := s.Recommend(context.Background(), "u", catalog.SearchQuery{}, 1)
	if err != nil || len(results) != 1 || results[0].Place.Place.ID != "D" || meta.Fallback {
		t.Fatalf("filtered recommendation: %v %v", results, err)
	}
	// Adding a favorite excludes it immediately, without a rebuild.
	repo.favs = append(repo.favs, "D")
	repo.popular = map[string]float64{"D": 10}
	results, meta, err = s.Recommend(context.Background(), "u", catalog.SearchQuery{}, 10)
	if err != nil || len(results) != 0 || !meta.Fallback {
		t.Fatal("favorite exclusion / empty fallback failed")
	}
	repo.favs = nil
	repo.hits = []place.PlaceHit{hit("C"), hit("D")}
	repo.popular = map[string]float64{"C": 2, "D": 5}
	results, meta, err = s.Recommend(context.Background(), "u", catalog.SearchQuery{}, 1)
	if err != nil || !meta.Fallback || results[0].Place.Place.ID != "D" {
		t.Fatal("cold start popularity failed")
	}
}
func TestNotReady(t *testing.T) {
	s := New(&fixtureRepo{}, DefaultConfig())
	if _, _, err := s.Recommend(context.Background(), "u", catalog.SearchQuery{}, 10); err == nil {
		t.Fatal("expected unavailable")
	}
}
func TestDeterministicTieAndNoFallbackMix(t *testing.T) {
	near, far := 10.0, 20.0
	repo := &fixtureRepo{favs: []string{"A"}, hits: []place.PlaceHit{{Place: place.Place{ID: "C"}, DistanceM: &far}, {Place: place.Place{ID: "B"}, DistanceM: &near}, hit("D")}, popular: map[string]float64{"D": 100}}
	s := New(repo, DefaultConfig())
	s.loaded.Store(&Loaded{Model: &domain.Model{Items: map[string]domain.Item{"A": {Neighbors: []domain.Neighbor{{PlaceID: "C", Similarity: .5}, {PlaceID: "B", Similarity: .5}}}}}})
	items, _, err := s.Recommend(context.Background(), "u", catalog.SearchQuery{}, 10)
	if err != nil || len(items) != 2 || items[0].Place.Place.ID != "B" || repo.popCalls != 0 {
		t.Fatal("CF should not mix popularity; distance tie-break failed")
	}
}
