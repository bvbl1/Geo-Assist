package application

import (
	"context"
	"errors"
	"geo-assist/internal/identity/domain"
	app "geo-assist/internal/shared/application"
	shared "geo-assist/internal/shared/domain"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	users     map[shared.ID]domain.User
	sessions  map[string]domain.Session
	fail      error
	rotations int
}

func newFake() *fakeRepo {
	return &fakeRepo{users: map[shared.ID]domain.User{}, sessions: map[string]domain.Session{}}
}
func (r *fakeRepo) CreateUser(_ context.Context, u domain.User) error {
	if r.fail != nil {
		return r.fail
	}
	for _, v := range r.users {
		if v.Email == u.Email {
			return ErrEmailExists
		}
	}
	r.users[u.ID] = u
	return nil
}
func (r *fakeRepo) UserByEmail(_ context.Context, email string) (domain.User, error) {
	if r.fail != nil {
		return domain.User{}, r.fail
	}
	for _, u := range r.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, ErrNotFound
}
func (r *fakeRepo) UserByID(_ context.Context, id shared.ID) (domain.User, error) {
	if r.fail != nil {
		return domain.User{}, r.fail
	}
	u, ok := r.users[id]
	if !ok {
		return u, ErrNotFound
	}
	return u, nil
}
func (r *fakeRepo) UpdateProfile(_ context.Context, id shared.ID, in UpdateProfileInput, now time.Time) (domain.User, error) {
	u, ok := r.users[id]
	if !ok {
		return u, ErrNotFound
	}
	if in.DisplayName != nil {
		u.DisplayName = *in.DisplayName
	}
	if in.PreferredCategories != nil {
		u.PreferredCategories = *in.PreferredCategories
	}
	u.UpdatedAt = now
	r.users[id] = u
	return u, nil
}
func (r *fakeRepo) RotateSession(_ context.Context, old string, s domain.Session) error {
	if r.fail != nil {
		return r.fail
	}
	delete(r.sessions, old)
	r.sessions[s.TokenHash] = s
	r.rotations++
	return nil
}
func (r *fakeRepo) SessionByHash(_ context.Context, h string) (domain.Session, error) {
	if r.fail != nil {
		return domain.Session{}, r.fail
	}
	s, ok := r.sessions[h]
	if !ok {
		return s, ErrNotFound
	}
	return s, nil
}
func (r *fakeRepo) DeleteSession(_ context.Context, h string) error {
	if r.fail != nil {
		return r.fail
	}
	delete(r.sessions, h)
	return nil
}

type fakeHasher struct {
	compares int
	password string
}

func (h *fakeHasher) Hash(p string) (string, error) { h.password = p; return "hash:" + p, nil }
func (h *fakeHasher) Compare(hash, p string) bool   { h.compares++; return hash == "hash:"+p }

type fakeTokens struct{ n int }

func (t *fakeTokens) New() (string, error)     { t.n++; return strings.Repeat("a", t.n), nil }
func (t *fakeTokens) Hash(token string) string { return "digest:" + token }
func setup() (*Service, *fakeRepo, *fakeHasher) {
	r := newFake()
	h := &fakeHasher{}
	s := NewService(r, h, &fakeTokens{})
	s.now = func() time.Time { return time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC) }
	return s, r, h
}
func input() RegisterInput {
	return RegisterInput{Email: " Student@Example.com ", Password: " password-2026 ", DisplayName: " Student ", PreferredCategories: []string{"cafe"}}
}
func code(err error) string {
	var e *app.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func TestRegisterNormalizesIdentityButNotPassword(t *testing.T) {
	s, r, h := setup()
	u, err := s.Register(context.Background(), input())
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "student@example.com" || u.DisplayName != "Student" || u.Role != "user" || h.password != " password-2026 " {
		t.Fatalf("unexpected user: %+v", u)
	}
	if len(r.sessions) != 0 {
		t.Fatal("register must not create session")
	}
	_, err = s.Register(context.Background(), input())
	if code(err) != "EMAIL_ALREADY_EXISTS" {
		t.Fatalf("duplicate: %v", err)
	}
}
func TestRegisterRejectsInvalidAndMultibytePassword(t *testing.T) {
	cases := []RegisterInput{input(), input(), input(), input()}
	cases[0].Password = strings.Repeat("\u044f", 37)
	cases[1].Email = "Student <student@example.com>"
	cases[2].PreferredCategories = []string{"cafe", "cafe"}
	cases[3].DisplayName = "   "
	for i, in := range cases {
		s, _, _ := setup()
		if _, err := s.Register(context.Background(), in); code(err) != "VALIDATION_ERROR" {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
func TestLoginRotationAndExpiry(t *testing.T) {
	s, r, _ := setup()
	ctx := context.Background()
	u, err := s.Register(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := s.Login(ctx, u.Email, input().Password, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, first); err != nil {
		t.Fatal(err)
	}
	_, second, err := s.Login(ctx, u.Email, input().Password, first)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(r.sessions) != 1 {
		t.Fatal("session must rotate")
	}
	if _, err = s.Authenticate(ctx, first); code(err) != "UNAUTHENTICATED" {
		t.Fatal("old token accepted")
	}
	expires := r.sessions[s.tokens.Hash(second)].ExpiresAt
	s.now = func() time.Time { return expires }
	if _, err = s.Authenticate(ctx, second); code(err) != "UNAUTHENTICATED" {
		t.Fatal("expired session accepted before TTL cleanup")
	}
}
func TestCredentialsErrorAndStorageFailureAreDistinct(t *testing.T) {
	s, r, h := setup()
	ctx := context.Background()
	u, _ := s.Register(ctx, input())
	_, _, wrong := s.Login(ctx, u.Email, "bad password", "")
	_, _, missing := s.Login(ctx, "missing@example.com", "bad password", "")
	if code(wrong) != "INVALID_CREDENTIALS" || code(missing) != code(wrong) || h.compares != 2 {
		t.Fatal("must use same credential error and perform comparison")
	}
	r.fail = errors.New("storage unavailable")
	_, _, err := s.Login(ctx, u.Email, input().Password, "")
	if !errors.Is(err, r.fail) {
		t.Fatal("storage failure misclassified")
	}
}
func TestProfilePatchPreservesUnmentionedFields(t *testing.T) {
	s, _, _ := setup()
	ctx := context.Background()
	u, _ := s.Register(ctx, input())
	name := " New Name "
	updated, err := s.UpdateProfile(ctx, app.Actor{UserID: u.ID, Role: "user"}, UpdateProfileInput{DisplayName: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "New Name" || updated.Email != u.Email || len(updated.PreferredCategories) != 1 {
		t.Fatal("partial update lost fields")
	}
	empty := []string{}
	updated, err = s.UpdateProfile(ctx, app.Actor{UserID: u.ID}, UpdateProfileInput{PreferredCategories: &empty})
	if err != nil || len(updated.PreferredCategories) != 0 {
		t.Fatal("cannot clear categories")
	}
	_, err = s.UpdateProfile(ctx, app.Actor{UserID: u.ID}, UpdateProfileInput{})
	if code(err) != "VALIDATION_ERROR" {
		t.Fatal("empty patch accepted")
	}
}
func TestLogoutRevokesSessionAndIsIdempotent(t *testing.T) {
	s, _, _ := setup()
	ctx := context.Background()
	u, _ := s.Register(ctx, input())
	_, token, _ := s.Login(ctx, u.Email, input().Password, "")
	if err := s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, token); code(err) != "UNAUTHENTICATED" {
		t.Fatal("logout did not revoke")
	}
	if err := s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
}
func TestDummyHashHasConfiguredBcryptCost(t *testing.T) {
	s, _, _ := setup()
	cost, err := bcrypt.Cost([]byte(s.dummyHash))
	if err != nil || cost != 12 {
		t.Fatalf("invalid timing fallback: cost=%d err=%v", cost, err)
	}
}
