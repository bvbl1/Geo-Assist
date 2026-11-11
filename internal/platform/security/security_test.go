package security

import (
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestPasswordHashAndVerification(t *testing.T) {
	h := BcryptHasher{}
	hash, err := h.Hash("demo-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	cost, _ := bcrypt.Cost([]byte(hash))
	if cost != 12 || hash == "demo-password-2026" || !h.Compare(hash, "demo-password-2026") || h.Compare(hash, "wrong-password") {
		t.Fatal("password hash contract violated")
	}
	if _, err = h.Hash(strings.Repeat("x", 73)); err == nil {
		t.Fatal("oversized password accepted")
	}
}
func TestTokensAreRandomAndStoredAsDigest(t *testing.T) {
	g := CryptoTokens{}
	a, err := g.New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.New()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 || a == b || len(g.Hash(a)) != 64 || g.Hash(a) == a || g.Hash(a) != g.Hash(a) {
		t.Fatal("token contract violated")
	}
}
