package domain

import (
	"crypto/rand"
	"encoding/hex"
)

type ID string

func NewID() ID {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return ID(hex.EncodeToString(b[:]))
}

func ValidID(s string) bool {
	if len(s) != 24 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
