package httpapi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testenv"
)

// Test environments are workspace-wide, like the runner they configure: the
// runner infers and extends them (runner/testenv.go); people read, replace
// and forget them here.

func toTestEnvironment(row store.TestEnvironment) api.TestEnvironment {
	var recipe api.TestRecipe
	_ = json.Unmarshal(row.Recipe, &recipe) // same JSON shape as testenv.Recipe
	return api.TestEnvironment{
		Repo: row.Repo, Recipe: recipe, InferredFrom: row.InferredFrom, LearnedFromRunId: row.LearnedFromRunID,
		EditedAt: row.EditedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (s *Server) ListTestEnvironments(ctx context.Context, _ api.ListTestEnvironmentsRequestObject) (api.ListTestEnvironmentsResponseObject, error) {
	_, _, ws := me(ctx)
	rows, err := s.Q.ListTestEnvironments(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := api.ListTestEnvironments200JSONResponse{Environments: make([]api.TestEnvironment, 0, len(rows))}
	for _, row := range rows {
		out.Environments = append(out.Environments, toTestEnvironment(row))
	}
	return out, nil
}

func (s *Server) PutTestEnvironment(ctx context.Context, req api.PutTestEnvironmentRequestObject) (api.PutTestEnvironmentResponseObject, error) {
	_, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	repo := testenv.RepoKey(req.Body.Repo)
	if repo == "" || len(repo) > 500 || !strings.Contains(repo, "/") {
		return nil, apperr.Invalid("repo", "must be a repository URL or key like github.com/acme/app")
	}
	raw, _ := json.Marshal(req.Body.Recipe)
	var recipe testenv.Recipe
	if err := json.Unmarshal(raw, &recipe); err != nil {
		return nil, apperr.Invalid("recipe", "%s", err.Error())
	}
	if err := recipe.Validate(); err != nil {
		return nil, apperr.Invalid("recipe", "%s", err.Error())
	}
	b, _ := json.Marshal(recipe) // normalized by Validate
	row, err := s.Q.PutTestEnvironment(ctx, store.PutTestEnvironmentParams{WorkspaceID: ws, Repo: repo, Recipe: b})
	if err != nil {
		return nil, err
	}
	return api.PutTestEnvironment200JSONResponse(toTestEnvironment(row)), nil
}

func (s *Server) DeleteTestEnvironment(ctx context.Context, req api.DeleteTestEnvironmentRequestObject) (api.DeleteTestEnvironmentResponseObject, error) {
	_, _, ws := me(ctx)
	n, err := s.Q.DeleteTestEnvironment(ctx, store.DeleteTestEnvironmentParams{WorkspaceID: ws, Repo: testenv.RepoKey(req.Params.Repo)})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, apperr.ErrNotFound
	}
	return api.DeleteTestEnvironment204Response{}, nil
}
