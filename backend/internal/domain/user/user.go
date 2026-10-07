// Package user defines the user entity and credential rules.
package user

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

// Status of a user.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// User is an account owner (tenant).
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	DisplayName  string
	Status       Status
	CreatedAt    time.Time
}

// Password length bounds.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 128
)

// NormalizeEmail validates and lower-cases an email address.
func NormalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 {
		return "", errs.Validationf("invalid email address").WithField("email", "invalid email")
	}
	return e, nil
}

// ValidatePassword enforces the password policy.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen || n > MaxPasswordLen {
		return errs.Validationf("password must be %d-%d characters", MinPasswordLen, MaxPasswordLen).
			WithField("password", "invalid length")
	}
	return nil
}

// ValidateDisplayName trims and bounds a display name.
func ValidateDisplayName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if utf8.RuneCountInString(s) > 100 {
		return "", errs.Validationf("display_name too long").WithField("display_name", "max 100 characters")
	}
	return s, nil
}
