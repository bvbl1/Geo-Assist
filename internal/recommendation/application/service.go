package application

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	catalog "geo-assist/internal/catalog/application"
	place "geo-assist/internal/catalog/domain"
	"geo-assist/internal/recommendation/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
)

type Attempt struct {
	ID               string     `json:"id" bson:"_id"`
	SchemaVersion    int32      `json:"-" bson:"schema_version"`
	Algorithm        string     `json:"algorithm" bson:"algorithm"`
	AlgorithmVersion int32      `json:"algorithm_version" bson:"algorithm_version"`
	Builder          string     `json:"builder_impl" bson:"builder_impl"`
	Status           string     `json:"status" bson:"status"`
	Trigger          string     `json:"trigger" bson:"trigger"`
	StartedAt        time.Time  `json:"started_at" bson:"started_at"`
	FinishedAt       *time.Time `json:"finished_at" bson:"finished_at"`
	BuiltAt          *time.Time `json:"built_at" bson:"built_at"`
	SourceStartedAt  time.Time  `json:"source_read_started_at" bson:"source_read_started_at"`
	SourceFinishedAt time.Time  `json:"source_read_finished_at" bson:"source_read_finished_at"`
	Fingerprint      string     `json:"source_fingerprint" bson:"source_fingerprint"`
	ItemCount        int        `json:"item_count" bson:"item_count"`
	EdgeCount        int        `json:"edge_count" bson:"edge_count"`
	PairCount        int        `json:"pair_count" bson:"pair_count"`
	ModelBytes       int        `json:"model_bytes" bson:"model_bytes"`
	DurationMS       int64      `json:"duration_ms" bson:"duration_ms"`
	Checksum         string     `json:"content_checksum" bson:"content_checksum"`
	ReusedModelID    string     `json:"reused_model_id,omitempty" bson:"reused_model_id"`
	Error            string     `json:"error_summary,omitempty" bson:"error_summary"`
}
type Meta struct {
	ActiveID    string     `bson:"active_model_id"`
	LastID      string     `bson:"last_attempt_id"`
	CheckedAt   *time.Time `bson:"last_checked_at"`
	VerifiedID  string     `bson:"verified_model_id"`
	NextAttempt *time.Time `bson:"next_attempt_at"`
	ManualAt    *time.Time `bson:"last_manual_started_at"`
	Lease       struct {
		Owner   string    `bson:"owner"`
		Expires time.Time `bson:"expires_at"`
	} `bson:"lease"`
}
type Loaded struct {
	Model   *domain.Model
	Attempt Attempt
}
type Repository interface {
	Init(context.Context) error
	Cleanup(context.Context, string) error
	Meta(context.Context) (Meta, error)
	Attempt(context.Context, string) (*Attempt, error)
	Acquire(context.Context, Attempt) error
	Renew(context.Context, string) error
	Snapshot(context.Context) (domain.Source, error)
	Save(context.Context, *Attempt, *domain.Model) error
	Finish(context.Context, Attempt, bool) error
	Fail(context.Context, Attempt) error
	Load(context.Context, string) (*Loaded, error)
	Candidates(context.Context, catalog.SearchQuery) ([]place.PlaceHit, error)
	Favorites(context.Context, shared.ID) ([]string, error)
	Popularity(context.Context) (map[string]float64, error)
}
type Config struct {
	CheckInterval, PollInterval, BuildTimeout, SourceTimeout time.Duration
	AutoRebuild                                              bool
}

func DefaultConfig() Config {
	return Config{5 * time.Minute, 5 * time.Second, 120 * time.Second, 20 * time.Second, true}
}

type Service struct {
	repo      Repository
	cfg       Config
	loaded    atomic.Pointer[Loaded]
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	refreshMu sync.Mutex
	closed    bool
}

func New(repo Repository, cfg Config) *Service { return &Service{repo: repo, cfg: cfg} }
func (s *Service) Start(ctx context.Context) error {
	if err := s.repo.Init(ctx); err != nil {
		return err
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	if err := s.Refresh(s.ctx); err != nil {
		slog.Warn("recommendation load failed", "error", err)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		poll := time.NewTicker(s.cfg.PollInterval)
		defer poll.Stop()
		check := time.NewTicker(s.cfg.CheckInterval)
		defer check.Stop()
		if s.cfg.AutoRebuild {
			_, _ = s.RequestBuild("startup", false)
		}
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-poll.C:
				if err := s.Refresh(s.ctx); err != nil {
					slog.Warn("recommendation refresh failed", "error", err)
				}
				if s.cfg.AutoRebuild {
					m, e := s.repo.Meta(s.ctx)
					if e == nil && ((m.NextAttempt != nil && !m.NextAttempt.After(time.Now())) || (m.Lease.Owner != "" && !m.Lease.Expires.After(time.Now()))) {
						_, _ = s.RequestBuild("scheduled", false)
					}
				}
			case <-check.C:
				if s.cfg.AutoRebuild {
					_, _ = s.RequestBuild("scheduled", false)
				}
			}
		}
	}()
	return nil
}
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
func (s *Service) Refresh(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	m, err := s.repo.Meta(ctx)
	if err != nil {
		return err
	}
	old := s.loaded.Load()
	if m.ActiveID == "" || old != nil && old.Attempt.ID == m.ActiveID {
		return nil
	}
	next, err := s.repo.Load(ctx, m.ActiveID)
	if err == nil {
		s.loaded.Store(next)
	}
	return err
}
func (s *Service) RequestBuild(trigger string, force bool) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx == nil || s.ctx.Err() != nil {
		return Attempt{}, app.E(503, "MODEL_UNAVAILABLE", "Recommendation worker unavailable")
	}
	a := Attempt{ID: string(shared.NewID()), SchemaVersion: 1, Algorithm: domain.Algorithm, AlgorithmVersion: 1, Builder: "inverted", Status: "building", Trigger: trigger, StartedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.Acquire(ctx, a); err != nil {
		return Attempt{}, err
	}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.build(a, force) }()
	return a, nil
}
func (s *Service) build(a Attempt, force bool) {
	ctx, cancel := context.WithTimeout(s.ctx, s.cfg.BuildTimeout)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.repo.Renew(ctx, a.ID); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	err := func() error {
		if err := s.repo.Cleanup(ctx, a.ID); err != nil {
			return err
		}
		a.SourceStartedAt = time.Now().UTC()
		sourceCtx, stop := context.WithTimeout(ctx, s.cfg.SourceTimeout)
		src, err := s.repo.Snapshot(sourceCtx)
		stop()
		if err != nil {
			return err
		}
		a.SourceFinishedAt = time.Now().UTC()
		a.Fingerprint = src.Fingerprint()
		a.ItemCount = len(src.Places)
		a.EdgeCount = len(src.Edges)
		meta, err := s.repo.Meta(ctx)
		if err != nil {
			return err
		}
		if meta.ActiveID != "" && !force {
			active, err := s.repo.Attempt(ctx, meta.ActiveID)
			if err != nil {
				return err
			}
			if active != nil && active.Fingerprint == a.Fingerprint {
				if _, loadErr := s.repo.Load(ctx, meta.ActiveID); loadErr == nil {
					a.Status = "skipped"
					a.ReusedModelID = meta.ActiveID
					return s.repo.Finish(ctx, a, false)
				}
			}
		}
		model, err := domain.Build(ctx, src)
		if err != nil {
			return err
		}
		a.PairCount = model.PairCount
		if err = s.repo.Save(ctx, &a, model); err != nil {
			return err
		}
		a.Status = "ready"
		if err = s.repo.Finish(ctx, a, true); err != nil {
			return err
		}
		slog.Info("recommendation model published", "model_id", a.ID, "items", a.ItemCount, "pairs", a.PairCount)
		return s.Refresh(ctx)
	}()
	if err != nil {
		cleanup, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		a.Status = "failed"
		a.Error = "Build failed; previous model remains active. See server logs."
		_ = s.repo.Fail(cleanup, a)
		slog.Error("recommendation build failed", "build_id", a.ID, "error", err)
	}
}

type Result struct {
	Place  place.PlaceHit
	Score  float64
	Reason string
}
type ResultMeta struct {
	Algorithm  string     `json:"algorithm"`
	ModelID    string     `json:"model_id"`
	BuiltAt    *time.Time `json:"model_built_at"`
	CheckedAt  *time.Time `json:"source_checked_at"`
	Refreshing bool       `json:"refresh_in_progress"`
	Fallback   bool       `json:"fallback_used"`
}

func (s *Service) Recommend(ctx context.Context, user shared.ID, q catalog.SearchQuery, limit int) ([]Result, ResultMeta, error) {
	model := s.loaded.Load()
	if model == nil {
		return nil, ResultMeta{}, app.E(503, "MODEL_NOT_READY", "Recommendation model is being prepared")
	}
	meta, err := s.repo.Meta(ctx)
	if err != nil {
		return nil, ResultMeta{}, err
	}
	info := ResultMeta{Algorithm: domain.Algorithm, ModelID: model.Attempt.ID, BuiltAt: model.Attempt.BuiltAt, Refreshing: meta.Lease.Owner != "" && meta.Lease.Expires.After(time.Now())}
	if meta.VerifiedID == model.Attempt.ID {
		info.CheckedAt = meta.CheckedAt
	}
	favs, err := s.repo.Favorites(ctx, user)
	if err != nil {
		return nil, info, err
	}
	hits, err := s.repo.Candidates(ctx, q)
	if err != nil {
		return nil, info, err
	}
	excluded := map[string]bool{}
	for _, id := range favs {
		excluded[id] = true
	}
	rank := func(scores map[string]float64, reason string) []Result {
		results := []Result{}
		for _, hit := range hits {
			id := string(hit.Place.ID)
			if excluded[id] || scores[id] <= 0 {
				continue
			}
			results = append(results, Result{hit, scores[id], reason})
		}
		sort.Slice(results, func(i, j int) bool {
			a, b := results[i], results[j]
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			if a.Place.DistanceM != nil && b.Place.DistanceM != nil && *a.Place.DistanceM != *b.Place.DistanceM {
				return *a.Place.DistanceM < *b.Place.DistanceM
			}
			return a.Place.Place.ID < b.Place.Place.ID
		})
		if len(results) > limit {
			results = results[:limit]
		}
		return results
	}
	results := rank(model.Model.Scores(favs), "similar_to_favorites")
	if len(results) == 0 {
		scores, err := s.repo.Popularity(ctx)
		if err != nil {
			return nil, info, err
		}
		reason := "popular"
		if q.Geo != nil {
			reason = "popular_nearby"
		}
		results = rank(scores, reason)
		info.Fallback = true
		info.Algorithm = "popularity"
	}
	return results, info, nil
}
func (s *Service) Status(ctx context.Context, id string) (map[string]any, error) {
	m, err := s.repo.Meta(ctx)
	if err != nil {
		return nil, err
	}
	active, err := s.repo.Attempt(ctx, m.ActiveID)
	if err != nil {
		return nil, err
	}
	last, err := s.repo.Attempt(ctx, m.LastID)
	if err != nil {
		return nil, err
	}
	requested, err := s.repo.Attempt(ctx, id)
	if err != nil {
		return nil, err
	}
	loadedID := ""
	if l := s.loaded.Load(); l != nil {
		loadedID = l.Attempt.ID
	}
	var next *time.Time
	if m.CheckedAt != nil && s.cfg.AutoRebuild {
		v := m.CheckedAt.Add(s.cfg.CheckInterval)
		next = &v
	}
	if m.NextAttempt != nil {
		next = m.NextAttempt
	}
	return map[string]any{"active": active, "loaded_model_id": loadedID, "last_attempt": last, "requested_attempt": requested, "last_checked_at": m.CheckedAt, "next_check_at": next}, nil
}
