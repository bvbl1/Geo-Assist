package application

import (
	"context"
	"errors"
	"geo-assist/internal/identity/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNotFound = errors.New("identity not found")
var ErrEmailExists = errors.New("email already exists")

const SessionTTL = 7 * 24 * time.Hour

type Repository interface {
	CreateUser(context.Context, domain.User) error
	UserByEmail(context.Context, string) (domain.User, error)
	UserByID(context.Context, shared.ID) (domain.User, error)
	UpdateProfile(context.Context, shared.ID, UpdateProfileInput, time.Time) (domain.User, error)
	RotateSession(context.Context, string, domain.Session) error
	SessionByHash(context.Context, string) (domain.Session, error)
	DeleteSession(context.Context, string) error
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Compare(string, string) bool
}
type TokenGenerator interface {
	New() (string, error)
	Hash(string) string
}
type Service struct {
	repo      Repository
	hasher    PasswordHasher
	tokens    TokenGenerator
	now       func() time.Time
	dummyHash string
}

func NewService(repo Repository, hasher PasswordHasher, tokens TokenGenerator) *Service {
	// Precomputed valid bcrypt cost-12 hash: used solely to equalize unknown-account checks.
	return &Service{repo: repo, hasher: hasher, tokens: tokens, now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }, dummyHash: "$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"}
}

type RegisterInput struct {
	Email, Password, DisplayName string
	PreferredCategories          []string
}
type UpdateProfileInput struct {
	DisplayName         *string
	PreferredCategories *[]string
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
func validateCategories(values []string) error {
	if len(values) > 5 {
		return app.Validation("preferred_categories", "too_many")
	}
	seen := map[string]bool{}
	for _, v := range values {
		switch v {
		case "cafe", "park", "library", "coworking", "attraction":
		default:
			return app.Validation("preferred_categories", "invalid_category")
		}
		if seen[v] {
			return app.Validation("preferred_categories", "duplicate")
		}
		seen[v] = true
	}
	return nil
}
func validName(name string) bool {
	return utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 80
}
func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	return s.create(ctx, in, "user")
}

// CreateAdmin is a local provisioning use case, never exposed by the public HTTP API.
func (s *Service) CreateAdmin(ctx context.Context, in RegisterInput) (domain.User, error) {
	return s.create(ctx, in, "admin")
}
func (s *Service) create(ctx context.Context, in RegisterInput, role string) (domain.User, error) {
	in.Email = NormalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) < 3 || len(in.Email) > 254 {
		return domain.User{}, app.Validation("email", "invalid_email")
	}
	if len(in.Password) < 12 || len(in.Password) > 72 {
		return domain.User{}, app.Validation("password", "length_12_to_72_bytes")
	}
	if !validName(in.DisplayName) {
		return domain.User{}, app.Validation("display_name", "length_1_to_80")
	}
	if err := validateCategories(in.PreferredCategories); err != nil {
		return domain.User{}, err
	}
	if in.PreferredCategories == nil {
		in.PreferredCategories = []string{}
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	now := s.now()
	u := domain.User{ID: shared.NewID(), Email: in.Email, PasswordHash: hash, DisplayName: in.DisplayName, Role: role, PreferredCategories: in.PreferredCategories, CreatedAt: now, UpdatedAt: now}
	if err = s.repo.CreateUser(ctx, u); errors.Is(err, ErrEmailExists) {
		return domain.User{}, app.E(409, "EMAIL_ALREADY_EXISTS", "Email is already registered")
	}
	return u, err
}
func (s *Service) Login(ctx context.Context, email, password, oldToken string) (domain.User, string, error) {
	// Avoid invoking bcrypt with oversized input while retaining generic credentials errors.
	u, err := s.repo.UserByEmail(ctx, NormalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		s.hasher.Compare(s.dummyHash, password)
		return domain.User{}, "", app.E(401, "INVALID_CREDENTIALS", "Invalid email or password")
	}
	if err != nil {
		return domain.User{}, "", err
	}
	if len(password) > 72 || !s.hasher.Compare(u.PasswordHash, password) {
		return domain.User{}, "", app.E(401, "INVALID_CREDENTIALS", "Invalid email or password")
	}
	token, err := s.tokens.New()
	if err != nil {
		return domain.User{}, "", err
	}
	now := s.now()
	session := domain.Session{ID: shared.NewID(), UserID: u.ID, TokenHash: s.tokens.Hash(token), CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}
	oldHash := ""
	if oldToken != "" {
		oldHash = s.tokens.Hash(oldToken)
	}
	if err = s.repo.RotateSession(ctx, oldHash, session); err != nil {
		return domain.User{}, "", err
	}
	return u, token, nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (app.Actor, error) {
	unauth := app.E(401, "UNAUTHENTICATED", "Authentication required")
	if token == "" || len(token) > 128 {
		return app.Actor{}, unauth
	}
	session, err := s.repo.SessionByHash(ctx, s.tokens.Hash(token))
	if errors.Is(err, ErrNotFound) {
		return app.Actor{}, unauth
	}
	if err != nil {
		return app.Actor{}, err
	}
	if !session.ExpiresAt.After(s.now()) {
		return app.Actor{}, unauth
	}
	u, err := s.repo.UserByID(ctx, session.UserID)
	if errors.Is(err, ErrNotFound) {
		return app.Actor{}, unauth
	}
	if err != nil {
		return app.Actor{}, err
	}
	return app.Actor{UserID: u.ID, Role: u.Role}, nil
}
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, s.tokens.Hash(token))
}
func (s *Service) GetProfile(ctx context.Context, actor app.Actor) (domain.User, error) {
	if !shared.ValidID(string(actor.UserID)) {
		return domain.User{}, app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	u, err := s.repo.UserByID(ctx, actor.UserID)
	if errors.Is(err, ErrNotFound) {
		return domain.User{}, app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	return u, err
}
func (s *Service) UpdateProfile(ctx context.Context, actor app.Actor, in UpdateProfileInput) (domain.User, error) {
	if !shared.ValidID(string(actor.UserID)) {
		return domain.User{}, app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	if in.DisplayName == nil && in.PreferredCategories == nil {
		return domain.User{}, app.Validation("body", "at_least_one_field_required")
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if !validName(name) {
			return domain.User{}, app.Validation("display_name", "length_1_to_80")
		}
		in.DisplayName = &name
	}
	if in.PreferredCategories != nil {
		if err := validateCategories(*in.PreferredCategories); err != nil {
			return domain.User{}, err
		}
	}
	u, err := s.repo.UpdateProfile(ctx, actor.UserID, in, s.now())
	if errors.Is(err, ErrNotFound) {
		return domain.User{}, app.E(401, "UNAUTHENTICATED", "Authentication required")
	}
	return u, err
}
