package domain

import (
	shared "geo-assist/internal/shared/domain"
	"time"
)

type User struct {
	ID                  shared.ID
	Email               string
	PasswordHash        string
	DisplayName         string
	Role                string
	PreferredCategories []string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Session struct {
	ID        shared.ID
	UserID    shared.ID
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}
