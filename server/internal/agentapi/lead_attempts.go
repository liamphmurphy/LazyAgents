package agentapi

import (
	"context"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A ticket run that follows a failed or cancelled one started from nothing:
// it couldn't see why the last attempt stopped, what it planned or where it
// left its work, so it repeated the same dead end. get_prior_attempts gives
// it the earlier runs on its own ticket and nothing else.

// maxPriorAttempts bounds how many earlier runs get_prior_attempts returns.
const maxPriorAttempts = 5

type priorAttempt struct {
	RunID           uuid.UUID        `json:"run_id"`
	Agent           string           `json:"agent"`
	State           string           `json:"state"`
	Outcome         string           `json:"outcome,omitempty"`
	WaitReason      string           `json:"wait_reason,omitempty"`
	StartedAt       string           `json:"started_at"`
	EndedAt         string           `json:"ended_at,omitempty"`
	SavedWorkBranch string           `json:"saved_work_branch,omitempty"`
	SaveError       string           `json:"save_error,omitempty"`
	Plan            []map[string]any `json:"plan"`
}

// priorRuns are the top-level runs on run's ticket that started before it,
// newest first. Delegated runs report to the run that started them.
func priorRuns(ctx context.Context, d Deps, run store.Run) ([]store.Run, error) {
	purpose := "ticket"
	rows, err := d.Q.ListRuns(ctx, store.ListRunsParams{WorkspaceID: run.WorkspaceID, TicketID: run.TicketID, Purpose: &purpose,
		States: append(append([]string{}, chatLiveRunStates...), chatFinishedRunStates...)})
	if err != nil {
		return nil, err
	}
	out := []store.Run{}
	for _, r := range rows {
		if r.ID != run.ID && r.ParentRunID == nil && r.StartedAt.Before(run.StartedAt) {
			out = append(out, r)
		}
	}
	return out, nil
}

func getPriorAttempts(ctx context.Context, d Deps, c agents.Caller, in TicketArg) (any, error) {
	run, t, err := d.Runs.Active(ctx, d.Q, c, in.Ticket)
	if err != nil {
		return nil, err
	}
	rows, err := priorRuns(ctx, d, run)
	if err != nil {
		return nil, err
	}
	n := &runNamer{d: d, ws: c.Agent.WorkspaceID, tickets: map[uuid.UUID]store.Ticket{}, agents: map[uuid.UUID]string{}}
	out := []priorAttempt{}
	for _, r := range rows[:min(len(rows), maxPriorAttempts)] {
		cr, err := n.toChatRun(ctx, r)
		if err != nil {
			return nil, err
		}
		a := priorAttempt{RunID: r.ID, Agent: cr.Agent, State: r.State, Outcome: strOr(r.Outcome), WaitReason: strOr(r.WaitReason), StartedAt: cr.StartedAt, EndedAt: cr.EndedAt,
			SavedWorkBranch: strOr(r.CheckpointBranch), SaveError: strOr(r.CheckpointError), Plan: []map[string]any{}}
		steps, err := d.Q.ListPlan(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		for i, st := range steps {
			a.Plan = append(a.Plan, map[string]any{"n": i + 1, "text": st.Text, "state": st.State})
		}
		out = append(out, a)
	}
	res := map[string]any{"ticket": t.Key, "attempts": out}
	if t.Branch != nil {
		res["ticket_branch"] = *t.Branch
	}
	if t.PrUrl != nil {
		res["pr_url"] = *t.PrUrl
	}
	if len(out) == 0 {
		res["message"] = "This is the first run on " + t.Key + "."
	}
	return res, nil
}
