// Package tracker is the domain service for tickets, criteria, comments and
// the activity timeline. Every mutation writes its activity events in the
// same transaction (PLAN.md §4 Activity).
package tracker

import "github.com/liamphmurphy/lazyagents/server/internal/apperr"

var ErrNotFound = apperr.ErrNotFound

type ValidationError = apperr.ValidationError

func invalid(field, format string, args ...any) error { return apperr.Invalid(field, format, args...) }
