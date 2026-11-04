package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// Delegation (PLAN.md §6.8). Main agents take tickets; sub agents only run
// when a main agent hands them part of a ticket. A delegated run is a child of
// the main agent's run: same ticket, and it works in the parent's checkout on
// the parent's branch. The parent waits (wait_reason "delegation") and is
// resumed with the child's outcome, so one agent edits the checkout at a time.
// Parallel delegation is planned but not built.

const (
	VerbDelegated = "delegated" // {run_id, child_run_id, agent_id, agent_name, task}

	// MaxDelegations bounds the delegated runs under one parent run.
	MaxDelegations = 8
	maxTaskRunes   = 20000
)

// Contract is what a main agent asks of a sub agent beyond the task text. The
// runner checks the result against it before waking the main agent.
type Contract struct {
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	// AllowedPaths are repo-relative files, directories (trailing "/") or
	// globs the sub agent may change. Empty means unrestricted.
	AllowedPaths []string `json:"allowed_paths,omitempty"`
	// TestCommand must exit 0 in the checkout for the result to pass.
	TestCommand string `json:"test_command,omitempty"`
	// BaseSHA is the checkout's HEAD when the sub agent started; the runner sets it.
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseUncommitted are files already dirty at that point; they aren't the sub agent's.
	BaseUncommitted []string `json:"base_uncommitted,omitempty"`
}

const (
	maxContractItems = 20
	maxContractRunes = 500
)

// Normalize trims the contract and rejects what can't be checked or rendered.
func (c Contract) Normalize() (Contract, error) {
	clean := func(field string, in []string) ([]string, error) {
		var out []string
		for _, v := range in {
			if v = strings.TrimSpace(v); v != "" {
				if len([]rune(v)) > maxContractRunes {
					return nil, apperr.Invalid(field, "each entry must be at most %d characters", maxContractRunes)
				}
				out = append(out, v)
			}
		}
		if len(out) > maxContractItems {
			return nil, apperr.Invalid(field, "at most %d entries", maxContractItems)
		}
		return out, nil
	}
	var err error
	if c.AcceptanceCriteria, err = clean("acceptance_criteria", c.AcceptanceCriteria); err != nil {
		return c, err
	}
	if c.AllowedPaths, err = clean("allowed_paths", c.AllowedPaths); err != nil {
		return c, err
	}
	for _, p := range c.AllowedPaths {
		if strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
			return c, apperr.Invalid("allowed_paths", "%q must be a repo-relative path without ..", p)
		}
		if _, err := path.Match(p, "x"); err != nil {
			return c, apperr.Invalid("allowed_paths", "%q is not a valid glob", p)
		}
	}
	if c.TestCommand = strings.TrimSpace(c.TestCommand); len([]rune(c.TestCommand)) > maxContractRunes {
		return c, apperr.Invalid("test_command", "must be at most %d characters", maxContractRunes)
	}
	c.BaseSHA, c.BaseUncommitted = "", nil
	return c, nil
}

// Empty reports whether the contract adds nothing to the task.
func (c Contract) Empty() bool {
	return len(c.AcceptanceCriteria) == 0 && len(c.AllowedPaths) == 0 && c.TestCommand == ""
}

// PathAllowed reports whether a changed file is inside the contract's
// boundaries. A pattern matches a file exactly, as a directory prefix
// ("internal/runs/" or "internal/runs"), or as a path.Match glob on the whole
// path or its base name.
func (c Contract) PathAllowed(file string) bool {
	if len(c.AllowedPaths) == 0 {
		return true
	}
	for _, p := range c.AllowedPaths {
		if file == p || strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
		if ok, _ := path.Match(p, file); ok {
			return true
		}
	}
	return false
}

// ChildPrompt is the first message of a delegated run.
func ChildPrompt(ticket, parentHandle, task string, contract Contract) string {
	p := "@" + parentHandle + " delegated part of " + ticket + " to you. You work in the same checkout and branch; " +
		"do not call claim_ticket. Do only what the task says, commit your work to the current branch, then call complete_run " +
		"with a short summary of what you did and anything the main agent must check. If you're blocked, call complete_run and say why.\n\n## Task\n\n" + task
	if len(contract.AcceptanceCriteria) > 0 {
		p += "\n\n## Acceptance criteria\n\nThe task is done only when all of these hold:\n"
		for _, a := range contract.AcceptanceCriteria {
			p += "- " + a + "\n"
		}
	}
	if len(contract.AllowedPaths) > 0 {
		p += "\n\n## Allowed files\n\nChange only these paths. The server rejects work that touches anything else; if the task needs more, say so in complete_run instead:\n"
		for _, a := range contract.AllowedPaths {
			p += "- " + a + "\n"
		}
	}
	if contract.TestCommand != "" {
		p += "\n\n## Tests\n\nThis must exit 0 before you finish (the server runs it again after you): `" + contract.TestCommand + "`\n"
	}
	return strings.TrimRight(p, "\n")
}

type Delegation struct {
	Run   store.Run
	Agent store.Agent
}

// Delegate starts a sub agent's run under the caller's run and parks the
// caller until it finishes.
func (s *Service) Delegate(ctx context.Context, c Caller, ticketKey, handle, task string) (Delegation, error) {
	return s.DelegateWithContract(ctx, c, ticketKey, handle, task, Contract{})
}

// DelegateWithContract is Delegate with acceptance criteria, file boundaries
// and a test command the runner checks the result against.
func (s *Service) DelegateWithContract(ctx context.Context, c Caller, ticketKey, handle, task string, contract Contract) (Delegation, error) {
	var out Delegation
	task = strings.TrimSpace(task)
	if task == "" {
		return out, apperr.Invalid("task", "say what the sub agent should do, with enough context to work alone")
	}
	if len([]rune(task)) > maxTaskRunes {
		return out, apperr.Invalid("task", "must be at most %d characters", maxTaskRunes)
	}
	contract, err := contract.Normalize()
	if err != nil {
		return out, err
	}
	handle = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	err = s.tx(ctx, func(q *store.Queries) error {
		if c.Agent.Kind == agents.KindSub {
			return apperr.Conflict("sub agents can't delegate")
		}
		run, t, err := s.running(ctx, q, c, ticketKey)
		if err != nil {
			return err
		}
		if run.ParentRunID != nil {
			return apperr.Conflict("a delegated run can't delegate further")
		}
		if run.Mode != "managed" || run.WorkspaceVolume == nil {
			return apperr.Conflict("delegating needs a managed run with a ready workspace; unmanaged sessions can't delegate")
		}
		ws := run.WorkspaceID
		sub, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: handle})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("agent", "no agent @%s; call list_subagents", handle)
		} else if err != nil {
			return err
		}
		if sub.Kind != agents.KindSub {
			return apperr.Invalid("agent", "@%s is a main agent; delegate to a sub agent (list_subagents)", handle)
		}
		if _, err := q.ActiveChildRun(ctx, &run.ID); err == nil {
			return apperr.Conflict("a delegated run is already in progress")
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if n, err := q.CountChildRuns(ctx, &run.ID); err != nil {
			return err
		} else if n >= MaxDelegations {
			return apperr.Conflict("this run has already delegated %d times; finish the rest yourself or hand back with complete_run", n)
		}
		p, err := q.LockProject(ctx, store.LockProjectParams{ID: t.ProjectID, WorkspaceID: ws})
		if err != nil {
			return err
		}
		resolved, err := agents.Resolve(ctx, q, ws, sub, p.Key)
		if err != nil {
			return err
		}
		resolved.Repository = &agentdef.Repository{URL: p.RepoUrl, DefaultBranch: p.DefaultBranch}
		raw, err := json.Marshal(resolved)
		if err != nil {
			return err
		}
		child, err := q.CreateRun(ctx, store.CreateRunParams{
			WorkspaceID: ws, TicketID: &t.ID, AgentID: sub.ID, Mode: "managed", State: "queued",
			ResolvedSpec: raw, DefinitionVersionID: sub.CurrentDefinitionVersionID, Harness: resolved.Harness, Model: resolved.Model, Purpose: "ticket",
			ParentRunID: &run.ID,
		})
		if err != nil {
			return err
		}
		if run.DispatchedByUserID != nil {
			if err := q.SetRunDispatcher(ctx, store.SetRunDispatcherParams{ID: child.ID, DispatchedByUserID: run.DispatchedByUserID}); err != nil {
				return err
			}
		}
		if _, err := q.StartTurn(ctx, store.StartTurnParams{RunID: child.ID, Trigger: "start", InputMessage: ChildPrompt(t.Key, c.Agent.Handle, task, contract)}); err != nil {
			return err
		}
		if !contract.Empty() {
			raw, err := json.Marshal(contract)
			if err != nil {
				return err
			}
			if child, err = q.SetRunContract(ctx, store.SetRunContractParams{ID: child.ID, Contract: raw}); err != nil {
				return err
			}
		}
		if err := event(ctx, q, child.ID, "lifecycle", map[string]any{"state": "queued", "mode": "managed", "parent_run_id": run.ID, "delegated_by": c.Agent.Handle}); err != nil {
			return err
		}
		wait := "delegation"
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "waiting", WaitReason: &wait}); err != nil {
			return err
		}
		if err := q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](0)}); err != nil {
			return err
		}
		if err := event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "waiting", "reason": "delegation", "child_run_id": child.ID, "agent": sub.Handle}); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, ws, t.ID, t.EpicID, agentActor(c), VerbDelegated, map[string]any{
			"run_id": run.ID, "child_run_id": child.ID, "agent_id": sub.ID, "agent_name": sub.Name, "task": truncate(task, 300), "has_contract": !contract.Empty(),
		}, nil); err != nil {
			return err
		}
		if err := RecomputeAgentStatus(ctx, q, c.Agent.ID); err != nil {
			return err
		}
		if err := RecomputeAgentStatus(ctx, q, sub.ID); err != nil {
			return err
		}
		out = Delegation{Run: child, Agent: sub}
		return nil
	})
	return out, err
}

// ListSubagents is who a main agent can delegate to.
func (s *Service) ListSubagents(ctx context.Context, c Caller) ([]store.Agent, error) {
	if c.Agent.Kind == agents.KindSub {
		return nil, apperr.Conflict("sub agents can't delegate")
	}
	return store.New(s.Pool).ListSubagents(ctx, c.Agent.WorkspaceID)
}

// WakeParent resumes a waiting parent once its delegated run has ended,
// telling it how. The parent stays parked if a person paused or cancelled it.
func WakeParent(ctx context.Context, q *store.Queries, child store.Run, state, outcome string) error {
	if child.ParentRunID == nil {
		return nil
	}
	parent, err := q.LockRun(ctx, *child.ParentRunID)
	if err != nil {
		return err
	}
	if parent.State != "waiting" || deref(parent.WaitReason) != "delegation" {
		return nil
	}
	handle := child.AgentID.String()
	if a, err := q.GetAgent(ctx, store.GetAgentParams{ID: child.AgentID, WorkspaceID: child.WorkspaceID}); err == nil {
		handle = "@" + a.Handle
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		outcome = "(no summary)"
	}
	var msg string
	switch state {
	case "completed":
		msg = handle + " finished the delegated task.\n\n" + outcome
	case "cancelled":
		msg = handle + "'s delegated run was cancelled: " + outcome
	default:
		msg = handle + "'s delegated run failed: " + outcome
	}
	msg += "\n\nThe server checks the workspace against the delegation contract (changed files, allowed paths, test command) before you resume; those results follow. " +
		"Don't repeat checks that already ran. Read the diff for what they can't judge, read get_ticket, verify the acceptance criteria, and delegate again or fix any remaining work yourself. " +
		"When every criterion is verified and checked, commit and push any changes, record the PR with set_pr, obtain delegated reviewer/QA approval, and finish with complete_run status=done."
	return resume(ctx, q, parent, "delegation", msg)
}

// CancelChildren ends a parent's live delegated runs (the parent was cancelled
// or failed); the runner kills their processes.
func CancelChildren(ctx context.Context, q *store.Queries, parentID uuid.UUID, reason string) error {
	kids, err := q.ChildRuns(ctx, &parentID)
	if err != nil {
		return err
	}
	for _, k := range kids {
		if slices.Contains(Terminal, k.State) {
			continue
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: k.ID, State: "cancelled", Outcome: &reason}); err != nil {
			return err
		}
		_ = q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: k.ID, ExitCode: ptr[int32](143)})
		_ = q.DropPendingTurns(ctx, k.ID)
		if err := q.ExpireRunApprovals(ctx, k.ID); err != nil {
			return err
		}
		if err := event(ctx, q, k.ID, "lifecycle", map[string]any{"state": "cancelled", "reason": reason}); err != nil {
			return err
		}
		if err := RecomputeAgentStatus(ctx, q, k.AgentID); err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// completeChild ends a delegated run: no ticket status change, and the parent
// is resumed with the summary.
func (s *Service) completeChild(ctx context.Context, c Caller, key, outcome, verdict string) error {
	return s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if run.ParentRunID == nil {
			return fmt.Errorf("not a delegated run")
		}
		done, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome})
		if err != nil {
			return err
		}
		if err := q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](0)}); err != nil {
			return err
		}
		if err := q.ExpireRunApprovals(ctx, run.ID); err != nil {
			return err
		}
		life := map[string]any{"state": "completed", "outcome": outcome}
		if verdict != "" {
			life["verdict"] = verdict
		}
		if err := event(ctx, q, run.ID, "lifecycle", life); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbRunCompleted, map[string]any{"run_id": run.ID, "outcome": outcome, "delegated": true, "verdict": verdict}, nil); err != nil {
			return err
		}
		if err := RecomputeAgentStatus(ctx, q, c.Agent.ID); err != nil {
			return err
		}
		return WakeParent(ctx, q, done, "completed", outcome)
	})
}
