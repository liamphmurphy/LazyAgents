// Package apperr holds the domain errors the HTTP layer maps to responses:
// ErrNotFound → 404, *ValidationError → 422.
package apperr

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

// ValidationError is a client mistake in one input field.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

func Invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// ConflictError means the request is valid but the current state forbids it
// (e.g. acting on a run that isn't running). Mapped to 409.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

func Conflict(format string, args ...any) error {
	return &ConflictError{Msg: fmt.Sprintf(format, args...)}
}
