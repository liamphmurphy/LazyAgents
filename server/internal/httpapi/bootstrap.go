package httpapi

import (
	"context"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
)

func (s *Server) GetBootstrap(ctx context.Context, _ api.GetBootstrapRequestObject) (api.GetBootstrapResponseObject, error) {
	u, _, ws := me(ctx)
	w, err := s.Q.GetWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}
	users, err := s.Q.ListUsers(ctx, ws)
	if err != nil {
		return nil, err
	}
	projects, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	agents, err := s.Q.ListAgents(ctx, ws)
	if err != nil {
		return nil, err
	}
	epics, err := s.Q.ListEpics(ctx, ws)
	if err != nil {
		return nil, err
	}
	labels, err := s.Q.ListLabels(ctx, ws)
	if err != nil {
		return nil, err
	}

	out := api.GetBootstrap200JSONResponse{
		Workspace: api.Workspace{Id: w.ID, Name: w.Name, Slug: w.Slug},
		Me:        toUser(u),
		Users:     make([]api.User, 0, len(users)),
		Projects:  make([]api.Project, 0, len(projects)),
		Agents:    make([]api.Agent, 0, len(agents)),
		Epics:     make([]api.Epic, 0, len(epics)),
		Labels:    make([]api.Label, 0, len(labels)),
	}
	for _, x := range users {
		out.Users = append(out.Users, toUser(x))
	}
	for _, x := range projects {
		out.Projects = append(out.Projects, toProject(x))
	}
	for _, x := range agents {
		out.Agents = append(out.Agents, toAgent(x))
		if x.Status == "running" {
			out.Counts.LiveAgents++
		}
	}
	for _, x := range epics {
		out.Epics = append(out.Epics, api.Epic{Id: x.ID, ProjectId: x.ProjectID, Name: x.Name, Health: api.EpicHealth(x.Health)})
	}
	for _, x := range labels {
		out.Labels = append(out.Labels, toLabel(x))
	}
	// Inbox counts (approvals, mentions) arrive with Phase 4/6.
	return out, nil
}
