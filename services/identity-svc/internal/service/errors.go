package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrEmailTaken is returned when the email is already registered.
var ErrEmailTaken = errors.New("service: email already registered")

// ValidationError is a caller-correctable input problem whose message is safe
// to show to the client.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// validateUsername enforces the shared username rules: at least 2 characters,
// only alphanumerics, hyphens and underscores. It returns the trimmed username.
func validateUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if username == "" {
		return "", validationErrorf("username is required")
	}
	if len(username) < 2 {
		return "", validationErrorf("username must be at least 2 characters")
	}
	for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return "", validationErrorf("username may only contain alphanumeric characters, hyphens, and underscores")
		}
	}
	return username, nil
}

// isUniqueViolation reports whether err is a Postgres unique_violation. The
// only unique constraint hit when inserting a signup user is the global email
// uniqueness index (a new account has no other users), so it maps to ErrEmailTaken.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
