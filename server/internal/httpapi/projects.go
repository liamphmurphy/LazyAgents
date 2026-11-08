package httpapi

import (
	"context"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/projects"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func (s *Server) ListProjects(ctx context.Context, _ api.ListProjectsRequestObject) (api.ListProjectsResponseObject, error) {
	_, _, ws := me(ctx)
	rows, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := api.ListProjects200JSONResponse{}
	for _, p := range rows {
		out = append(out, toProject(p))
	}
	return out, nil
}
func (s *Server) CreateProject(ctx context.Context, req api.CreateProjectRequestObject) (api.CreateProjectResponseObject, error) {
	_, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	in := projects.Config{Name: req.Body.Name, Color: req.Body.Color, RepoURL: req.Body.RepoUrl, DefaultBranch: req.Body.DefaultBranch}
	p, err := (projects.Service{Pool: s.Pool}).Create(ctx, ws, req.Body.Key, in)
	if err != nil {
		return nil, err
	}
	return api.CreateProject201JSONResponse(toProject(p)), nil
}
func (s *Server) UpdateProject(ctx context.Context, req api.UpdateProjectRequestObject) (api.UpdateProjectResponseObject, error) {
	_, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	if req.Body.Revision < 1 || req.Body.Revision > 2147483647 {
		return nil, apperr.Invalid("revision", "invalid revision")
	}
	in := projects.Config{Name: req.Body.Name, Color: req.Body.Color, RepoURL: req.Body.RepoUrl, DefaultBranch: req.Body.DefaultBranch, Archived: req.Body.Archived, Revision: int32(req.Body.Revision)}
	p, err := (projects.Service{Pool: s.Pool}).Update(ctx, ws, req.Key, in)
	if err != nil {
		return nil, err
	}
	return api.UpdateProject200JSONResponse(toProject(p)), nil
}
func (s *Server) SetTicketDependencies(ctx context.Context, req api.SetTicketDependenciesRequestObject) (api.SetTicketDependenciesResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	if req.Body.Revision < 1 || req.Body.Revision > 2147483647 {
		return nil, apperr.Invalid("revision", "invalid revision")
	}
	if err := s.Tracker.SetDependencies(ctx, ws, tracker.UserActor(u.ID), req.Key, int32(req.Body.Revision), req.Body.BlockedBy); err != nil {
		return nil, err
	}
	out, err := s.reloadTicket(ctx, ws, req.Key)
	return api.SetTicketDependencies200JSONResponse(out), err
}
