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

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// Delegation (docs/dev/plan.md §6.8). Main agents take tickets; sub agents only run
// when a main agent hands them part of a ticket. A delegated run is a child of
// the main agent's run: same ticket, but it works in its own git worktree
// (inside the parent's workspace) on its own branch cut from the parent's
// HEAD. The parent waits (wait_reason "delegation"); up to MaxParallelDelegations
// children run at once as long as their allowed_paths are disjoint. When the
// last active child ends the runner checks each finished child in its
// worktree, merges the ones that passed into the parent's branch, and resumes
// the parent once with a report covering all of them. A child can continue an
// earlier child's worktree, branch and harness session (continue_run_id).

const (
	VerbDelegated = "delegated" // {run_id, child_run_id, agent_id, agent_name, task}

	// MaxDelegations bounds the delegated runs under one parent run.
	MaxDelegations = 8
	// MaxParallelDelegations bounds the delegated runs of one parent that are live at once.
	MaxParallelDelegations = 3
	maxTaskRunes           = 20000
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
	// ContinueRunID is an earlier, finished delegated run of the same parent
	// and sub agent to carry on: same worktree and branch, same harness session.
	ContinueRunID string `json:"continue_run_id,omitempty"`
	// BaseSHA is the worktree's HEAD when the sub agent started; the runner sets it.
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseUncommitted is kept for old runs; a worktree starts clean, so it is empty.
	BaseUncommitted []string `json:"base_uncommitted,omitempty"`
	// Branch and Worktree (relative to the workspace) are the child's git
	// branch and worktree. The runner sets them, or they're copied from the run
	// being continued.
	Branch   string `json:"branch,omitempty"`
	Worktree string `json:"worktree,omitempty"`
	// ResumeSession is set while the run's first turn resumes the continued
	// run's harness session; the runner clears it if that session is gone.
	ResumeSession bool `json:"resume_session,omitempty"`
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
	if c.ContinueRunID = strings.TrimSpace(c.ContinueRunID); c.ContinueRunID != "" {
		if _, err := uuid.Parse(c.ContinueRunID); err != nil {
			return c, apperr.Invalid("continue_run_id", "%q isn't a run id", c.ContinueRunID)
		}
	}
	c.BaseSHA, c.BaseUncommitted, c.Branch, c.Worktree, c.ResumeSession = "", nil, "", "", false
	return c, nil
}

// region is the part of the repository an allowed path claims: the path itself
// or, for a glob, the directory it is confined under ("" = anywhere).
func region(p string) string {
	p = strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")
	if !strings.ContainsAny(p, "*?[") {
		return p
	}
	var dir []string
	for _, el := range strings.Split(p, "/") {
		if strings.ContainsAny(el, "*?[") {
			break
		}
		dir = append(dir, el)
	}
	return strings.Join(dir, "/")
}

// PathsOverlap reports whether two sets of allowed paths might touch the same
// file. It is conservative: equal paths, one inside the other's directory, and
// globs (judged by the directory they're confined under, or anywhere if none)
// all count as overlapping.
func PathsOverlap(a, b []string) bool {
	for _, x := range a {
		rx := region(x)
		for _, y := range b {
			ry := region(y)
			if rx == "" || ry == "" || rx == ry || strings.HasPrefix(rx, ry+"/") || strings.HasPrefix(ry, rx+"/") {
				return true
			}
		}
	}
	return false
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
	p := "@" + parentHandle + " delegated part of " + ticket + " to you. You work in your own git worktree on your own branch, cut from the main agent's; " +
		"other sub agents may be working in parallel in theirs, so stay inside your task and do only what it says. " +
		"AGENTS.md says how to commit and finish."
	if contract.ContinueRunID != "" {
		p += " This continues your earlier run on this ticket: same worktree and branch, so your earlier work is still here. The task below is the follow-up."
	}
	p += "\n\n## Task\n\n" + task
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
		run, t, err := s.delegating(ctx, q, c, ticketKey)
		if err != nil {
			return err
		}
		// Match tracker/dispatch ordering. Child cancellation may already hold a
		// ticket FK lock when it wakes this parent; never hold the parent while
		// waiting for a project locked by a ticket patch.
		p, err := q.LockProject(ctx, store.LockProjectParams{ID: t.ProjectID, WorkspaceID: run.WorkspaceID})
		if err != nil {
			return err
		}
		// Serialise a parent's delegate calls (an agent may issue several at once):
		// the limits below read the other children.
		if run, err = q.LockRun(ctx, run.ID); err != nil {
			return err
		}
		if run.State != "running" && !(run.State == "waiting" && deref(run.WaitReason) == "delegation") {
			return apperr.Conflict("your run is no longer running")
		}
		if _, t, err = s.delegating(ctx, q, c, ticketKey); err != nil {
			return err
		}
		turns, err := q.ListTurns(ctx, run.ID)
		if err != nil {
			return err
		}
		for _, turn := range turns {
			if turn.Status == "pending" && turn.Trigger == "delegation" {
				return apperr.Conflict("your delegated runs already finished; end your turn to read their report")
			}
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
		kids, err := q.ChildRuns(ctx, &run.ID)
		if err != nil {
			return err
		}
		if len(kids) >= MaxDelegations {
			return apperr.Conflict("this run has already delegated %d times; finish the rest yourself or hand back with complete_run", len(kids))
		}
		prev, err := continuedRun(contract, kids, run, sub)
		if err != nil {
			return err
		}
		if prev != nil {
			var pc Contract
			_ = json.Unmarshal(prev.Contract, &pc)
			contract.Branch, contract.Worktree = pc.Branch, pc.Worktree
		}
		if err := checkParallel(kids, contract); err != nil {
			return err
		}
		resolved, err := agents.ResolveRunnable(ctx, q, ws, sub, p.Key)
		if err != nil {
			return err
		}
		// The child inherits the ticket's epic context so an epic ticket's
		// children also leave the full suite to the epic review.
		if resolved.Repository, err = RepositoryFor(ctx, q, p, t); err != nil {
			return err
		}
		resolved.Repository.EpicTickets = nil
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
		if prev != nil && prev.Harness == child.Harness && prev.HarnessSessionID != nil {
			// Same harness: the first turn resumes the earlier run's conversation.
			if err := q.SetRunSession(ctx, store.SetRunSessionParams{ID: child.ID, HarnessSessionID: prev.HarnessSessionID}); err != nil {
				return err
			}
			contract.ResumeSession = true
		}
		if !contract.Empty() || prev != nil {
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
		if run.State == "running" { // the first delegation of a turn parks the parent; later ones find it parked
			wait := "delegation"
			if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "waiting", WaitReason: &wait}); err != nil {
				return err
			}
			if err := q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](0)}); err != nil {
				return err
			}
		}
		if err := event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "waiting", "reason": "delegation", "child_run_id": child.ID, "agent": sub.Handle}); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, ws, t.ID, t.EpicID, agentActor(c), VerbDelegated, map[string]any{
			"run_id": run.ID, "child_run_id": child.ID, "agent_id": sub.ID, "agent_name": sub.Name, "task": truncate(task, 300), "has_contract": !contract.Empty(), "continues": contract.ContinueRunID,
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

// delegating is running, except that a parent parked by an earlier delegate
// call of the same turn (its harness process is still going) may delegate again.
func (s *Service) delegating(ctx context.Context, q *store.Queries, c Caller, key string) (store.Run, store.Ticket, error) {
	run, t, err := s.Active(ctx, q, c, key)
	if err == nil && run.State == "waiting" && deref(run.WaitReason) == "delegation" {
		turns, lerr := q.ListTurns(ctx, run.ID)
		if lerr != nil {
			return run, t, lerr
		}
		for _, tn := range turns {
			if tn.Status == "running" {
				return run, t, nil
			}
		}
	}
	return s.running(ctx, q, c, key)
}

// continuedRun resolves contract.ContinueRunID: a finished delegated run of
// this parent and this sub agent that no later run has already taken over.
func continuedRun(contract Contract, kids []store.Run, parent store.Run, sub store.Agent) (*store.Run, error) {
	if contract.ContinueRunID == "" {
		return nil, nil
	}
	var prev *store.Run
	at := 0
	for i := range kids {
		if kids[i].ID.String() == contract.ContinueRunID {
			prev, at = &kids[i], i
		}
	}
	switch {
	case prev == nil:
		return nil, apperr.Invalid("continue_run_id", "%s isn't a delegated run of this run; use the run_id delegate returned", contract.ContinueRunID)
	case prev.AgentID != sub.ID:
		return nil, apperr.Invalid("continue_run_id", "that run was delegated to a different agent; to continue it delegate to the same sub agent")
	case !slices.Contains(Terminal, prev.State):
		return nil, apperr.Invalid("continue_run_id", "that run is still in progress; wait for it to finish")
	}
	if len(prev.Checks) == 0 {
		return nil, apperr.Invalid("continue_run_id", "that run hasn't been checked and merged yet; wait for your delegation report")
	}
	var pc Contract
	_ = json.Unmarshal(prev.Contract, &pc)
	if pc.Worktree == "" {
		return nil, apperr.Invalid("continue_run_id", "that run never got a worktree (it failed to start), so there is nothing to continue; delegate afresh")
	}
	for _, k := range kids[at+1:] { // children are ordered by start
		var kc Contract
		if json.Unmarshal(k.Contract, &kc) == nil && kc.Worktree == pc.Worktree {
			return nil, apperr.Invalid("continue_run_id", "run %s already continued that one; continue the latest run instead", k.ID)
		}
	}
	return prev, nil
}

// checkParallel enforces the limits on children that are live at once: at most
// MaxParallelDelegations, and once two run together both need non-empty,
// disjoint allowed_paths so their worktrees can be merged without conflicts.
func checkParallel(kids []store.Run, contract Contract) error {
	var live []store.Run
	for _, k := range kids {
		if !slices.Contains(Terminal, k.State) {
			live = append(live, k)
		}
	}
	if len(live) == 0 {
		return nil
	}
	if len(live) >= MaxParallelDelegations {
		return apperr.Conflict("%d delegated runs are already in progress (the limit); wait for them to finish before delegating more", len(live))
	}
	if len(contract.AllowedPaths) == 0 {
		return apperr.Invalid("allowed_paths", "a delegated run is already in progress, so this one needs allowed_paths that don't overlap with it; set allowed_paths or delegate sequentially")
	}
	for _, k := range live {
		var kc Contract
		_ = json.Unmarshal(k.Contract, &kc)
		if len(kc.AllowedPaths) == 0 {
			return apperr.Conflict("the delegated run %s has no allowed_paths, so it may touch anything; wait for it to finish and then delegate, or delegate sequentially", k.ID)
		}
		if PathsOverlap(kc.AllowedPaths, contract.AllowedPaths) {
			return apperr.Conflict("allowed_paths overlap with the delegated run %s in progress (%s); narrow allowed_paths so they're disjoint, or delegate sequentially", k.ID, strings.Join(kc.AllowedPaths, ", "))
		}
	}
	return nil
}

// ListSubagents is who a main agent can delegate to.
func (s *Service) ListSubagents(ctx context.Context, c Caller) ([]store.Agent, error) {
	if c.Agent.Kind == agents.KindSub {
		return nil, apperr.Conflict("sub agents can't delegate")
	}
	return store.New(s.Pool).ListSubagents(ctx, c.Agent.WorkspaceID)
}

// WakeParent resumes a waiting parent once its last live delegated run has
// ended, telling it how each run that ended since it was parked went. The
// parent stays parked while any child is live, or if a person paused or
// cancelled it.
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
	msg, ok, err := delegationWakeMessage(ctx, q, parent)
	if err != nil || !ok {
		return err
	}
	return resume(ctx, q, parent, "delegation", msg)
}

// delegationWakeMessage includes every terminal child not yet checked by the runner.
func delegationWakeMessage(ctx context.Context, q *store.Queries, parent store.Run) (string, bool, error) {
	kids, err := q.ChildRuns(ctx, &parent.ID)
	if err != nil {
		return "", false, err
	}
	var ended []store.Run
	first := true // no earlier report: the parent hasn't seen the full guidance yet
	for _, k := range kids {
		if !slices.Contains(Terminal, k.State) {
			return "", false, nil
		}
		if len(k.Checks) == 0 {
			ended = append(ended, k)
		} else {
			first = false
		}
	}
	if len(ended) == 0 {
		return "", false, nil
	}
	line := func(k store.Run) string {
		handle := k.AgentID.String()
		if a, err := q.GetAgent(ctx, store.GetAgentParams{ID: k.AgentID, WorkspaceID: k.WorkspaceID}); err == nil {
			handle = "@" + a.Handle
		}
		o := strings.TrimSpace(deref(k.Outcome))
		if o == "" {
			o = "(no summary)"
		}
		switch k.State {
		case "completed":
			return handle + " finished the delegated task.\n\n" + o
		case "cancelled":
			return handle + "'s delegated run was cancelled: " + o
		default:
			return handle + "'s delegated run failed: " + o
		}
	}
	var msg string
	if len(ended) == 1 {
		msg = line(ended[0])
	} else {
		msg = fmt.Sprintf("%d delegated runs have ended.", len(ended))
		for i, k := range ended {
			msg += fmt.Sprintf("\n\n### %d. %s", i+1, line(k))
		}
	}
	if !first {
		return msg + "\n\nChecks and merge results follow, as before.", true, nil
	}
	msg += "\n\nThe server checks each delegated run in its worktree against its contract (changed files, allowed paths, test command) and merges the ones that passed into your branch before you resume; those results follow. " +
		"Don't repeat checks that already ran or re-read the ticket you already have. Read the diff for what they can't judge, verify the acceptance criteria, and delegate again (continue_run_id sends fixes to the same sub agent) or fix small remaining work yourself. " +
		"When every criterion is verified and checked, commit and push any changes, record the PR with set_pr, and finish with complete_run status=done (delegate a review first only if the change is risky or large)."
	return msg, true, nil
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
