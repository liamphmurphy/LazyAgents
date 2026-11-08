package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func toWorkflowGraph(raw []byte) (api.WorkflowGraph, error) {
	var g api.WorkflowGraph
	parsed, err := runs.ParseWorkflowGraph(raw)
	if err != nil {
		return g, err
	}
	normal, err := json.Marshal(parsed)
	if err != nil {
		return g, err
	}
	return g, json.Unmarshal(normal, &g)
}

func fromWorkflowGraph(g api.WorkflowGraph) (runs.WorkflowGraph, error) {
	raw, err := json.Marshal(g)
	if err != nil {
		return runs.WorkflowGraph{}, err
	}
	return runs.ParseWorkflowGraph(raw)
}

func (s *Server) toWorkflow(ctx context.Context, w store.Workflow) (api.Workflow, error) {
	out := api.Workflow{Id: w.ID, Name: w.Name, Description: w.Description, PublishedVersion: int(w.PublishedVersion), UpdatedAt: w.UpdatedAt, Proposals: []api.WorkflowProposal{}, Issues: []api.WorkflowIssue{}}
	var err error
	if out.Draft, err = toWorkflowGraph(w.Draft); err != nil {
		return out, err
	}
	parsed, err := runs.ParseWorkflowGraph(w.Draft)
	if err != nil {
		return out, err
	}
	for _, i := range runs.ValidateWorkflow(parsed) {
		issue := api.WorkflowIssue{Message: i.Message}
		if i.Node != "" {
			issue.Node = &i.Node
		}
		out.Issues = append(out.Issues, issue)
	}
	if w.Published != nil {
		published, err := runs.ParseWorkflowGraph(w.Published)
		if err != nil {
			return out, err
		}
		a, _ := json.Marshal(parsed)
		b, _ := json.Marshal(published)
		out.UnpublishedChanges = string(a) != string(b)
	} else {
		out.UnpublishedChanges = true
	}
	rows, err := s.Q.ListPendingWorkflowProposals(ctx, w.ID)
	if err != nil {
		return out, err
	}
	for _, p := range rows {
		g, err := toWorkflowGraph(p.Graph)
		if err != nil {
			return out, err
		}
		out.Proposals = append(out.Proposals, api.WorkflowProposal{Id: p.ID, Summary: p.Summary, Graph: g, CreatedAt: p.CreatedAt})
	}
	return out, nil
}

func (s *Server) ListWorkflows(ctx context.Context, _ api.ListWorkflowsRequestObject) (api.ListWorkflowsResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListWorkflows(ctx, store.ListWorkflowsParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out := api.Workflows{Workflows: make([]api.Workflow, 0, len(rows))}
	for _, w := range rows {
		item, err := s.toWorkflow(ctx, w)
		if err != nil {
			return nil, err
		}
		out.Workflows = append(out.Workflows, item)
	}
	return api.ListWorkflows200JSONResponse(out), nil
}

func (s *Server) CreateWorkflow(ctx context.Context, req api.CreateWorkflowRequestObject) (api.CreateWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	w, err := s.Runs.CreateWorkflow(ctx, ws, u.ID, req.Body.Name, deref(req.Body.Description))
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflow(ctx, w)
	return api.CreateWorkflow201JSONResponse(out), err
}

func (s *Server) GetWorkflow(ctx context.Context, req api.GetWorkflowRequestObject) (api.GetWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	w, err := s.Q.GetWorkflow(ctx, store.GetWorkflowParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	out, err := s.toWorkflow(ctx, w)
	return api.GetWorkflow200JSONResponse(out), err
}

func (s *Server) UpdateWorkflow(ctx context.Context, req api.UpdateWorkflowRequestObject) (api.UpdateWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	edit := runs.WorkflowEdit{Name: req.Body.Name, Description: req.Body.Description}
	if req.Body.Graph != nil {
		g, err := fromWorkflowGraph(*req.Body.Graph)
		if err != nil {
			return nil, err
		}
		edit.Graph = &g
	}
	w, err := s.Runs.SaveWorkflow(ctx, ws, u.ID, req.Id, edit)
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflow(ctx, w)
	return api.UpdateWorkflow200JSONResponse(out), err
}

func (s *Server) DeleteWorkflow(ctx context.Context, req api.DeleteWorkflowRequestObject) (api.DeleteWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	n, err := s.Q.DeleteWorkflow(ctx, store.DeleteWorkflowParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, apperr.ErrNotFound
	}
	return api.DeleteWorkflow204Response{}, nil
}

func (s *Server) PublishWorkflow(ctx context.Context, req api.PublishWorkflowRequestObject) (api.PublishWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	w, err := s.Runs.PublishWorkflow(ctx, ws, u.ID, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflow(ctx, w)
	return api.PublishWorkflow200JSONResponse(out), err
}

func (s *Server) ResolveWorkflowProposal(ctx context.Context, req api.ResolveWorkflowProposalRequestObject) (api.ResolveWorkflowProposalResponseObject, error) {
	u, _, ws := me(ctx)
	w, err := s.Runs.ResolveWorkflowProposal(ctx, ws, u.ID, req.Id, req.ProposalId, string(req.Action) == "apply")
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflow(ctx, w)
	return api.ResolveWorkflowProposal200JSONResponse(out), err
}

func (s *Server) toWorkflowRun(ctx context.Context, r store.WorkflowRun) (api.WorkflowRun, error) {
	out := api.WorkflowRun{Id: r.ID, ChatId: r.ChatID, WorkflowId: r.WorkflowID, MessageId: r.MessageID, Name: r.Name, Version: int(r.Version), Input: r.Input,
		State: api.WorkflowRunState(r.State), Error: r.Error, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, Steps: []api.WorkflowStep{}}
	var err error
	if out.Graph, err = toWorkflowGraph(r.Graph); err != nil {
		return out, err
	}
	rows, err := s.Q.LatestWorkflowSteps(ctx, r.ID)
	if err != nil {
		return out, err
	}
	all, err := s.Q.ListWorkflowRunImages(ctx, r.ID)
	if err != nil {
		return out, err
	}
	graph, err := runs.ParseWorkflowGraph(r.Graph)
	if err != nil {
		return out, err
	}
	carried := runs.WorkflowImagesByNode(graph, rows, all)
	for _, st := range rows {
		step := api.WorkflowStep{NodeId: st.NodeID, Attempt: int(st.Attempt), State: api.WorkflowStepState(st.State), Output: st.Output, AgentRunId: st.AgentRunID,
			Decision: st.Decision, Note: st.Note, CheckResult: st.CheckResult, CreatedAt: st.CreatedAt, FinishedAt: st.FinishedAt}
		if images := carried[st.NodeID]; len(images) > 0 {
			list := toWorkflowImages(images)
			step.Images = &list
		}
		out.Steps = append(out.Steps, step)
	}
	if len(all) > 0 {
		list := toWorkflowImages(all)
		out.Images = &list
	}
	checks, err := s.Q.ListWorkflowCheckAttempts(ctx, r.ID)
	if err != nil {
		return out, err
	}
	out.CheckAttempts = make([]api.WorkflowCheckAttempt, len(checks))
	for i, c := range checks {
		out.CheckAttempts[i] = api.WorkflowCheckAttempt{NodeId: c.NodeID, Attempt: int(c.Attempt), Result: c.CheckResult, Output: c.Output, Note: c.Note, FinishedAt: c.FinishedAt}
	}
	return out, nil
}

func toWorkflowImages(rows []runs.WorkflowImage) []api.WorkflowImage {
	out := make([]api.WorkflowImage, len(rows))
	for i, im := range rows {
		out[i] = api.WorkflowImage{Id: im.ID, NodeId: im.NodeID, Attempt: int(im.Attempt), Filename: im.Filename, ContentType: im.ContentType, SizeBytes: im.SizeBytes, CreatedAt: im.CreatedAt}
	}
	return out
}

// GetWorkflowRunImageContent serves an image of the caller's own run; the run
// lookup is the authorization, and an image of another run is a 404.
func (s *Server) GetWorkflowRunImageContent(ctx context.Context, req api.GetWorkflowRunImageContentRequestObject) (api.GetWorkflowRunImageContentResponseObject, error) {
	u, _, ws := me(ctx)
	if _, err := s.Q.GetWorkflowRun(ctx, store.GetWorkflowRunParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID}); errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	im, err := s.Q.GetWorkflowRunImage(ctx, store.GetWorkflowRunImageParams{ID: req.ImageId, RunID: req.Id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, im.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return contentResponse{body: o.Body, closer: o.Body, contentType: im.ContentType, length: im.SizeBytes, filename: im.Filename, inline: true}, nil
}

func (s *Server) toWorkflowRuns(ctx context.Context, rows []store.WorkflowRun) (api.WorkflowRuns, error) {
	out := api.WorkflowRuns{Runs: make([]api.WorkflowRun, 0, len(rows))}
	for _, r := range rows {
		item, err := s.toWorkflowRun(ctx, r)
		if err != nil {
			return out, err
		}
		out.Runs = append(out.Runs, item)
	}
	return out, nil
}

func (s *Server) ListWorkflowRuns(ctx context.Context, _ api.ListWorkflowRunsRequestObject) (api.ListWorkflowRunsResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListWorkflowRuns(ctx, store.ListWorkflowRunsParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRuns(ctx, rows)
	return api.ListWorkflowRuns200JSONResponse(out), err
}

func (s *Server) GetWorkflowRun(ctx context.Context, req api.GetWorkflowRunRequestObject) (api.GetWorkflowRunResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Q.GetWorkflowRun(ctx, store.GetWorkflowRunParams{ID: req.Id, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRun(ctx, r)
	return api.GetWorkflowRun200JSONResponse(out), err
}

func (s *Server) DecideWorkflowGate(ctx context.Context, req api.DecideWorkflowGateRequestObject) (api.DecideWorkflowGateResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Runs.DecideWorkflowGate(ctx, ws, u.ID, req.Id, runs.WorkflowDecision{NodeID: req.Body.NodeId, Decision: req.Body.Decision, Note: deref(req.Body.Note)})
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRun(ctx, r)
	return api.DecideWorkflowGate200JSONResponse(out), err
}

func (s *Server) StopWorkflowRun(ctx context.Context, req api.StopWorkflowRunRequestObject) (api.StopWorkflowRunResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Runs.StopWorkflowRun(ctx, ws, u.ID, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRun(ctx, r)
	return api.StopWorkflowRun200JSONResponse(out), err
}

func (s *Server) ListChatWorkflowRuns(ctx context.Context, req api.ListChatWorkflowRunsRequestObject) (api.ListChatWorkflowRunsResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListChatWorkflowRuns(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRuns(ctx, rows)
	return api.ListChatWorkflowRuns200JSONResponse(out), err
}

func (s *Server) StartChatWorkflow(ctx context.Context, req api.StartChatWorkflowRequestObject) (api.StartChatWorkflowResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Runs.StartWorkflow(ctx, ws, u.ID, req.Id, runs.WorkflowStart{WorkflowID: req.Body.WorkflowId, Input: req.Body.Input, MessageID: req.Body.MessageId})
	if err != nil {
		return nil, err
	}
	out, err := s.toWorkflowRun(ctx, r)
	return api.StartChatWorkflow201JSONResponse(out), err
}
