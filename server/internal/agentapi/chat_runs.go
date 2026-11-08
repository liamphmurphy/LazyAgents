package agentapi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Run tools: the Personal Assistant reads the agent runs of its chat's
// project. Only ticket runs are listed: planning sessions and other people's
// chats stay private.

var (
	chatLiveRunStates     = []string{"queued", "starting", "running", "waiting", "waiting_for_quota", "paused", "blocked"}
	chatFinishedRunStates = []string{"completed", "failed", "cancelled"}
)

type ListRunsIn struct {
	State  string `json:"state,omitempty" jsonschema:"live (default: queued, running, waiting, paused or blocked), finished (completed, failed or cancelled), all, or one exact state"`
	Ticket string `json:"ticket,omitempty" jsonschema:"Only runs of this ticket, e.g. PAY-214 (includes delegated sub agent runs)"`
	Agent  string `json:"agent,omitempty" jsonschema:"Only runs of this agent handle, e.g. implementer"`
	Offset int    `json:"offset,omitempty" jsonschema:"Skip this many runs (pages of 50, newest first)"`
}

type GetRunIn struct {
	ID string `json:"id" jsonschema:"Run id from list_runs"`
}

// chatRun is a run as the assistant sees it.
type chatRun struct {
	ID         uuid.UUID  `json:"id"`
	Ticket     string     `json:"ticket,omitempty"`
	Title      string     `json:"ticket_title,omitempty"`
	Agent      string     `json:"agent"`
	State      string     `json:"state"`
	WaitReason string     `json:"wait_reason,omitempty"`
	Outcome    string     `json:"outcome,omitempty"`
	Harness    string     `json:"harness"`
	Model      string     `json:"model"`
	ParentRun  *uuid.UUID `json:"parent_run_id,omitempty"`
	StartedAt  string     `json:"started_at"`
	EndedAt    string     `json:"ended_at,omitempty"`
	CostUSD    float64    `json:"cost_usd"`
	ToolCalls  int32      `json:"tool_calls"`
}

// runNamer looks up and caches ticket keys and agent handles for runs.
type runNamer struct {
	d       Deps
	ws      uuid.UUID
	tickets map[uuid.UUID]store.Ticket
	agents  map[uuid.UUID]string
}

func (n *runNamer) toChatRun(ctx context.Context, r store.Run) (chatRun, error) {
	out := chatRun{ID: r.ID, State: r.State, WaitReason: strOr(r.WaitReason), Outcome: strOr(r.Outcome), Harness: r.Harness, Model: r.Model,
		ParentRun: r.ParentRunID, StartedAt: r.StartedAt.Format(time.RFC3339), CostUSD: float64(r.CostUsdMicros) / 1e6, ToolCalls: r.ToolCalls}
	if r.EndedAt != nil {
		out.EndedAt = r.EndedAt.Format(time.RFC3339)
	}
	if r.TicketID != nil {
		t, ok := n.tickets[*r.TicketID]
		if !ok {
			var err error
			if t, err = n.d.Q.GetTicketByID(ctx, *r.TicketID); err != nil {
				return out, err
			}
			n.tickets[t.ID] = t
		}
		out.Ticket, out.Title = t.Key, t.Title
	}
	h, ok := n.agents[r.AgentID]
	if !ok {
		a, err := n.d.Q.GetAgent(ctx, store.GetAgentParams{ID: r.AgentID, WorkspaceID: n.ws})
		if err != nil {
			return out, err
		}
		h = a.Handle
		n.agents[r.AgentID] = h
	}
	out.Agent = h
	return out, nil
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func requireChat(ctx context.Context, d Deps, c agents.Caller) (*chatScope, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err == nil && s == nil {
		err = apperr.Conflict("this tool requires a chat")
	}
	return s, err
}

func listChatRuns(ctx context.Context, d Deps, c agents.Caller, in ListRunsIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	if in.Offset < 0 || in.Offset > 100000 {
		return nil, apperr.Invalid("offset", "must be between 0 and 100000")
	}
	purpose := "ticket"
	arg := store.ListRunsParams{WorkspaceID: ws, Purpose: &purpose, ProjectID: &s.ProjectID, Skip: int32(in.Offset)}
	switch st := strings.ToLower(strings.TrimSpace(in.State)); st {
	case "", "live":
		arg.States = chatLiveRunStates
	case "finished":
		arg.States = chatFinishedRunStates
	case "all":
		arg.States = append(append([]string{}, chatLiveRunStates...), chatFinishedRunStates...)
	default:
		if !slices.Contains(chatLiveRunStates, st) && !slices.Contains(chatFinishedRunStates, st) {
			return nil, apperr.Invalid("state", "use live, finished, all or a run state")
		}
		arg.States = []string{st}
	}
	empty := map[string]any{"runs": []chatRun{}, "has_more": false}
	if key := strings.ToUpper(strings.TrimSpace(in.Ticket)); key != "" {
		t, err := d.Q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: key})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !s.allowsTicket(t)) {
			return nil, apperr.Invalid("ticket", "no ticket %s in project %s", key, s.ProjectKey)
		} else if err != nil {
			return nil, err
		}
		arg.TicketID = &t.ID
	}
	if h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Agent), "@")); h != "" {
		a, err := d.Q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: h})
		if errors.Is(err, pgx.ErrNoRows) {
			return empty, nil
		} else if err != nil {
			return nil, err
		}
		arg.AgentID = &a.ID
	}
	rows, err := d.Q.ListRuns(ctx, arg)
	if err != nil {
		return nil, err
	}
	more := len(rows) > 50
	if more {
		rows = rows[:50]
	}
	n := &runNamer{d: d, ws: ws, tickets: map[uuid.UUID]store.Ticket{}, agents: map[uuid.UUID]string{}}
	out := make([]chatRun, 0, len(rows))
	for _, r := range rows {
		cr, err := n.toChatRun(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return map[string]any{"runs": out, "has_more": more}, nil
}

func getChatRun(ctx context.Context, d Deps, c agents.Caller, in GetRunIn) (any, error) {
	s, err := requireChat(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	id, err := uuid.Parse(strings.TrimSpace(in.ID))
	if err != nil {
		return nil, apperr.Invalid("id", "not a run id")
	}
	r, err := d.Q.GetRun(ctx, store.GetRunParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && r.Purpose != "ticket") {
		return nil, apperr.Invalid("id", "no run %s in project %s", in.ID, s.ProjectKey)
	} else if err != nil {
		return nil, err
	}
	n := &runNamer{d: d, ws: ws, tickets: map[uuid.UUID]store.Ticket{}, agents: map[uuid.UUID]string{}}
	run, err := n.toChatRun(ctx, r)
	if err != nil {
		return nil, err
	}
	if !s.allowsTicket(n.tickets[*r.TicketID]) {
		return nil, apperr.Invalid("id", "no run %s in project %s", in.ID, s.ProjectKey)
	}
	steps, err := d.Q.ListPlan(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	plan := make([]map[string]any, len(steps))
	for i, st := range steps {
		plan[i] = map[string]any{"n": i + 1, "text": st.Text, "state": st.State}
	}
	kids, err := d.Q.ChildRuns(ctx, &r.ID)
	if err != nil {
		return nil, err
	}
	children := make([]chatRun, 0, len(kids))
	for _, k := range kids {
		cr, err := n.toChatRun(ctx, k)
		if err != nil {
			return nil, err
		}
		children = append(children, cr)
	}
	return map[string]any{"run": run, "plan": plan, "delegated_runs": children,
		"input_tokens": r.InputTokens, "output_tokens": r.OutputTokens}, nil
}
