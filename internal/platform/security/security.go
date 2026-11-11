package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"golang.org/x/crypto/bcrypt"
)

type BcryptHasher struct{}

func (BcryptHasher) Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(h), err
}
func (BcryptHasher) Compare(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type CryptoTokens struct{}

func (CryptoTokens) New() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (CryptoTokens) Hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
