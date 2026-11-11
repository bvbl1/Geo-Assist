package application

import "geo-assist/internal/shared/domain"

type Actor struct {
	UserID domain.ID
	Role   string
}
type Detail struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
type Error struct {
	Status  int      `json:"-"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []Detail `json:"details"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func E(status int, code, message string) error {
	return &Error{Status: status, Code: code, Message: message, Details: []Detail{}}
}
func Validation(field, reason string) error {
	return &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "Request validation failed", Details: []Detail{{Field: field, Reason: reason}}}
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
type Page[T any] struct {
	Items   []T
	HasMore bool
}
