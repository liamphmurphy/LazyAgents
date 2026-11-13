package auth

import (
	"context"

	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// There is no login for now (see docs/dev/plan.md §5.4): the API resolves every
// request to the workspace's local user. These helpers carry that user.

type ctxKey struct{}

// WithUser stores the acting user on the request context.
func WithUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(pageview.Person(ctx, u.ID), ctxKey{}, u)
}

// UserFrom returns the acting user set by the HTTP middleware.
func UserFrom(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(store.User)
	return u, ok
}
