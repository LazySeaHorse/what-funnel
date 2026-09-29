package service

import (
	"errors"
	"fmt"
)

// Sentinel error categories. Handlers map them to HTTP statuses with
// errors.Is; anything that matches none of them is an internal failure and
// must not leak its message to clients.
var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("invalid request")
	ErrConflict   = errors.New("conflict")
)

// categorizedError carries a client-safe message and matches one of the
// sentinel categories via errors.Is.
type categorizedError struct {
	kind error
	msg  string
}

func (e *categorizedError) Error() string { return e.msg }
func (e *categorizedError) Unwrap() error { return e.kind }

func newCategorized(kind error, format string, args ...any) error {
	return &categorizedError{kind: kind, msg: fmt.Sprintf(format, args...)}
}

func notFoundf(format string, args ...any) error {
	return newCategorized(ErrNotFound, format, args...)
}

func forbiddenf(format string, args ...any) error {
	return newCategorized(ErrForbidden, format, args...)
}

func invalidf(format string, args ...any) error {
	return newCategorized(ErrValidation, format, args...)
}

func conflictf(format string, args ...any) error {
	return newCategorized(ErrConflict, format, args...)
}

// ClientMessage returns the message that is safe to show to API clients for
// a categorized error, and false for internal errors.
func ClientMessage(err error) (string, bool) {
	var c *categorizedError
	if errors.As(err, &c) {
		return c.msg, true
	}
	return "", false
}
