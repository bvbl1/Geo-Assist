package domain

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	shared "geo-assist/internal/shared/domain"
)

const (
	View            = "view"
	Select          = "select"
	FavoriteAdded   = "favorite_added"
	FavoriteRemoved = "favorite_removed"
	RatingSet       = "rating_set"
	RatingRemoved   = "rating_removed"
)

type UserPlace struct {
	ID, UserID, PlaceID shared.ID
	Favorite            bool
	FavoritedAt         *time.Time
	Rating              *int
	CreatedAt           time.Time
	UpdatedAt           *time.Time
}

type Interaction struct {
	ID, UserID, PlaceID    shared.ID
	EventKey, Type         string
	PreviousRating, Rating *int
	OccurredAt             time.Time
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidEventKey(s string) bool { return uuidPattern.MatchString(s) }

func ValidEventType(s string) bool {
	switch s {
	case View, Select, FavoriteAdded, FavoriteRemoved, RatingSet, RatingRemoved:
		return true
	}
	return false
}

func NewEventKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func CopyRating(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
