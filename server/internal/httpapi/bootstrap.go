package httpapi

import (
	"context"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
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
	statuses, err := s.liveAgentStatuses(ctx, ws)
	if err != nil {
		return nil, err
	}
	for _, x := range agents {
		a := toAgent(x)
		a.Status = api.AgentStatusIdle
		if st, ok := statuses[x.ID]; ok {
			a.Status = st
		}
		out.Agents = append(out.Agents, a)
		if a.Status == api.AgentStatusRunning {
			out.Counts.LiveAgents++
		}
	}
	for _, x := range epics {
		out.Epics = append(out.Epics, toEpic(x))
	}
	for _, x := range labels {
		out.Labels = append(out.Labels, toLabel(x))
	}
	items, err := s.Q.ListInbox(ctx, store.ListInboxParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		out.Counts.Inbox = int(items[0].UnreadCount)
	}
	sugg, err := s.Q.CountSuggestions(ctx, ws)
	if err != nil {
		return nil, err
	}
	out.Counts.WikiSuggestions = int(sugg)
	return out, nil
}
