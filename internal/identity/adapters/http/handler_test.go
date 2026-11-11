package httpadapter

import (
	"encoding/json"
	"geo-assist/internal/identity/domain"
	"strings"
	"testing"
	"time"
)

func TestLimiterResetsAndSeparatesKeys(t *testing.T) {
	now := time.Now()
	l := &limiter{entries: map[string]bucket{}, now: func() time.Time { return now }}
	if l.take("a", 1, time.Minute) != 0 || l.take("a", 1, time.Minute) <= 0 || l.take("b", 1, time.Minute) != 0 {
		t.Fatal("limit not applied per key")
	}
	now = now.Add(time.Minute)
	if l.take("a", 1, time.Minute) != 0 {
		t.Fatal("window did not reset")
	}
}
func TestResponseOmitsCredentialsAndUsesArray(t *testing.T) {
	u := domain.User{Email: "a@example.com", PasswordHash: "secret-hash"}
	b, err := json.Marshal(response(u))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "hash") || strings.Contains(s, "Password") || !strings.Contains(s, `"preferred_categories":[]`) {
		t.Fatalf("unsafe response: %s", s)
	}
}
