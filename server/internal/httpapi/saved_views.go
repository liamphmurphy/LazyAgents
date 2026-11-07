package httpapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Saved views are per person: every query is scoped to the signed-in user, so
// one person can't read, rename or delete another's views (they 404).

func toSavedView(v store.SavedView) api.SavedView {
	return api.SavedView{Id: v.ID, Name: v.Name, Query: v.Query, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func savedViewName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return "", apperr.Invalid("name", "must be 1 to 80 characters")
	}
	return name, nil
}

func savedViewQuery(query string) (string, error) {
	query = strings.TrimPrefix(strings.TrimSpace(query), "?")
	if len(query) > 2000 {
		return "", apperr.Invalid("query", "must be at most 2000 characters")
	}
	return query, nil
}

func savedViewErr(err error) error {
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &pe) && pe.Code == "23505":
		return apperr.ConflictField("name", "You already have a view with that name")
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.ErrNotFound
	}
	return err
}

func (s *Server) ListSavedViews(ctx context.Context, _ api.ListSavedViewsRequestObject) (api.ListSavedViewsResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListSavedViews(ctx, store.ListSavedViewsParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out := api.ListSavedViews200JSONResponse{Views: make([]api.SavedView, 0, len(rows))}
	for _, v := range rows {
		out.Views = append(out.Views, toSavedView(v))
	}
	return out, nil
}

func (s *Server) CreateSavedView(ctx context.Context, req api.CreateSavedViewRequestObject) (api.CreateSavedViewResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	name, err := savedViewName(req.Body.Name)
	if err != nil {
		return nil, err
	}
	query, err := savedViewQuery(req.Body.Query)
	if err != nil {
		return nil, err
	}
	v, err := s.Q.CreateSavedView(ctx, store.CreateSavedViewParams{WorkspaceID: ws, UserID: u.ID, Name: name, Query: query})
	if err != nil {
		return nil, savedViewErr(err)
	}
	return api.CreateSavedView201JSONResponse(toSavedView(v)), nil
}

func (s *Server) UpdateSavedView(ctx context.Context, req api.UpdateSavedViewRequestObject) (api.UpdateSavedViewResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	p := store.UpdateSavedViewParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID}
	if req.Body.Name != nil {
		name, err := savedViewName(*req.Body.Name)
		if err != nil {
			return nil, err
		}
		p.Name = &name
	}
	if req.Body.Query != nil {
		query, err := savedViewQuery(*req.Body.Query)
		if err != nil {
			return nil, err
		}
		p.Query = &query
	}
	v, err := s.Q.UpdateSavedView(ctx, p)
	if err != nil {
		return nil, savedViewErr(err)
	}
	return api.UpdateSavedView200JSONResponse(toSavedView(v)), nil
}

func (s *Server) DeleteSavedView(ctx context.Context, req api.DeleteSavedViewRequestObject) (api.DeleteSavedViewResponseObject, error) {
	u, _, ws := me(ctx)
	n, err := s.Q.DeleteSavedView(ctx, store.DeleteSavedViewParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, apperr.ErrNotFound
	}
	return api.DeleteSavedView204Response{}, nil
}
