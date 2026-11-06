// Package runs is the run lifecycle shared by the Agent API (MCP and REST)
// and the people supervising it (PLAN.md §5.2, §6.4): claiming tickets,
// plans, usage, approvals and the run_gated broker, waiting and resuming.
package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/chatmodels"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

// Timeline verbs written by runs (see tracker/activity.go for the rest).
const (
	VerbRunStarted        = "run_started"        // {run_id, mode}
	VerbPlanPosted        = "plan_posted"        // {run_id, steps}
	VerbTestsRan          = "tests_ran"          // {run_id, passed, failed, duration_s}
	VerbBranchSet         = "branch_set"         // {branch}
	VerbPROpened          = "pr_opened"          // {number, url}
	VerbApprovalRequested = "approval_requested" // {approval_id, scope, summary}
	VerbApprovalApproved  = "approval_approved"  // {approval_id, scope, note}
	VerbApprovalDenied    = "approval_denied"    // {approval_id, scope, note}
	VerbGatedRan          = "gated_action_ran"   // {approval_id, scope, ok, summary}
	VerbWaitingForHuman   = "waiting_for_human"  // {run_id, reason}
	VerbRunPaused         = "run_paused"
	VerbRunResumed        = "run_resumed"
	VerbRunCancelled      = "run_cancelled"
	VerbRunCompleted      = "run_completed" // {run_id, outcome}
	VerbDispatched        = "dispatched"    // {run_id, agent_id, agent_name}
	VerbRunFailed         = "run_failed"    // {run_id, reason}
	VerbPRMerged          = "pr_merged"     // {number, url} (system)
	VerbPRClosed          = "pr_closed"     // {number, url} (system)
)

var Terminal = []string{"completed", "failed", "cancelled"}

type Service struct {
	Pool      *pgxpool.Pool
	Secrets   *secrets.Service
	Artifacts *artifacts.Service
	Tracker   *tracker.Service
	// RequireHarnessKey mirrors the runner's setting: chats can't select a
	// harness whose API-key secret is missing.
	RequireHarnessKey bool
	// OpenCodeIsolated mirrors the runner's RUNNER_OPENCODE_ISOLATED: chats can
	// select OpenCode only where isolated config is attested.
	OpenCodeIsolated bool
	// ModelDiscovery overrides live provider discovery for embedded backends and tests.
	ModelDiscovery func(context.Context, string, string) ([]chatmodels.Model, error)
	// Poll is how often waits re-check state (tests shorten it).
	Poll time.Duration
	// HandoffChars bounds the prior transcript handed to a chat segment started
	// by a harness switch (0: DefaultHandoffChars).
	HandoffChars int
}

type Caller = agents.Caller

func (s *Service) tx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(store.New(tx)) })
}

func agentActor(c Caller) tracker.Actor { return tracker.AgentActor(c.Agent.ID) }

func event(ctx context.Context, q *store.Queries, runID uuid.UUID, typ string, payload map[string]any) error {
	raw, _ := json.Marshal(payload)
	return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: runID, Type: typ, Payload: raw})
}

// RecomputeAgentStatus derives running/waiting/blocked/idle from live runs.
func RecomputeAgentStatus(ctx context.Context, q *store.Queries, agentID uuid.UUID) error {
	live, err := q.ActiveRunsForAgent(ctx, agentID)
	if err != nil {
		return err
	}
	status := "idle"
	rank := map[string]int{"idle": 0, "waiting": 1, "blocked": 2, "running": 3}
	for _, r := range live {
		if r.Purpose == "chat" && r.State == "waiting" {
			continue // idle chats do not make the ticket agent busy
		}
		st := "running"
		switch r.State {
		case "queued":
			continue // dispatched, not started: the agent isn't busy yet
		case "waiting", "paused":
			st = "waiting"
		case "blocked":
			st = "blocked"
		}
		if rank[st] > rank[status] {
			status = st
		}
	}
	return q.SetAgentStatus(ctx, store.SetAgentStatusParams{ID: agentID, Status: status})
}

// ---------------------------------------------------------------- claiming

// Claim opens a run on a ticket assigned to the calling agent, freezing the
// resolved definition (§6.4). Claiming a ticket the agent already runs
// returns the live run, so reconnecting sessions can pick up where they were.
func (s *Service) Claim(ctx context.Context, c Caller, key string) (store.Run, error) {
	ws := c.Agent.WorkspaceID
	var run store.Run
	moved := false
	err := s.tx(ctx, func(q *store.Queries) error {
		t, err := q.LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: ws, Key: strings.ToUpper(strings.TrimSpace(key))})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("ticket", "no ticket %s", key)
		} else if err != nil {
			return err
		}
		if c.Agent.Kind == agents.KindSub {
			return apperr.Conflict("@%s is a sub agent: it runs when a main agent delegates to it, not from a ticket", c.Agent.Handle)
		}
		if t.AssigneeAgentID == nil || *t.AssigneeAgentID != c.Agent.ID {
			return apperr.Conflict("%s isn't assigned to @%s. A person assigns tickets; ask for it in a comment.", t.Key, c.Agent.Handle)
		}
		if live, err := q.ActiveRunForTicket(ctx, &t.ID); err == nil {
			if live.AgentID != c.Agent.ID {
				return apperr.Conflict("%s already has a run by another agent", t.Key)
			}
			if c.RunID != nil && *c.RunID != live.ID {
				return apperr.Conflict("this token belongs to a different run")
			}
			if live.Mode == "managed" && c.RunID == nil {
				return apperr.Conflict("%s was dispatched to the runner; it runs there, not in this session", t.Key)
			}
			run = live
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if t.Status == "done" {
			return apperr.Conflict("%s is done", t.Key)
		}
		p, err := q.LockProject(ctx, store.LockProjectParams{ID: t.ProjectID, WorkspaceID: ws})
		if err != nil {
			return err
		}
		blockers, err := tracker.DispatchBlockers(ctx, q, t)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			return apperr.Conflict("%s", strings.Join(blockers, "; "))
		}
		resolved, err := agents.Resolve(ctx, q, ws, c.Agent, p.Key)
		if err != nil {
			return err
		}
		resolved.Repository = &agentdef.Repository{URL: p.RepoUrl, DefaultBranch: p.DefaultBranch}
		raw, _ := json.Marshal(resolved)
		mode := "managed"
		if c.Kind == "unmanaged" {
			mode = "unmanaged"
		}
		run, err = q.CreateRun(ctx, store.CreateRunParams{
			WorkspaceID: ws, TicketID: &t.ID, AgentID: c.Agent.ID, Mode: mode, State: "running",
			ResolvedSpec: raw, DefinitionVersionID: c.Agent.CurrentDefinitionVersionID, Harness: resolved.Harness, Model: resolved.Model, Purpose: "ticket",
		})
		if err != nil {
			return err
		}
		if _, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "start"}); err != nil {
			return err
		}
		if err := event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "running", "mode": mode}); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, ws, t.ID, t.EpicID, agentActor(c), VerbRunStarted, map[string]any{"run_id": run.ID, "mode": mode}, nil); err != nil {
			return err
		}
		moved = t.Status == "backlog" || t.Status == "todo"
		return RecomputeAgentStatus(ctx, q, c.Agent.ID)
	})
	if err == nil && moved {
		st := "in_progress"
		_, err = s.Tracker.Update(ctx, ws, agentActor(c), key, tracker.Patch{Status: &st})
	}
	return run, err
}

// Dispatch queues managed runs for tickets' assigned agents (§6.6): a
// person's check before any agent runs. All or nothing.
func (s *Service) Dispatch(ctx context.Context, ws, userID uuid.UUID, keys []string) ([]store.Run, error) {
	if len(keys) == 0 {
		return nil, apperr.Invalid("keys", "pick at least one ticket")
	}
	var out []store.Run
	err := s.tx(ctx, func(q *store.Queries) error {
		for _, key := range keys {
			t, err := q.LockTicketByKey(ctx, store.LockTicketByKeyParams{WorkspaceID: ws, Key: strings.ToUpper(strings.TrimSpace(key))})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Invalid("keys", "no ticket %s", key)
			} else if err != nil {
				return err
			}
			if t.AssigneeAgentID == nil {
				return apperr.Invalid("keys", "%s has no agent; assign one first", t.Key)
			}
			if t.Status == "done" {
				return apperr.Conflict("%s is done", t.Key)
			}
			if live, err := q.ActiveRunForTicket(ctx, &t.ID); err == nil {
				return apperr.Conflict("%s already has a %s run", t.Key, live.State)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			agent, err := q.GetAgent(ctx, store.GetAgentParams{ID: *t.AssigneeAgentID, WorkspaceID: ws})
			if err != nil {
				return err
			}
			if agent.Kind == agents.KindSub {
				return apperr.Conflict("%s is assigned to @%s, a sub agent; assign a main agent, which can delegate to it", t.Key, agent.Handle)
			}
			p, err := q.LockProject(ctx, store.LockProjectParams{ID: t.ProjectID, WorkspaceID: ws})
			if err != nil {
				return err
			}
			// Unfinished dependencies don't refuse the dispatch: the run stays
			// queued until its blockers are Done (see ClaimQueuedRun).
			blockers, err := tracker.HardBlockers(ctx, q, t)
			if err != nil {
				return err
			}
			if len(blockers) > 0 {
				return apperr.Conflict("%s: %s", t.Key, strings.Join(blockers, "; "))
			}
			resolved, err := agents.ResolveRunnable(ctx, q, ws, agent, p.Key)
			if err != nil {
				return err
			}
			resolved.Repository = &agentdef.Repository{URL: p.RepoUrl, DefaultBranch: p.DefaultBranch}
			raw, _ := json.Marshal(resolved)
			run, err := q.CreateRun(ctx, store.CreateRunParams{
				WorkspaceID: ws, TicketID: &t.ID, AgentID: agent.ID, Mode: "managed", State: "queued",
				ResolvedSpec: raw, DefinitionVersionID: agent.CurrentDefinitionVersionID, Harness: resolved.Harness, Model: resolved.Model, Purpose: "ticket",
			})
			if err != nil {
				return err
			}
			if err := q.SetRunDispatcher(ctx, store.SetRunDispatcherParams{ID: run.ID, DispatchedByUserID: &userID}); err != nil {
				return err
			}
			if _, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "start", InputMessage: StartPrompt(t.Key)}); err != nil {
				return err
			}
			if err := event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "queued", "mode": "managed"}); err != nil {
				return err
			}
			if err := tracker.Record(ctx, q, ws, t.ID, t.EpicID, tracker.UserActor(userID), VerbDispatched,
				map[string]any{"run_id": run.ID, "agent_id": agent.ID, "agent_name": agent.Name}, nil); err != nil {
				return err
			}
			if err := RecomputeAgentStatus(ctx, q, agent.ID); err != nil {
				return err
			}
			out = append(out, run)
		}
		return nil
	})
	return out, err
}

// StartPrompt is the first message of a managed run. The rest of what the
// agent needs is in AGENTS.md and the LazyAgents tools.
func StartPrompt(key string) string {
	return "You've been dispatched to work on " + key + ". Follow AGENTS.md: call claim_ticket(\"" + key +
		"\") first, read the memory and context it gives you, post_plan, then work. When you need a person, " +
		"call request_approval or await_human and end your turn. Verify every acceptance criterion; implement or delegate any gaps. " +
		"Once all criteria are verified and checked, commit and push any changes within your authorized scopes, record the PR, obtain delegated reviewer/QA approval, and finish with complete_run(status=done)."
}

// Active resolves which run a tool call acts on: the token's run, the run on
// the named ticket, or the caller's only live run.
func (s *Service) Active(ctx context.Context, q *store.Queries, c Caller, ticketKey string) (store.Run, store.Ticket, error) {
	ws := c.Agent.WorkspaceID
	var run store.Run
	var err error
	switch {
	case c.RunID != nil:
		run, err = q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: ws})
	case ticketKey != "":
		var t store.Ticket
		t, err = tracker.Get(ctx, q, ws, strings.ToUpper(strings.TrimSpace(ticketKey)))
		if err != nil {
			return run, t, apperr.Invalid("ticket", "no ticket %s", ticketKey)
		}
		run, err = q.ActiveRunForTicket(ctx, &t.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return run, t, apperr.Conflict("no live run on %s: call claim_ticket first", t.Key)
		}
	default:
		live, lerr := q.ActiveRunsForAgent(ctx, c.Agent.ID)
		if lerr != nil {
			return run, store.Ticket{}, lerr
		}
		switch len(live) {
		case 0:
			return run, store.Ticket{}, apperr.Conflict("no live run: call claim_ticket first")
		case 1:
			run, err = q.GetRun(ctx, store.GetRunParams{ID: live[0].ID, WorkspaceID: ws})
		default:
			keys := make([]string, len(live))
			for i, r := range live {
				keys[i] = r.TicketKey
			}
			return run, store.Ticket{}, apperr.Invalid("ticket", "you have several live runs (%s): pass ticket", strings.Join(keys, ", "))
		}
	}
	if err != nil {
		return run, store.Ticket{}, err
	}
	if run.AgentID != c.Agent.ID {
		return run, store.Ticket{}, apperr.ErrNotFound
	}
	if run.TicketID == nil {
		return run, store.Ticket{}, apperr.Conflict("planning runs use propose_tickets")
	}
	t, err := q.GetTicketByID(ctx, *run.TicketID)
	return run, t, err
}

// running loads the active run and requires it to be running.
func (s *Service) running(ctx context.Context, q *store.Queries, c Caller, key string) (store.Run, store.Ticket, error) {
	run, t, err := s.Active(ctx, q, c, key)
	if err != nil {
		return run, t, err
	}
	switch run.State {
	case "running":
		return run, t, nil
	case "waiting":
		return run, t, apperr.Conflict("the run is waiting for a person (%s); call await_signal", deref(run.WaitReason))
	case "paused":
		return run, t, apperr.Conflict("a person paused this run; stop and call await_signal")
	default:
		return run, t, apperr.Conflict("the run is %s", run.State)
	}
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// Spec decodes a run's frozen resolved definition.
func Spec(run store.Run) (agentdef.Resolved, error) {
	var r agentdef.Resolved
	err := json.Unmarshal(run.ResolvedSpec, &r)
	return r, err
}

// ---------------------------------------------------------------- plan

func (s *Service) PostPlan(ctx context.Context, c Caller, key string, steps []string) ([]store.PlanStep, error) {
	clean := make([]string, 0, len(steps))
	for _, st := range steps {
		if st = strings.TrimSpace(st); st != "" {
			if len([]rune(st)) > 500 {
				return nil, apperr.Invalid("steps", "each step is limited to 500 characters")
			}
			clean = append(clean, st)
		}
	}
	if len(clean) == 0 || len(clean) > 30 {
		return nil, apperr.Invalid("steps", "post 1–30 steps")
	}
	var out []store.PlanStep
	err := s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if err := q.DeletePlan(ctx, run.ID); err != nil {
			return err
		}
		for i, st := range clean {
			if err := q.InsertPlanStep(ctx, store.InsertPlanStepParams{RunID: run.ID, Position: int32(i + 1), Text: st, State: "pending"}); err != nil {
				return err
			}
		}
		if err := event(ctx, q, run.ID, "message", map[string]any{"kind": "plan", "steps": clean}); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbPlanPosted, map[string]any{"run_id": run.ID, "steps": len(clean)}, nil); err != nil {
			return err
		}
		out, err = q.ListPlan(ctx, run.ID)
		return err
	})
	return out, err
}

var stepStates = []string{"pending", "current", "done", "needs_human", "failed"}

// UpdateStep sets one step's state. Marking a step current clears any other
// current step, so the plan always shows one place the agent is at.
func (s *Service) UpdateStep(ctx context.Context, c Caller, key string, n int, state string) error {
	if !slices.Contains(stepStates, state) {
		return apperr.Invalid("state", "must be one of %s", strings.Join(stepStates, ", "))
	}
	return s.tx(ctx, func(q *store.Queries) error {
		run, _, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if state == "current" {
			if err := q.ClearCurrentStep(ctx, store.ClearCurrentStepParams{RunID: run.ID, Position: int32(n)}); err != nil {
				return err
			}
		}
		rows, err := q.SetStepState(ctx, store.SetStepStateParams{RunID: run.ID, Position: int32(n), State: state})
		if err != nil {
			return err
		}
		if rows == 0 {
			return apperr.Invalid("step", "no step %d in the plan", n)
		}
		return event(ctx, q, run.ID, "message", map[string]any{"kind": "step", "step": n, "state": state})
	})
}

// ---------------------------------------------------------------- events

// LogEvent records a rich timeline event. kind "test_run" takes
// {passed, failed, duration_s, command}; anything else goes to the run log only.
func (s *Service) LogEvent(ctx context.Context, c Caller, key, kind string, payload map[string]any) error {
	kind = strings.TrimSpace(kind)
	if kind == "" || len(kind) > 40 {
		return apperr.Invalid("kind", "is required")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if err := event(ctx, q, run.ID, "message", map[string]any{"kind": kind, "payload": payload}); err != nil {
			return err
		}
		if kind != "test_run" {
			return nil
		}
		num := func(k string) any {
			if v, ok := payload[k].(float64); ok {
				return v
			}
			return 0
		}
		p := map[string]any{"run_id": run.ID, "passed": num("passed"), "failed": num("failed"), "duration_s": num("duration_s")}
		if cmd, ok := payload["command"].(string); ok {
			p["command"] = cmd
		}
		return tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbTestsRan, p, nil)
	})
}

// ReportUsage adds self-reported usage. Only unmanaged sessions report;
// managed runs read usage from the harness stream (Phase 5).
func (s *Service) ReportUsage(ctx context.Context, c Caller, key string, toolCalls, inTok, outTok int64, costUSD float64) error {
	if toolCalls < 0 || inTok < 0 || outTok < 0 || costUSD < 0 {
		return apperr.Invalid("usage", "values can't be negative")
	}
	return s.tx(ctx, func(q *store.Queries) error {
		run, _, err := s.Active(ctx, q, c, key)
		if err != nil {
			return err
		}
		if run.Mode != "unmanaged" {
			return apperr.Conflict("managed runs get usage from the harness")
		}
		src := "reported"
		_, err = q.AddRunUsage(ctx, store.AddRunUsageParams{ID: run.ID, ToolCalls: int32(toolCalls), InputTokens: inTok, OutputTokens: outTok,
			CostUsdMicros: int64(costUSD * 1e6), CostSource: &src})
		return err
	})
}

var branchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func (s *Service) SetBranch(ctx context.Context, c Caller, key, branch string) error {
	branch = strings.TrimSpace(branch)
	if !branchRe.MatchString(branch) || strings.Contains(branch, "..") {
		return apperr.Invalid("branch", "isn't a valid branch name")
	}
	return s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if err := q.SetTicketBranch(ctx, store.SetTicketBranchParams{ID: t.ID, Branch: &branch}); err != nil {
			return err
		}
		return tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbBranchSet, map[string]any{"branch": branch}, nil)
	})
}

var prURLRe = regexp.MustCompile(`^https://github\.com/[\w.-]+/[\w.-]+/pull/(\d+)$`)

// SetPR records the ticket's pull request. The PR sync job (Phase 5) then
// keeps its state and diff stats current.
func (s *Service) SetPR(ctx context.Context, c Caller, key, url string, draft bool) error {
	m := prURLRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return apperr.Invalid("url", "must be a GitHub pull request URL")
	}
	var num int32
	fmt.Sscan(m[1], &num)
	state := "open"
	if draft {
		state = "draft"
	}
	return s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if err := requireMockup(ctx, q, run, t); err != nil {
			return err
		}
		u := m[0]
		if err := q.SetTicketPR(ctx, store.SetTicketPRParams{ID: t.ID, PrNumber: &num, PrUrl: &u, PrState: &state}); err != nil {
			return err
		}
		return tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbPROpened, map[string]any{"number": num, "url": u, "draft": draft}, nil)
	})
}

// CheckCriterion ticks acceptance criterion n (1-based, in display order).
func (s *Service) CheckCriterion(ctx context.Context, c Caller, key string, n int, done bool) error {
	var id uuid.UUID
	err := s.tx(ctx, func(q *store.Queries) error {
		run, t, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		if done {
			if err := requireMockup(ctx, q, run, t); err != nil {
				return err
			}
		}
		if n < 1 {
			return apperr.Invalid("n", "criteria are numbered from 1")
		}
		cr, err := q.GetCriterionByPosition(ctx, store.GetCriterionByPositionParams{TicketID: t.ID, Offset: int32(n - 1)})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("n", "no criterion %d", n)
		}
		id = cr.ID
		return err
	})
	if err != nil {
		return err
	}
	_, err = s.Tracker.UpdateCriterion(ctx, c.Agent.WorkspaceID, agentActor(c), id, nil, &done)
	return err
}

// CreateReport stores an agent-authored REPORT and attaches it to the ticket
// as generated output ("Generated by Atlas").
func (s *Service) CreateReport(ctx context.Context, c Caller, key, title, body string) (store.Artifact, error) {
	var t store.Ticket
	var projectKey string
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		_, t, err = s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		p, err := q.GetProject(ctx, store.GetProjectParams{ID: t.ProjectID, WorkspaceID: t.WorkspaceID})
		projectKey = p.Key
		return err
	})
	if err != nil {
		return store.Artifact{}, err
	}
	a, err := s.Artifacts.Create(ctx, t.WorkspaceID, artifacts.Actor{Type: "agent", ID: c.Agent.ID}, artifacts.NewArtifact{
		ProjectKey: projectKey, Type: "report", Title: title, EpicID: t.EpicID, Content: artifacts.Content{BodyMD: &body},
	})
	if err != nil {
		return a, err
	}
	return a, s.Tracker.AttachArtifact(ctx, t.WorkspaceID, agentActor(c), t.Key, a.ID, "generated")
}

// Comment posts as the agent.
func (s *Service) Comment(ctx context.Context, c Caller, key, body string) (store.ActivityEvent, error) {
	var t store.Ticket
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		_, t, err = s.Active(ctx, q, c, key)
		return err
	})
	if err != nil {
		return store.ActivityEvent{}, err
	}
	return s.Tracker.Comment(ctx, t.WorkspaceID, agentActor(c), t.Key, tracker.NewComment{BodyMd: body})
}

// ---------------------------------------------------------------- finishing

// Review verdicts a delegated reviewer or QA run reports with complete_run.
const (
	VerdictApproved         = "approved"
	VerdictChangesRequested = "changes_requested"
)

// ReviewClear is nil when a main run may mark its ticket Done: at least one
// delegated reviewer/QA run reported a verdict, and each such agent's latest
// verdict is an all clear.
func ReviewClear(ctx context.Context, q *store.Queries, run store.Run) error {
	children, err := q.ChildRuns(ctx, &run.ID)
	if err != nil {
		return err
	}
	latest := map[uuid.UUID]string{}
	for _, ch := range children {
		if ch.State != "completed" {
			continue
		}
		v, err := q.RunVerdict(ctx, ch.ID)
		if errors.Is(err, pgx.ErrNoRows) || v == "" {
			continue
		} else if err != nil {
			return err
		}
		latest[ch.AgentID] = v
	}
	if len(latest) == 0 {
		return apperr.Conflict("a ticket can only be marked done after a delegated reviewer or QA agent gives the all clear (complete_run with verdict=approved). Delegate a review first, or finish with status in_review")
	}
	for _, v := range latest {
		if v != VerdictApproved {
			return apperr.Conflict("a reviewer requested changes; address them and have it re-review, or finish with status in_review")
		}
	}
	return nil
}

// Complete ends the run and moves the ticket (default: In Review). Done needs
// the reviewers' all clear; verdict is only for delegated reviewer/QA runs.
func (s *Service) Complete(ctx context.Context, c Caller, key, outcome, status, verdict string) error {
	if status == "" {
		status = "in_review"
	}
	if status != "in_review" && status != "done" && status != "todo" {
		return apperr.Invalid("status", "must be in_review, done, or todo (to hand back)")
	}
	if verdict != "" && verdict != VerdictApproved && verdict != VerdictChangesRequested {
		return apperr.Invalid("verdict", "must be approved or changes_requested")
	}
	outcome = strings.TrimSpace(outcome)
	if c.RunID != nil {
		// A delegated run reports back to its parent instead of moving the ticket.
		var delegated bool
		if err := s.tx(ctx, func(q *store.Queries) error {
			r, err := q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: c.Agent.WorkspaceID})
			delegated = err == nil && r.ParentRunID != nil
			return nil
		}); err != nil {
			return err
		}
		if delegated {
			return s.completeChild(ctx, c, key, outcome, verdict)
		}
	}
	var t store.Ticket
	err := s.tx(ctx, func(q *store.Queries) error {
		run, tk, err := s.running(ctx, q, c, key)
		if err != nil {
			return err
		}
		t = tk
		if verdict != "" {
			return apperr.Invalid("verdict", "only a delegated reviewer or QA run reports a verdict")
		}
		if status == "in_review" || status == "done" {
			if err := requireMockup(ctx, q, run, tk); err != nil {
				return err
			}
			criteria, err := q.ListCriteria(ctx, tk.ID)
			if err != nil {
				return err
			}
			var unfinished []string
			for i, criterion := range criteria {
				if !criterion.Done {
					unfinished = append(unfinished, fmt.Sprintf("%d", i+1))
				}
			}
			if len(unfinished) > 0 {
				return apperr.Conflict("acceptance criteria %s are unchecked; verify each criterion and call check_criterion only when satisfied. Implement missing work yourself or delegate it, then retry complete_run. If blocked on a person, use await_human or request_approval; use status=todo only to explicitly hand back unfinished work", strings.Join(unfinished, ", "))
			}
		}
		if status == "done" {
			if err := ReviewClear(ctx, q, run); err != nil {
				return err
			}
		}
		if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
			return err
		}
		if err := q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](0)}); err != nil {
			return err
		}
		if err := q.ExpireRunApprovals(ctx, run.ID); err != nil {
			return err
		}
		if err := event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "completed", "outcome": outcome}); err != nil {
			return err
		}
		if err := tracker.Record(ctx, q, run.WorkspaceID, t.ID, t.EpicID, agentActor(c), VerbRunCompleted, map[string]any{"run_id": run.ID, "outcome": outcome}, nil); err != nil {
			return err
		}
		return RecomputeAgentStatus(ctx, q, c.Agent.ID)
	})
	if err != nil {
		return err
	}
	_, err = s.Tracker.Update(ctx, t.WorkspaceID, agentActor(c), t.Key, tracker.Patch{Status: &status})
	return err
}

func ptr[T any](v T) *T { return &v }
