// Package seed creates the first workspace: either the Northwind Labs demo
// from the design mockups (SEED=true) or an empty workspace with one local
// user. With no login (docs/dev/plan.md §5.4), the first user created is who every
// request acts as: Maya Chen in the demo.
package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/auth"
	"github.com/liamphmurphy/lazyagents/server/internal/personas"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

type Options struct {
	Demo bool
	// Artifacts creates the demo documents. Without a blob store the design
	// export is skipped; without a service, artifacts are skipped entirely.
	Artifacts *artifacts.Service
	// The local user for a non-demo workspace.
	UserName  string
	UserEmail string
}

// Result says what was created, for the startup log.
type Result struct {
	Created bool
}

// Run seeds only when the database has no workspace yet.
func Run(ctx context.Context, pool *pgxpool.Pool, o Options) (Result, error) {
	n, err := store.New(pool).CountWorkspaces(ctx)
	if err != nil || n > 0 {
		return Result{}, err
	}
	if o.Demo {
		return demo(ctx, pool, o.Artifacts)
	}
	if o.UserName == "" {
		o.UserName = "Local user"
	}
	if o.UserEmail == "" {
		o.UserEmail = "me@localhost"
	}
	return Result{Created: true}, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		ws, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "My workspace", Slug: "default"})
		if err != nil {
			return err
		}
		_, err = createUser(ctx, q, ws.ID, o.UserName, o.UserEmail, "", nil)
		return err
	})
}

// createUser stores a random, unknown password: nobody signs in while login
// is removed, and a real hash keeps the column honest for when it returns.
func createUser(ctx context.Context, q *store.Queries, ws uuid.UUID, name, email, role string, color *string) (store.User, error) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return store.User{}, err
	}
	hash, err := auth.HashPassword(hex.EncodeToString(secret))
	if err != nil {
		return store.User{}, err
	}
	initials := ""
	for _, p := range strings.Fields(name) {
		initials += strings.ToUpper(p[:1])
	}
	if len(initials) > 2 {
		initials = initials[:1] + initials[len(initials)-1:]
	}
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		WorkspaceID: ws, Name: name, Email: email, Initials: initials, RoleTitle: role, AvatarColor: color, PasswordHash: hash,
	})
	if err != nil {
		return u, err
	}
	// Every person starts with the built-in "Assistant" as their default persona.
	return u, personas.SeedDefault(ctx, q, ws, u.ID)
}

func ptr[T any](v T) *T { return &v }

func date(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func at(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return &t
}

type demoTicket struct {
	num      int32
	title    string
	status   string
	priority string
	labels   []string
	epic     string
	agent    string
	due      string
	desc     string
	criteria []string
}

// Ticket data mirrors docs/design-system/screens/tickets.html.
var payTickets = []demoTicket{
	{num: 158, title: "Archive settlement CSV exports to object storage", status: "backlog", priority: "low", labels: []string{"data"}, epic: "Maintenance"},
	{num: 162, title: "Evaluate multi-currency settlement accounts", status: "backlog", priority: "none", labels: []string{"research"}, due: "2026-11-21"},
	{num: 171, title: "Consolidate fee calculation modules", status: "backlog", priority: "low", labels: []string{"refactor"}, epic: "Maintenance"},
	{num: 176, title: "Remove deprecated v1 charge serializer", status: "in_review", priority: "low", labels: []string{"cleanup"}, epic: "Maintenance", agent: "echo"},
	{num: 187, title: "Backfill currency precision for legacy JPY rows", status: "in_progress", priority: "medium", labels: []string{"data"}, epic: "Ledger v2", agent: "pike"},
	{num: 198, title: "Idempotency keys for refund endpoint", status: "in_review", priority: "high", labels: []string{"api"}, epic: "Refunds API", agent: "quill"},
	{num: 203, title: "Add retry with jitter to webhook dispatcher", status: "in_progress", priority: "high", labels: []string{"reliability"}, epic: "Webhooks 2.0", agent: "quill"},
	{num: 209, title: "Rate-limit refunds per merchant", status: "in_review", priority: "medium", labels: []string{"api", "security"}, epic: "Refunds API", agent: "sentry"},
	{
		num: 214, title: "Migrate ledger_entries to a partitioned table", status: "in_progress", priority: "urgent",
		labels: []string{"migration", "db"}, epic: "Ledger v2", agent: "atlas", due: "2026-11-04",
		desc: "`ledger_entries` has grown to 18M rows and month-end reconciliation queries now exceed 9s at p95. " +
			"Partition the table by `posted_at` month so reconciliation can prune to a single partition, per the Ledger v2 PRD §4.2.",
		criteria: []string{
			"Monthly range partitions created for 2019-01 through 2027-12",
			"Writes dual-routed during backfill with zero dropped rows",
			"Reconciliation query p95 under 800ms on staging snapshot",
			"Migration applied to staging with no lock longer than 200ms",
			"Legacy index retained for 72h as a rollback window",
		},
	},
	{num: 219, title: "Surface refund status in the merchant dashboard", status: "todo", priority: "low", labels: []string{"ui"}, epic: "Refunds API", due: "2026-11-12"},
	{num: 221, title: "Emit ledger events to the analytics stream", status: "in_progress", priority: "medium", labels: []string{"events"}, epic: "Ledger v2", agent: "relay"},
	{num: 224, title: "Write runbook for ledger partition rollback", status: "todo", priority: "medium", labels: []string{"docs"}, epic: "Ledger v2", agent: "atlas"},
	{num: 226, title: "Reconcile payouts nightly against bank statements", status: "todo", priority: "high", labels: []string{"finance"}, epic: "Ledger v2", due: "2026-11-10"},
	{num: 230, title: "Add contract tests for webhook payloads", status: "todo", priority: "medium", labels: []string{"testing"}, epic: "Webhooks 2.0", agent: "quill"},
}

func demo(ctx context.Context, pool *pgxpool.Pool, arts *artifacts.Service) (Result, error) {
	res := Result{Created: true}

	var ws uuid.UUID
	ids := struct {
		users    map[string]store.User
		agents   map[string]store.Agent
		projects map[string]store.Project
		epics    map[string]store.Epic
		labels   map[string]uuid.UUID
	}{map[string]store.User{}, map[string]store.Agent{}, map[string]store.Project{}, map[string]store.Epic{}, map[string]uuid.UUID{}}

	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		w, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Name: "Northwind Labs", Slug: "northwind"})
		if err != nil {
			return err
		}
		ws = w.ID

		for _, u := range []struct {
			key, name, email, role string
			color                  *string
		}{
			{"maya", "Maya Chen", "maya@northwind.dev", "Eng lead", nil},
			{"dev", "Dev Patel", "dev@northwind.dev", "Staff engineer", ptr("#3A2F4A")},
			{"lena", "Lena Sato", "lena@northwind.dev", "Product manager", ptr("#2F3A44")},
		} {
			// Maya is created first, so she's the local user.
			if ids.users[u.key], err = createUser(ctx, q, ws, u.name, u.email, u.role, u.color); err != nil {
				return err
			}
		}

		for i, a := range []struct{ name, role, specialty, model, status string }{
			{"Atlas", "Backend engineer", "Backend & migrations", "opus-5.5", "waiting"},
			{"Quill", "API engineer", "API endpoints & tests", "sonnet-5.5", "running"},
			{"Sentry", "Reviewer", "Spec review & docs QA", "opus-5.5", "running"},
			{"Juno", "Mobile engineer", "Mobile · React Native", "sonnet-5.5", "running"},
			{"Nova", "Mobile engineer", "Mobile · auth & sessions", "sonnet-5.5", "idle"},
			{"Relay", "Integrations engineer", "Integrations", "sonnet-5.5", "running"},
			{"Pike", "Data engineer", "Data & backfills", "sonnet-5.5", "running"},
			{"Echo", "Maintenance engineer", "Refactors & cleanup", "haiku-5.5", "idle"},
		} {
			handle := strings.ToLower(a.name)
			if ids.agents[handle], err = q.CreateAgent(ctx, store.CreateAgentParams{
				WorkspaceID: ws, Name: a.name, Handle: handle, ColorIndex: int32(i),
				Role: a.role, Specialty: a.specialty, Model: a.model, Status: a.status,
			}); err != nil {
				return err
			}
		}

		for _, p := range []struct{ key, name, color, repo string }{
			{"PAY", "Payments Platform", "#8B8FFF", "git@github.com:northwind/payments.git"},
			{"MOB", "Mobile App v3", "#F0BF4C", "git@github.com:northwind/mobile.git"},
			{"DOC", "Developer Docs", "#3FC1B0", "git@github.com:northwind/docs.git"},
			{"BIL", "Billing Migration", "#F27A8A", "git@github.com:northwind/billing.git"},
		} {
			if ids.projects[p.key], err = q.CreateProject(ctx, store.CreateProjectParams{
				WorkspaceID: ws, Key: p.key, Name: p.name, Color: p.color, RepoUrl: &p.repo,
			}); err != nil {
				return err
			}
		}

		maya := ids.users["maya"].ID
		for _, e := range []struct{ project, name, objective, start, target, health string }{
			{"PAY", "Ledger v2", "Partitioned, auditable ledger that reconciles month-end in under a second.", "2026-10-01", "2026-12-15", "at_risk"},
			{"PAY", "Webhooks 2.0", "At-least-once webhook delivery with retries and signed payloads.", "2026-10-15", "2026-12-01", "on_track"},
			{"PAY", "Refunds API", "Self-serve partial refunds with idempotency and rate limits.", "2026-09-15", "2026-11-20", "on_track"},
			{"PAY", "Maintenance", "Keep the payments codebase healthy.", "2026-10-01", "2026-12-31", "on_track"},
			{"MOB", "Offline mode", "Queue writes offline and sync safely on reconnect.", "2026-10-01", "2026-12-20", "on_track"},
			{"DOC", "API reference v2", "Generated, versioned API reference with runnable examples.", "2026-10-10", "2026-11-30", "on_track"},
			{"BIL", "Stripe → Ledger cutover", "Move invoicing off the legacy billing tables.", "2026-11-01", "2027-01-31", "off_track"},
		} {
			if ids.epics[e.name], err = q.CreateEpic(ctx, store.CreateEpicParams{
				WorkspaceID: ws, ProjectID: ids.projects[e.project].ID, Name: e.name, Objective: e.objective,
				OwnerUserID: &maya, StartDate: date(e.start), TargetDate: date(e.target), Health: e.health,
			}); err != nil {
				return err
			}
		}

		if err := seedDefinitions(ctx, q, ws, ids.users["maya"].ID, ids.agents, ids.projects); err != nil {
			return err
		}

		for _, l := range []struct{ name, color string }{
			{"migration", "#8B8FFF"}, {"events", "#3FC1B0"}, {"db", "#5EA8FF"}, {"reliability", "#F0BF4C"},
			{"research", "#8B8B94"}, {"refactor", "#E8925A"}, {"cleanup", "#8B8B94"}, {"data", "#5EA8FF"},
			{"testing", "#F0BF4C"}, {"api", "#B98BF5"}, {"finance", "#6FCF7F"}, {"docs", "#3FC1B0"},
			{"ui", "#F27A8A"}, {"compliance", "#EF6461"}, {"security", "#EF6461"},
		} {
			lbl, err := q.CreateLabel(ctx, store.CreateLabelParams{WorkspaceID: ws, Name: l.name, Color: l.color})
			if err != nil {
				return err
			}
			ids.labels[l.name] = lbl.ID
		}
		return nil
	})
	if err != nil {
		return res, err
	}

	// Tickets go through the tracker service so their activity is real.
	svc := &tracker.Service{Pool: pool}
	q := store.New(pool)
	maya := tracker.UserActor(ids.users["maya"].ID)
	pay := ids.projects["PAY"]
	created := at("2026-10-30 10:15")
	for _, t := range payTickets {
		if err := q.SetNextTicketSeq(ctx, store.SetNextTicketSeqParams{ID: pay.ID, NextTicketSeq: t.num}); err != nil {
			return res, err
		}
		in := tracker.NewTicket{
			ProjectKey: "PAY", Title: t.title, DescriptionMd: t.desc, Status: t.status, Priority: t.priority,
			Criteria: t.criteria, CreatedAt: created,
		}
		if t.epic != "" {
			e := ids.epics[t.epic]
			in.EpicID = &e.ID
		}
		if t.agent != "" {
			a := ids.agents[t.agent]
			in.AssigneeAgentID = &a.ID
		}
		for _, l := range t.labels {
			in.LabelIDs = append(in.LabelIDs, ids.labels[l])
		}
		if t.due != "" {
			in.DueDate = date(t.due)
		}
		if t.num == 214 {
			in.ReviewerUserID = ptr(ids.users["maya"].ID)
			in.Estimate = ptr("L · ~6h agent time")
		}
		if _, err := svc.Create(ctx, ws, maya, in); err != nil {
			return res, fmt.Errorf("seed PAY-%d: %w", t.num, err)
		}
	}
	if err := q.SetNextTicketSeq(ctx, store.SetNextTicketSeqParams{ID: pay.ID, NextTicketSeq: 231}); err != nil {
		return res, err
	}

	// PAY-214's story so far (agent runs, plans and approvals join in Phase 4).
	t214, err := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: "PAY-214"})
	if err != nil {
		return res, err
	}
	if err := q.SetTicketCode(ctx, store.SetTicketCodeParams{
		ID: t214.ID, Branch: ptr("atlas/pay-214-partition-ledger"), PrNumber: ptr[int32](491),
		PrUrl: ptr("https://github.com/northwind/payments/pull/491"), PrState: ptr("draft"),
		PrAdditions: ptr[int32](412), PrDeletions: ptr[int32](37),
	}); err != nil {
		return res, err
	}
	crit, err := q.ListCriteria(ctx, t214.ID)
	if err != nil {
		return res, err
	}
	// Ticked directly: the run that ticked them is Phase 4 history we don't fake.
	for _, c := range crit[:2] {
		if _, err := q.UpdateCriterion(ctx, store.UpdateCriterionParams{ID: c.ID, Text: c.Text, Done: true}); err != nil {
			return res, err
		}
	}
	atlas := ids.agents["atlas"].ID
	dev := tracker.UserActor(ids.users["dev"].ID)
	root, err := svc.Comment(ctx, ws, dev, "PAY-214", tracker.NewComment{
		BodyMd: "Keep the old index around until the backfill verification finishes — ops wants a rollback window before we drop anything.",
		At:     at("2026-10-31 11:02"),
	})
	if err != nil {
		return res, err
	}
	if id, ok := commentID(root); ok {
		if _, err := svc.Comment(ctx, ws, tracker.Actor{Type: "agent", ID: &atlas}, "PAY-214", tracker.NewComment{
			BodyMd:          "Understood. Step 8 now retains `ledger_entries_legacy_idx` for 72h. I opened PAY-224 for the rollback runbook.",
			ParentCommentID: &id,
			At:              at("2026-10-31 11:03"),
		}); err != nil {
			return res, err
		}
	}
	if arts != nil {
		if err := seedArtifacts(ctx, arts, q, svc, ws, ids.users, ids.agents, ids.epics); err != nil {
			return res, err
		}
		if err := seedWiki(ctx, pool, arts, svc, ws, ids.users, ids.agents); err != nil {
			return res, err
		}
	}
	return res, nil
}
