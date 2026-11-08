package agentapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// workflowTools is a workflow step's whole MCP surface: read its project, then report.
var workflowTools = []string{"submit_workflow_output", "search_knowledge", "read_page", "read_artifact", "get_ticket", "get_backlinks"}

// workflowWebTools is the surface of a step that reads the web: it can't also
// read the workspace, so a page's instructions have nothing to carry out.
var workflowWebTools = []string{"submit_workflow_output"}

func workflowStepHasWeb(run store.Run) bool {
	spec, err := runs.Spec(run)
	return err != nil || spec.HarnessOptions[agentdef.OptionWebSearch] == "true"
}

func workflowToolsFor(run store.Run) []string {
	if workflowStepHasWeb(run) {
		return workflowWebTools
	}
	return workflowTools
}

// workflowScope limits a step's reads to its chat's project, like the chat itself.
func workflowScope(ctx context.Context, d Deps, run store.Run) (*chatScope, error) {
	wf, err := d.Q.WorkflowRunForAgentRun(ctx, &run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Conflict("this step is no longer part of a workflow")
	}
	if err != nil {
		return nil, err
	}
	p, err := d.Q.GetProject(ctx, store.GetProjectParams{ID: wf.ProjectID, WorkspaceID: wf.WorkspaceID})
	if err != nil {
		return nil, err
	}
	return &chatScope{ChatID: wf.ChatID, ProjectID: wf.ProjectID, ProjectKey: p.Key, UserID: wf.UserID}, nil
}

func workflowChatScope(ctx context.Context, d Deps, c agents.Caller, tool string) (*chatScope, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("%s only works in a chat with a person", tool)
	}
	return s, nil
}

type NoWorkflowIn struct{}

func listWorkflows(ctx context.Context, d Deps, c agents.Caller, _ NoWorkflowIn) (any, error) {
	s, err := workflowChatScope(ctx, d, c, "list_workflows")
	if err != nil {
		return nil, err
	}
	rows, err := d.Q.ListWorkflows(ctx, store.ListWorkflowsParams{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, w := range rows {
		out = append(out, map[string]any{"id": w.ID, "name": w.Name, "description": w.Description, "published_version": w.PublishedVersion})
	}
	return map[string]any{"workflows": out}, nil
}

type ReadWorkflowIn struct {
	WorkflowID uuid.UUID `json:"workflow_id"`
}

func readWorkflow(ctx context.Context, d Deps, c agents.Caller, in ReadWorkflowIn) (any, error) {
	s, err := workflowChatScope(ctx, d, c, "read_workflow")
	if err != nil {
		return nil, err
	}
	w, err := d.Q.GetWorkflow(ctx, store.GetWorkflowParams{ID: in.WorkflowID, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	g, err := runs.ParseWorkflowGraph(w.Draft)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": w.ID, "name": w.Name, "description": w.Description, "published_version": w.PublishedVersion, "draft": g, "issues": runs.ValidateWorkflow(g)}, nil
}

type ProposeWorkflowEditIn struct {
	WorkflowID  *uuid.UUID        `json:"workflow_id,omitempty" jsonschema:"The workflow to change; omit to start a new one"`
	Name        string            `json:"name,omitempty" jsonschema:"Required for a new workflow"`
	Description string            `json:"description,omitempty" jsonschema:"For a new workflow: when it should be used"`
	Summary     string            `json:"summary" jsonschema:"One sentence on what this change does"`
	Ops         []runs.WorkflowOp `json:"ops" jsonschema:"The edits, applied in order"`
}

func proposeWorkflowEdit(ctx context.Context, d Deps, c agents.Caller, in ProposeWorkflowEditIn) (any, error) {
	s, err := workflowChatScope(ctx, d, c, "propose_workflow_edit")
	if err != nil {
		return nil, err
	}
	p, err := d.Runs.ProposeWorkflowEdit(ctx, c.Agent.WorkspaceID, s.UserID, &s.ChatID, runs.WorkflowProposalInput{WorkflowID: in.WorkflowID, Name: in.Name, Description: in.Description, Summary: in.Summary, Ops: in.Ops})
	if err != nil {
		return nil, err
	}
	return map[string]any{"proposed": true, "workflow_id": p.WorkflowID, "proposal_id": p.ID, "workflows_url": "/workflows/" + p.WorkflowID.String(), "state": "waiting for the person to apply it"}, nil
}

type SubmitWorkflowOutputIn struct {
	Output string `json:"output" jsonschema:"Your complete result, in markdown"`
}

func submitWorkflowOutput(ctx context.Context, d Deps, c agents.Caller, in SubmitWorkflowOutputIn) (any, error) {
	if c.RunID == nil {
		return nil, apperr.Conflict("submit_workflow_output only works in a workflow step")
	}
	if err := d.Runs.SubmitWorkflowOutput(ctx, *c.RunID, in.Output); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true}, nil
}
