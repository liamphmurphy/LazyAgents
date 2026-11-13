// Package pageview carries the trusted knowledge viewer across service boundaries.
// Only request authentication and chat ownership lookups may supply a UserID.
package pageview

import (
	"context"
	"github.com/google/uuid"
)

// Viewer without a UserID is a run: private pages are never visible.
// Agent pages are visible to runs, but excluded from people's default discovery.
type Viewer struct {
	UserID       *uuid.UUID
	IncludeAgent bool
}
type key struct{}

func With(ctx context.Context, v Viewer) context.Context { return context.WithValue(ctx, key{}, v) }
func From(ctx context.Context) Viewer {
	if v, ok := ctx.Value(key{}).(Viewer); ok {
		return v
	}
	return Viewer{IncludeAgent: true}
}
func Person(ctx context.Context, id uuid.UUID) context.Context { return With(ctx, Viewer{UserID: &id}) }
func IncludeAgents(ctx context.Context, include bool) context.Context {
	v := From(ctx)
	v.IncludeAgent = include
	return With(ctx, v)
}

// Run explicitly drops any person inherited from a dispatch/preview request.
func Run(ctx context.Context) context.Context { return With(ctx, Viewer{IncludeAgent: true}) }
