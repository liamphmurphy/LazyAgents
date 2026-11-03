// Package agentapi is the Agent API (PLAN.md §5.2): one table of tools,
// served over MCP at /mcp and mirrored as REST at /api/v1/agent/tools/{name}.
// Every call authenticates with an agent bearer token (la_…).
package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
	"github.com/liamphmurphy/lazyagents/server/internal/wiki"
)

type Deps struct {
	Q       *store.Queries
	Runs    *runs.Service
	Tracker *tracker.Service
	Wiki    *wiki.Service
}

// Tool is one Agent API operation, independent of transport.
type Tool struct {
	Name        string
	Description string
	// register adds the tool to an MCP server and returns a REST invoker.
	call   func(ctx context.Context, d Deps, c agents.Caller, raw json.RawMessage) (any, error)
	addMCP func(srv mcpServer, d Deps)
}

// TicketArg is shared by tools that act on a run: optional when the caller
// has exactly one live run (or a run-scoped token).
type TicketArg struct {
	Ticket string `json:"ticket,omitempty" jsonschema:"Ticket key, e.g. PAY-214. Optional when you have one live run."`
}

type Empty struct{}

type WhoamiOut struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Handle       string         `json:"handle"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Instructions string         `json:"instructions"`
	Scopes       map[string]any `json:"scopes"`
	Session      string         `json:"session"`
	LiveRuns     []string       `json:"live_runs"`
	Queue        []QueueItem    `json:"queue"`
	Project      string         `json:"project,omitempty"` // chats only
	ChatID       string         `json:"chat_id,omitempty"` // chats only
}

type QueueItem struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

func whoami(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	spec, _, err := agents.CurrentSpec(ctx, d.Q, c.Agent)
	if err != nil {
		return nil, err
	}
	out := WhoamiOut{ID: c.Agent.ID.String(), Name: c.Agent.Name, Handle: c.Agent.Handle, Role: c.Agent.Role, Model: spec.Model,
		Instructions: spec.Instructions, Scopes: map[string]any{}, Session: c.Kind, LiveRuns: []string{}, Queue: []QueueItem{}}
	for s, m := range spec.Scopes {
		out.Scopes[s] = m
	}
	if c.RunID != nil {
		if run, err := d.Q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: c.Agent.WorkspaceID}); err == nil {
			if resolved, err := runs.Spec(run); err == nil {
				out.Model = resolved.Model
				out.Instructions = resolved.Instructions
				out.Scopes = map[string]any{}
				for _, sc := range resolved.Scopes {
					out.Scopes[sc.Scope] = sc.Mode
				}
			}
		}
	}
	// A chat has no tickets or queue: it answers a person from project context.
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if scope != nil {
		out.Instructions = fmt.Sprintf(chatInstructions, scope.ProjectKey, scope.ChatID) + "\n\n" + out.Instructions
		out.Project, out.ChatID = scope.ProjectKey, scope.ChatID.String()
		return out, nil
	}
	live, err := d.Q.ActiveRunsForAgent(ctx, c.Agent.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range live {
		out.LiveRuns = append(out.LiveRuns, r.TicketKey)
	}
	q, err := d.Q.AgentQueue(ctx, &c.Agent.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range q {
		out.Queue = append(out.Queue, QueueItem{Key: t.Key, Title: t.Title, Status: t.Status, Priority: t.Priority})
	}
	return out, nil
}

type ClaimIn struct {
	Ticket string `json:"ticket" jsonschema:"Ticket key to start working on, e.g. PAY-214"`
}

func claim(ctx context.Context, d Deps, c agents.Caller, in ClaimIn) (any, error) {
	run, err := d.Runs.Claim(ctx, c, in.Ticket)
	if err != nil {
		return nil, err
	}
	spec, _ := runs.Spec(run)
	scopes := map[string]string{}
	for _, s := range spec.Scopes {
		scopes[s.Scope] = string(s.Mode)
	}
	// Wiki memory this run starts with (PLAN.md §12.3); delivered pages
	// count as one read each.
	memory := []wiki.ContextPage{}
	if t, err := d.Q.GetTicketByID(ctx, *run.TicketID); err == nil {
		if memory, err = wiki.RunContext(ctx, d.Q, c.Agent, t, spec.Memory); err != nil {
			return nil, err
		}
		if err := wiki.RecordContextReads(ctx, d.Q, c.Agent.WorkspaceID, c.Agent.ID, &run.ID, memory); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"run_id": run.ID, "ticket": strings.ToUpper(in.Ticket), "state": run.State, "scopes": scopes,
		"instructions": spec.Instructions,
		"memory":       memory,
		"next": "Read memory first: it's what you and others learned before. Call get_ticket, read the attached artifacts with read_artifact, then post_plan before editing. " +
			"When you learn something worth remembering, write_note under " + wiki.MemoryRoot(c.Agent.Handle) + " and link it with [[…]] to the tickets and documents it's about.",
	}, nil
}

type GetTicketIn struct {
	Ticket string `json:"ticket" jsonschema:"Ticket key, e.g. PAY-214"`
}

func getTicket(ctx context.Context, d Deps, c agents.Caller, in GetTicketIn) (any, error) {
	ws := c.Agent.WorkspaceID
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	t, err := tracker.Get(ctx, d.Q, ws, strings.ToUpper(strings.TrimSpace(in.Ticket)))
	if err != nil || !scope.allowsTicket(t) {
		return nil, apperr.Invalid("ticket", "no ticket %s", in.Ticket)
	}
	crit, err := d.Q.ListCriteria(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	criteria := make([]map[string]any, len(crit))
	for i, cr := range crit {
		criteria[i] = map[string]any{"n": i + 1, "text": cr.Text, "done": cr.Done}
	}
	docs, err := d.Q.ListTicketArtifacts(ctx, store.ListTicketArtifactsParams{TicketID: t.ID, AssigneeAgentID: &c.Agent.ID})
	if err != nil {
		return nil, err
	}
	arts := make([]map[string]any, len(docs))
	for i, a := range docs {
		m := map[string]any{"id": a.ID, "type": a.Type, "title": a.Title, "relation": a.Relation, "read_by_you": a.ReadByAssignee}
		if id := artifacts.DisplayID(a.Type, a.Number); id != nil {
			m["display_id"] = *id
		}
		arts[i] = m
	}
	deps, err := d.Q.TicketDependencies(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	comments, err := d.Q.ListTicketComments(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	cs := make([]map[string]any, len(comments))
	for i := range comments {
		cm := comments[len(comments)-1-i] // oldest first
		cs[i] = map[string]any{"author": cm.Author, "author_type": cm.AuthorType, "body": cm.BodyMd, "at": cm.CreatedAt}
	}
	out := map[string]any{
		"key": t.Key, "title": t.Title, "description": t.DescriptionMd, "status": t.Status, "priority": t.Priority,
		"blocked_by": func() []map[string]any {
			out := []map[string]any{}
			for _, d := range deps {
				out = append(out, map[string]any{"key": d.Key, "title": d.Title, "status": d.Status})
			}
			return out
		}(), "criteria": criteria, "artifacts": arts, "comments": cs, "assigned_to_you": t.AssigneeAgentID != nil && *t.AssigneeAgentID == c.Agent.ID,
	}
	if t.Branch != nil {
		out["branch"] = *t.Branch
	}
	if t.PrUrl != nil {
		out["pr_url"] = *t.PrUrl
	}
	return out, nil
}

type ReadArtifactIn struct {
	TicketArg
	Artifact string `json:"artifact" jsonschema:"Artifact id from get_ticket, or an ADR id like ADR-012"`
	Section  string `json:"section,omitempty" jsonschema:"Optional section anchor from the outline, e.g. reconciliation-performance"`
}

// readRunID is the run a read is attributed to: the live run when there is
// one, else the token's own run (chat runs have no ticket for Active to find).
func readRunID(ctx context.Context, d Deps, c agents.Caller, ticket string) *uuid.UUID {
	if run, _, err := d.Runs.Active(ctx, d.Q, c, ticket); err == nil {
		return &run.ID
	}
	return c.RunID
}

func readArtifact(ctx context.Context, d Deps, c agents.Caller, in ReadArtifactIn) (any, error) {
	ws := c.Agent.WorkspaceID
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	var a store.Artifact
	ref := strings.TrimSpace(in.Artifact)
	if id, err := uuid.Parse(ref); err == nil {
		a, err = d.Q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws})
		if err != nil {
			return nil, apperr.Invalid("artifact", "no artifact %s", ref)
		}
	} else if n, ok := strings.CutPrefix(strings.ToUpper(ref), "ADR-"); ok {
		var num int32
		for _, ch := range n {
			if ch < '0' || ch > '9' {
				return nil, apperr.Invalid("artifact", "%s isn't an ADR id", ref)
			}
			num = num*10 + int32(ch-'0')
		}
		a, err = d.Q.GetADRByNumber(ctx, store.GetADRByNumberParams{WorkspaceID: ws, Number: &num})
		if err != nil {
			return nil, apperr.Invalid("artifact", "no %s", ref)
		}
	} else {
		return nil, apperr.Invalid("artifact", "pass an artifact id or ADR-NNN")
	}
	if !scope.allowsArtifact(a) {
		return nil, apperr.Invalid("artifact", "no artifact %s", ref)
	}
	if a.CurrentVersionID == nil {
		return nil, apperr.Invalid("artifact", "has no content")
	}
	v, err := d.Q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	if v.BodyMd == nil {
		return nil, apperr.Invalid("artifact", "%s is a %s file, not text; ask a person to describe it", a.Title, v.ContentType)
	}
	body := *v.BodyMd
	if in.Section != "" {
		sec, ok := artifacts.SectionText(body, in.Section)
		if !ok {
			return nil, apperr.Invalid("section", "no section %q; call read_artifact without section to see the outline", in.Section)
		}
		body = sec
	}
	// Reads are attributed to the run when there is one, for citation counts.
	runID := readRunID(ctx, d, c, in.Ticket)
	if err := artifacts.RecordRead(ctx, d.Q, a, c.Agent.ID, runID, in.Section, nil); err != nil {
		return nil, err
	}
	outline := []map[string]any{}
	for _, s := range artifacts.Outline(*v.BodyMd) {
		outline = append(outline, map[string]any{"level": s.Level, "heading": s.Heading, "anchor": s.Anchor})
	}
	return map[string]any{"title": a.Title, "type": a.Type, "version": v.Label, "markdown": body, "outline": outline}, nil
}

type PlanIn struct {
	TicketArg
	Steps []string `json:"steps" jsonschema:"The plan, one short sentence per step, in order"`
}

type StepIn struct {
	TicketArg
	Step  int    `json:"step" jsonschema:"Step number, from 1"`
	State string `json:"state" jsonschema:"pending, current, done, needs_human or failed"`
}

type LogEventIn struct {
	TicketArg
	Kind    string         `json:"kind" jsonschema:"test_run for test results (payload: passed, failed, duration_s, command); anything else is logged to the run only"`
	Payload map[string]any `json:"payload,omitempty"`
}

type UsageIn struct {
	TicketArg
	ToolCalls    int64   `json:"tool_calls,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

type BranchIn struct {
	TicketArg
	Branch string `json:"branch" jsonschema:"Branch name, e.g. atlas/pay-214-partition-ledger"`
}

type PRIn struct {
	TicketArg
	URL   string `json:"url" jsonschema:"GitHub pull request URL"`
	Draft bool   `json:"draft,omitempty"`
}

type CriterionIn struct {
	TicketArg
	N    int   `json:"n" jsonschema:"Criterion number from get_ticket, from 1"`
	Done *bool `json:"done,omitempty" jsonschema:"Default true; false unticks"`
}

type ReportIn struct {
	TicketArg
	Title string `json:"title"`
	Body  string `json:"body" jsonschema:"Markdown. Use ## headings so people and agents can cite sections."`
}

type CommentIn struct {
	TicketArg
	Body string `json:"body" jsonschema:"Markdown"`
}

type NewTicketIn struct {
	TicketArg
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty" jsonschema:"none, low, medium, high or urgent"`
}

type ApprovalIn struct {
	TicketArg
	Scope  string         `json:"scope" jsonschema:"The scope that needs approval, e.g. db:migrate"`
	Action string         `json:"action" jsonschema:"One sentence naming the exact action, e.g. Apply 0042_partition_ledger_entries to staging"`
	Detail map[string]any `json:"detail,omitempty" jsonschema:"For brokered scopes, exactly what to run: db:migrate takes {sql}"`
	Risk   string         `json:"risk,omitempty" jsonschema:"What could go wrong and how big it is"`
}

type WaitIn struct {
	ApprovalID string `json:"approval_id"`
	TimeoutS   int    `json:"timeout_s,omitempty" jsonschema:"Up to 50; call again if it returns pending"`
}

type GatedIn struct {
	TicketArg
	ApprovalID string `json:"approval_id" jsonschema:"An approved request from request_approval; runs exactly what was approved"`
}

type AwaitHumanIn struct {
	TicketArg
	Reason string `json:"reason" jsonschema:"What you need from a person"`
}

type SignalIn struct {
	TicketArg
	Since    string `json:"since,omitempty" jsonschema:"Cursor from the previous await_signal (RFC 3339); omit the first time"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"Up to 50"`
}

type CompleteIn struct {
	TicketArg
	Outcome string `json:"outcome" jsonschema:"One or two sentences on what you did"`
	Status  string `json:"status,omitempty" jsonschema:"in_review (default), done, or todo to hand the ticket back"`
}

type DelegateIn struct {
	TicketArg
	Agent string `json:"agent" jsonschema:"Handle of a sub agent from list_subagents, e.g. docs-writer"`
	Task  string `json:"task" jsonschema:"What it should do, with the context it needs: it can't see your conversation. Name files, constraints and the done condition."`
}

type SearchIn struct {
	Q       string   `json:"q" jsonschema:"What to look for; web-search syntax (quotes, OR, -word)"`
	Kinds   []string `json:"kinds,omitempty" jsonschema:"Limit to note, prd, spec, adr, report, design or ticket"`
	Project string   `json:"project,omitempty" jsonschema:"Project key; notes without a project always match"`
	Limit   int      `json:"limit,omitempty" jsonschema:"Up to 50, default 10"`
}

type PageIn struct {
	TicketArg
	Ref     string `json:"ref" jsonschema:"A page title or path, ADR-012, an artifact id, or [[Page#Section]]"`
	Section string `json:"section,omitempty" jsonschema:"Optional heading (or its anchor) to read just that section"`
}

type WriteNoteIn struct {
	TicketArg
	Path string `json:"path" jsonschema:"Page path, e.g. agents/atlas/Partition gotchas (your memory) or Payments/Rollback runbook (shared)"`
	Body string `json:"body" jsonschema:"Markdown. Link what it's about with [[PAY-214]], [[ADR-012]], [[Page title]] or [[Page#Section]]."`
	Mode string `json:"mode,omitempty" jsonschema:"create, replace (default) or append"`
	Note string `json:"note,omitempty" jsonschema:"One line on what changed"`
}

type BacklinksIn struct {
	Ref string `json:"ref" jsonschema:"A page title or path, ADR-012, PAY-214, an epic name, @handle or an artifact id"`
}

// scopesFor is the caller's scope map: the live run's frozen definition if
// there is one, else the agent's current definition.
func scopesFor(ctx context.Context, d Deps, c agents.Caller, ticket string) (map[string]agentdef.Mode, *uuid.UUID, error) {
	out := map[string]agentdef.Mode{}
	if run, _, err := d.Runs.Active(ctx, d.Q, c, ticket); err == nil {
		spec, err := runs.Spec(run)
		if err != nil {
			return nil, nil, err
		}
		for _, s := range spec.Scopes {
			out[s.Scope] = s.Mode
		}
		return out, &run.ID, nil
	}
	spec, _, err := agents.CurrentSpec(ctx, d.Q, c.Agent)
	if err != nil {
		return nil, nil, err
	}
	return spec.Scopes, nil, nil
}

func searchKnowledge(ctx context.Context, d Deps, c agents.Caller, in SearchIn) (any, error) {
	if strings.TrimSpace(in.Q) == "" {
		return nil, apperr.Invalid("q", "is required")
	}
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	var project *uuid.UUID
	if scope != nil {
		// A chat searches its own project (plus project-less pages); a project argument can't widen that.
		project = &scope.ProjectID
		if in.Project != "" && !strings.EqualFold(strings.TrimSpace(in.Project), scope.ProjectKey) {
			return nil, apperr.Invalid("project", "chats can only search project %s", scope.ProjectKey)
		}
	} else if in.Project != "" {
		p, err := d.Q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: c.Agent.WorkspaceID, Key: strings.ToUpper(in.Project)})
		if err != nil {
			return nil, apperr.Invalid("project", "no project %s", in.Project)
		}
		project = &p.ID
	}
	hits, err := wiki.Search(ctx, d.Q, c.Agent.WorkspaceID, in.Q, in.Kinds, project, in.Limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(hits))
	for i, h := range hits {
		out[i] = map[string]any{"ref": wiki.LinkText(h.Target), "kind": h.Kind, "type": h.ArtifactType, "title": h.Title, "snippet": h.Snippet}
	}
	return map[string]any{"results": out, "hint": "Open one with read_page(ref)."}, nil
}

func readPage(ctx context.Context, d Deps, c agents.Caller, in PageIn) (any, error) {
	ws := c.Agent.WorkspaceID
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ref, t, err := wiki.LookupRef(ctx, d.Q, ws, in.Ref)
	if err != nil {
		return nil, apperr.Invalid("ref", "nothing called %q; try search_knowledge", in.Ref)
	}
	if ref.Kind == "ticket" {
		return nil, apperr.Invalid("ref", "%s is a ticket: use get_ticket", t.DisplayID)
	}
	if ref.Kind != "artifact" {
		return map[string]any{"kind": ref.Kind, "title": t.Title, "message": "Not a page. get_backlinks shows what links to it."}, nil
	}
	a, err := d.Q.GetArtifact(ctx, store.GetArtifactParams{ID: ref.ID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	if !scope.allowsArtifact(a) {
		return nil, apperr.Invalid("ref", "nothing called %q; try search_knowledge", in.Ref)
	}
	if a.CurrentVersionID == nil {
		return nil, apperr.Invalid("ref", "%q is only a suggestion so far", in.Ref)
	}
	v, err := d.Q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	if v.BodyMd == nil {
		return nil, apperr.Invalid("ref", "%s is a %s file, not text", a.Title, v.ContentType)
	}
	body := *v.BodyMd
	anchor := t.Anchor
	if in.Section != "" {
		anchor = artifacts.Slug(in.Section)
	}
	if anchor != "" {
		sec, ok := artifacts.SectionText(body, anchor)
		if !ok {
			return nil, apperr.Invalid("section", "no section %q; read the page without section to see its outline", anchor)
		}
		body = sec
	}
	runID := readRunID(ctx, d, c, in.Ticket)
	if err := artifacts.RecordRead(ctx, d.Q, a, c.Agent.ID, runID, anchor, nil); err != nil {
		return nil, err
	}
	outline := []map[string]any{}
	for _, s := range artifacts.Outline(*v.BodyMd) {
		outline = append(outline, map[string]any{"level": s.Level, "heading": s.Heading, "anchor": s.Anchor})
	}
	ls, err := d.Q.LinksFromSource(ctx, store.LinksFromSourceParams{SourceKind: "artifact", SourceID: a.ID})
	if err != nil {
		return nil, err
	}
	linksOut := []string{}
	for _, l := range ls {
		if !slices.Contains(linksOut, l.RawText) {
			linksOut = append(linksOut, l.RawText)
		}
	}
	out := map[string]any{"title": a.Title, "type": a.Type, "version": v.Label, "markdown": body, "outline": outline, "links": linksOut}
	if a.Path != nil {
		out["path"] = *a.Path
	}
	if t.DisplayID != "" {
		out["display_id"] = t.DisplayID
	}
	return out, nil
}

func writeNote(ctx context.Context, d Deps, c agents.Caller, in WriteNoteIn) (any, error) {
	scopes, _, err := scopesFor(ctx, d, c, in.Ticket)
	if err != nil {
		return nil, err
	}
	res, err := d.Wiki.AgentWrite(ctx, c.Agent, scopes, wiki.Save{Path: in.Path, Body: in.Body, Mode: in.Mode, Note: in.Note})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"path": *res.Note.Path, "state": res.State, "created": res.Created}
	if res.State == "suggested" {
		out["message"] = "Saved as a suggestion: a person will accept or reject it. Your own memory (" + wiki.MemoryRoot(c.Agent.Handle) + ") is always writable."
	}
	return ok(out), nil
}

func getBacklinks(ctx context.Context, d Deps, c agents.Caller, in BacklinksIn) (any, error) {
	ws := c.Agent.WorkspaceID
	scope, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	ref, t, err := wiki.LookupRef(ctx, d.Q, ws, in.Ref)
	if err != nil {
		return nil, apperr.Invalid("ref", "nothing called %q; try search_knowledge", in.Ref)
	}
	if scope != nil {
		// The target and every source must be in the chat's project (or workspace-wide).
		if ok, err := scope.sourceVisible(ctx, d, ref.Kind, ref.ID); err != nil {
			return nil, err
		} else if !ok {
			return nil, apperr.Invalid("ref", "nothing called %q; try search_knowledge", in.Ref)
		}
	}
	bl, err := wiki.Backlinks(ctx, d.Q, ws, ref)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(bl))
	for _, m := range bl {
		if scope != nil {
			if ok, err := scope.sourceVisible(ctx, d, m.SourceKind, m.SourceID); err != nil {
				return nil, err
			} else if !ok {
				continue
			}
		}
		r := map[string]any{"ref": wiki.LinkText(m.Source), "kind": m.Source.Kind, "title": m.Source.Title, "snippets": m.Snippets}
		if len(m.Anchors) > 0 {
			r["cites_sections"] = m.Anchors
		}
		out = append(out, r)
	}
	return map[string]any{"target": t.Title, "linked_from": out}, nil
}

func timeoutOf(s int) time.Duration { return time.Duration(s) * time.Second }

func parseUUID(field, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return id, apperr.Invalid(field, "must be an id returned by an earlier call")
	}
	return id, nil
}

func ok(extra map[string]any) map[string]any {
	out := map[string]any{"ok": true}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// Tools is the whole Agent API.
type TicketProposalIn struct {
	Ref         string   `json:"ref,omitempty" jsonschema:"Unique stable proposal reference, such as schema or api"`
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"Other proposal refs that must finish first"`
	BlockedBy   []string `json:"blocked_by,omitempty" jsonschema:"Existing ticket keys that must be Done first"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Criteria    []string `json:"criteria"`
	AgentID     string   `json:"agent_id,omitempty" jsonschema:"Optional suggested agent UUID; omit for unassigned"`
}

type ProposeTicketsIn struct {
	Tickets []TicketProposalIn `json:"tickets" jsonschema:"Draft tickets for a person to edit and accept"`
}

var Tools = []Tool{
	define("propose_tickets", "Submit draft tickets for human review and finish this planning run. Creates no tickets.", func(ctx context.Context, d Deps, c agents.Caller, in ProposeTicketsIn) (any, error) {
		items := make([]runs.Proposal, 0, len(in.Tickets))
		for _, item := range in.Tickets {
			p := runs.Proposal{Ref: item.Ref, DependsOn: item.DependsOn, BlockedBy: item.BlockedBy, Title: item.Title, Description: item.Description, Criteria: item.Criteria}
			if item.AgentID != "" {
				id, err := parseUUID("agent_id", item.AgentID)
				if err != nil {
					return nil, err
				}
				p.AgentID = &id
			}
			items = append(items, p)
		}
		p, err := d.Runs.ProposeTickets(ctx, c, items)
		if err != nil {
			return nil, err
		}
		return map[string]any{"planning_id": p.ID, "state": p.State}, nil
	}),
	define("whoami", "Your identity, instructions, scopes, live runs and the tickets assigned to you.", whoami),
	define("next_ticket", "Assigned tickets ready to claim: blockers Done, project active and no live run. Most urgent first.",
		func(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
			q, err := d.Q.ListTickets(ctx, store.ListTicketsParams{WorkspaceID: c.Agent.WorkspaceID, AssigneeAgentID: &c.Agent.ID, Ready: true})
			if err != nil {
				return nil, err
			}
			items := make([]QueueItem, len(q))
			for i, t := range q {
				items[i] = QueueItem{Key: t.Key, Title: t.Title, Status: t.Status, Priority: t.Priority}
			}
			return map[string]any{"queue": items}, nil
		}),
	define("claim_ticket", "Start (or reconnect to) a run on a ticket assigned to you. Returns your scopes for this project.", claim),
	define("get_ticket", "A ticket: description, numbered acceptance criteria, attached artifacts and recent comments.", getTicket),
	define("read_artifact", "Read a PRD, spec, ADR or report attached to a ticket, optionally one section. Reads are recorded as citations.", readArtifact),
	define("post_plan", "Post (or replace) your execution plan. Do this before editing anything.",
		func(ctx context.Context, d Deps, c agents.Caller, in PlanIn) (any, error) {
			steps, err := d.Runs.PostPlan(ctx, c, in.Ticket, in.Steps)
			if err != nil {
				return nil, err
			}
			return ok(map[string]any{"steps": len(steps)}), nil
		}),
	define("update_step", "Mark a plan step current, done, needs_human or failed. One step is current at a time.",
		func(ctx context.Context, d Deps, c agents.Caller, in StepIn) (any, error) {
			return ok(nil), d.Runs.UpdateStep(ctx, c, in.Ticket, in.Step, in.State)
		}),
	define("log_event", "Record a timeline event, e.g. kind test_run with {passed, failed, duration_s, command}.",
		func(ctx context.Context, d Deps, c agents.Caller, in LogEventIn) (any, error) {
			return ok(nil), d.Runs.LogEvent(ctx, c, in.Ticket, in.Kind, in.Payload)
		}),
	define("report_usage", "Unmanaged sessions only: add tool calls, tokens and cost for the run card.",
		func(ctx context.Context, d Deps, c agents.Caller, in UsageIn) (any, error) {
			return ok(nil), d.Runs.ReportUsage(ctx, c, in.Ticket, in.ToolCalls, in.InputTokens, in.OutputTokens, in.CostUSD)
		}),
	define("set_branch", "Record the branch you're working on.",
		func(ctx context.Context, d Deps, c agents.Caller, in BranchIn) (any, error) {
			return ok(nil), d.Runs.SetBranch(ctx, c, in.Ticket, in.Branch)
		}),
	define("set_pr", "Record the ticket's pull request once you've opened it.",
		func(ctx context.Context, d Deps, c agents.Caller, in PRIn) (any, error) {
			return ok(nil), d.Runs.SetPR(ctx, c, in.Ticket, in.URL, in.Draft)
		}),
	define("check_criterion", "Tick an acceptance criterion once you've verified it.",
		func(ctx context.Context, d Deps, c agents.Caller, in CriterionIn) (any, error) {
			done := in.Done == nil || *in.Done
			return ok(nil), d.Runs.CheckCriterion(ctx, c, in.Ticket, in.N, done)
		}),
	define("create_artifact", "Attach a REPORT you wrote (dry-run results, analysis). Agents only create reports.",
		func(ctx context.Context, d Deps, c agents.Caller, in ReportIn) (any, error) {
			a, err := d.Runs.CreateReport(ctx, c, in.Ticket, in.Title, in.Body)
			if err != nil {
				return nil, err
			}
			return ok(map[string]any{"artifact_id": a.ID}), nil
		}),
	define("comment", "Comment on the ticket as yourself. People reply in comments; mention them by name.",
		func(ctx context.Context, d Deps, c agents.Caller, in CommentIn) (any, error) {
			_, err := d.Runs.Comment(ctx, c, in.Ticket, in.Body)
			return ok(nil), err
		}),
	define("create_ticket", "Open a follow-up ticket in the same project (unassigned, in the backlog).",
		func(ctx context.Context, d Deps, c agents.Caller, in NewTicketIn) (any, error) {
			_, t, err := d.Runs.Active(ctx, d.Q, c, in.Ticket)
			if err != nil {
				return nil, err
			}
			p, err := d.Q.GetProject(ctx, store.GetProjectParams{ID: t.ProjectID, WorkspaceID: t.WorkspaceID})
			if err != nil {
				return nil, err
			}
			nt, err := d.Tracker.Create(ctx, t.WorkspaceID, tracker.AgentActor(c.Agent.ID), tracker.NewTicket{
				ProjectKey: p.Key, Title: in.Title, DescriptionMd: in.Description + "\n\nOpened by @" + c.Agent.Handle + " from " + t.Key + ".",
				Priority: in.Priority, EpicID: t.EpicID,
			})
			if err != nil {
				return nil, err
			}
			return ok(map[string]any{"ticket": nt.Key}), nil
		}),
	define("request_approval", "Ask a person before using a scope that requires approval. If it returns pending, your turn should end: call wait_for_approval (or stop, if you're managed).",
		func(ctx context.Context, d Deps, c agents.Caller, in ApprovalIn) (any, error) {
			out, err := d.Runs.RequestApproval(ctx, c, in.Ticket, in.Scope, in.Action, in.Detail, in.Risk)
			if err != nil {
				return nil, err
			}
			if out.NotRequired {
				return map[string]any{"state": "not_required", "message": in.Scope + " is allowed for you here; go ahead."}, nil
			}
			msg := "A person has been asked. Call wait_for_approval with this approval_id."
			if _, brokered := brokerFor(in.Scope); brokered {
				msg += " Once approved, call run_gated: LazyAgents runs the approved action itself."
			}
			return map[string]any{"state": "pending", "approval_id": out.Approval.ID, "message": msg}, nil
		}),
	define("wait_for_approval", "Wait (up to 50s per call) for a person to decide. Repeat while it returns pending.",
		func(ctx context.Context, d Deps, c agents.Caller, in WaitIn) (any, error) {
			id, err := parseUUID("approval_id", in.ApprovalID)
			if err != nil {
				return nil, err
			}
			ap, err := d.Runs.WaitApproval(ctx, c, id, timeoutOf(in.TimeoutS))
			if err != nil {
				return nil, err
			}
			return map[string]any{"state": ap.State, "note": ap.DecisionNote}, nil
		}),
	define("run_gated", "Execute an approved brokered action (e.g. db:migrate). You never see the credential; the result is returned.",
		func(ctx context.Context, d Deps, c agents.Caller, in GatedIn) (any, error) {
			id, err := parseUUID("approval_id", in.ApprovalID)
			if err != nil {
				return nil, err
			}
			res, err := d.Runs.RunGated(ctx, c, in.Ticket, id)
			if err != nil {
				return nil, err
			}
			return res, nil
		}),
	define("await_human", "End your turn until a person replies. A comment mentioning you resumes the run.",
		func(ctx context.Context, d Deps, c agents.Caller, in AwaitHumanIn) (any, error) {
			if err := d.Runs.AwaitHuman(ctx, c, in.Ticket, in.Reason); err != nil {
				return nil, err
			}
			return ok(map[string]any{"message": "Waiting. Unmanaged sessions: call await_signal to wait for the reply."}), nil
		}),
	define("await_signal", "Unmanaged sessions: wait (up to 50s) for a decision, a comment, or a pause/resume/cancel. Pass the returned cursor next time.",
		func(ctx context.Context, d Deps, c agents.Caller, in SignalIn) (any, error) {
			since := time.Time{}
			if in.Since != "" {
				t, err := time.Parse(time.RFC3339Nano, in.Since)
				if err != nil {
					return nil, apperr.Invalid("since", "use the cursor from the previous call")
				}
				since = t
			}
			sig, err := d.Runs.AwaitSignal(ctx, c, in.Ticket, since, timeoutOf(in.TimeoutS))
			if err != nil {
				return nil, err
			}
			comments := make([]map[string]any, len(sig.Comments))
			for i, cm := range sig.Comments {
				comments[i] = map[string]any{"author": cm.Author, "body": cm.BodyMd, "at": cm.CreatedAt}
			}
			decisions := make([]map[string]any, len(sig.Decisions))
			for i, ap := range sig.Decisions {
				decisions[i] = map[string]any{"approval_id": ap.ID, "state": ap.State, "note": ap.DecisionNote}
			}
			return map[string]any{"run_state": sig.State, "wait_reason": sig.WaitReason, "comments": comments, "decisions": decisions,
				"cursor": sig.Cursor.Format(time.RFC3339Nano)}, nil
		}),
	define("search_knowledge", "Search the wiki, agent memory, documents and tickets. Well-linked pages rank higher.", searchKnowledge),
	define("read_page", "Read a wiki page or any document by title, path, ADR id or [[Page#Section]]. Lists the page's outgoing [[links]]; reads count as citations.", readPage),
	define("write_note", "Write a wiki note. Your memory (agents/<your handle>/…) is always writable; other pages follow your wiki:write scope (allowed edits directly, requires_approval saves a suggestion). Link what it's about with [[…]].", writeNote),
	define("get_backlinks", "What links to a page, document, ticket, epic or agent, with the sentence around each link.", getBacklinks),
	define("list_subagents", "Sub agents you can delegate part of a ticket to with delegate: handle, role and specialty. Main agents only.",
		func(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
			subs, err := d.Runs.ListSubagents(ctx, c)
			if err != nil {
				return nil, err
			}
			items := make([]map[string]any, len(subs))
			for i, a := range subs {
				items[i] = map[string]any{"handle": a.Handle, "name": a.Name, "role": a.Role, "specialty": a.Specialty}
			}
			return map[string]any{"subagents": items}, nil
		}),
	define("delegate", "Hand a self-contained part of your ticket (implementation, docs, tests) to a sub agent. It works in your checkout on your branch while you wait: end your turn after calling this and you'll be resumed with its summary. One delegation at a time. Review its diff before relying on it.",
		func(ctx context.Context, d Deps, c agents.Caller, in DelegateIn) (any, error) {
			dg, err := d.Runs.Delegate(ctx, c, in.Ticket, in.Agent, in.Task)
			if err != nil {
				return nil, err
			}
			return map[string]any{"state": "delegated", "agent": dg.Agent.Handle, "run_id": dg.Run.ID,
				"message": "@" + dg.Agent.Handle + " is working on it. End your turn now: you'll be resumed with its result."}, nil
		}),
	define("complete_run", "Finish the run: the ticket moves to In Review (or done/todo). If you were delegated a task, this reports your summary back to the main agent instead and leaves the ticket alone.",
		func(ctx context.Context, d Deps, c agents.Caller, in CompleteIn) (any, error) {
			return ok(nil), d.Runs.Complete(ctx, c, in.Ticket, in.Outcome, in.Status)
		}),
}

// ByName indexes Tools.
var ByName = func() map[string]Tool {
	m := map[string]Tool{}
	for _, t := range Tools {
		m[t.Name] = t
	}
	return m
}()

// PublicError is what the agent sees: domain errors verbatim, anything else
// generic (internal details stay in the server log).
func PublicError(err error) (msg string, internal bool) {
	var ve *apperr.ValidationError
	var ce *apperr.ConflictError
	switch {
	case errors.As(err, &ve), errors.As(err, &ce):
		return err.Error(), false
	case errors.Is(err, apperr.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		return "not found", false
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "the request was cancelled", false
	}
	return "internal error", true
}
