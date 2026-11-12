package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// micros is runner/chat.go's conversion: float64 multiply, truncate. A
// function, so Go doesn't fold it as an exact constant expression.
func micros(usd float64) int64 { return int64(usd * 1e6) }

// The Costs report sums run counters server-side: every breakdown, the days and
// the top tickets add up to the total, chat replies are split by model with the
// runner's per-turn truncation, and filters scope it like the cost chips.
func TestGetCosts(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: &runs.Service{Pool: pool, Tracker: tr}}).Handler()
	get := func(query string, intended int) api.CostReport {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/costs"+query, nil))
		if r.Code != intended {
			t.Fatalf("%s: %d %s", query, r.Code, r.Body.String())
		}
		var out api.CostReport
		if intended == 200 {
			if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}

	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ws := user.WorkspaceID
	agent := func(handle string) store.Agent {
		a, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: handle})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	atlas, quill := agent("atlas"), agent("quill")
	ticket, err := tracker.Get(ctx, q, ws, "PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET estimate_points = 8 WHERE id = $1`, ticket.ID); err != nil {
		t.Fatal(err)
	}
	day := func(d, h int) time.Time { return time.Date(2020, 3, d, h, 0, 0, 0, time.UTC) }
	// Runs live in March 2020, far from the demo seed's runs.
	newRun := func(ws uuid.UUID, a store.Agent, purpose, model string, ticketID, parent *uuid.UUID, started time.Time, cost, in, out int64, source *string) store.Run {
		t.Helper()
		r, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: ticketID, AgentID: a.ID, Mode: "managed", State: "completed", ResolvedSpec: json.RawMessage(`{}`), Harness: "codex", Model: model, ParentRunID: parent, Purpose: purpose})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE runs SET started_at = $2, cost_usd_micros = $3, input_tokens = $4, output_tokens = $5, cost_source = $6 WHERE id = $1`, r.ID, started, cost, in, out, source); err != nil {
			t.Fatal(err)
		}
		return r
	}
	harness, computed := "harness", "computed"
	parent := newRun(ws, atlas, "ticket", "m1", &ticket.ID, nil, day(1, 10), 2_500_000, 100, 50, &computed)
	child := newRun(ws, quill, "ticket", "m2", &ticket.ID, &parent.ID, day(2, 9), 1_000_000, 40, 20, &harness)

	var other struct {
		ID  uuid.UUID
		Key string
	}
	if err := pool.QueryRow(ctx, `SELECT id, key FROM projects WHERE workspace_id = $1 AND key <> 'PAY' ORDER BY key LIMIT 1`, ws).Scan(&other.ID, &other.Key); err != nil {
		t.Fatal(err)
	}
	// Tokens with no cost source: unpriced.
	plan := newRun(ws, atlas, "planning", "m1", nil, nil, day(2, 12), 750_000, 10, 0, nil)
	if _, err := pool.Exec(ctx, `INSERT INTO planning_sessions (workspace_id, project_id, goal, run_id, created_by) VALUES ($1, $2, 'Plan', $3, $4)`, ws, other.ID, plan.ID, user.ID); err != nil {
		t.Fatal(err)
	}

	// A chat run that switched model mid-run. Replies carry fractional micros;
	// the counter holds their truncated sum plus 500 from a turn with no reply.
	a1, a2 := 1.00000175, 2.00000175
	chatCost := micros(a1) + micros(a2) + 500
	chatRun := newRun(ws, atlas, "chat", "b", nil, nil, day(1, 8), chatCost, 25, 15, &harness)
	// A legacy chat run whose replies claim more than its counter: all to its model.
	badChat := newRun(ws, atlas, "chat", "z", nil, nil, day(3, 8), 400_000, 5, 5, &harness)
	var chatID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO chat_sessions (workspace_id, project_id, title, created_by, agent_id, harness, model, current_run_id) VALUES ($1, $2, 'Spend', $3, $4, 'codex', 'b', $5) RETURNING id`, ws, ticket.ProjectID, user.ID, atlas.ID, badChat.ID).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	msg := func(seq int, role, model string, run uuid.UUID, at time.Time, usage string) {
		t.Helper()
		var u any
		if usage != "" {
			u = usage
		}
		if _, err := pool.Exec(ctx, `INSERT INTO chat_messages (session_id, seq, role, body_md, harness, model, run_id, usage, created_at) VALUES ($1, $2, $3, 'x', 'codex', $4, $5, $6::jsonb, $7)`, chatID, seq, role, model, run, u, at); err != nil {
			t.Fatal(err)
		}
	}
	msg(1, "user", "a", chatRun.ID, day(2, 8), "")
	msg(2, "assistant", "a", chatRun.ID, day(2, 8), `{"cost_usd":1.00000175,"input_tokens":10,"output_tokens":5}`)
	msg(3, "assistant", "b", chatRun.ID, day(3, 8), `{"cost_usd":2.00000175,"input_tokens":10,"output_tokens":5}`)
	msg(4, "assistant", "y", badChat.ID, day(3, 9), `{"cost_usd":0.5,"input_tokens":1,"output_tokens":1}`)

	// Outside the range, and in another workspace: excluded.
	newRun(ws, atlas, "ticket", "m1", &ticket.ID, nil, day(5, 10), 9_000_000, 1, 1, &harness)
	var foreignWS uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO workspaces (name,slug) VALUES ('Other','other-costs') RETURNING id").Scan(&foreignWS); err != nil {
		t.Fatal(err)
	}
	newRun(foreignWS, atlas, "planning", "m1", nil, nil, day(2, 10), 7_000_000, 1, 1, &harness)

	rep := get("?from=2020-03-01&to=2020-03-03", 200)
	want := 2_500_000 + 1_000_000 + 750_000 + chatCost + 400_000
	if rep.Total.CostMicros != want || rep.Total.Runs != 5 || rep.UnpricedRuns != 1 {
		t.Fatalf("total: %+v unpriced %d, want %d over 5 runs", rep.Total, rep.UnpricedRuns, want)
	}
	if rep.Total.Tokens != 150+60+10+40+10 {
		t.Fatalf("tokens: %d", rep.Total.Tokens)
	}
	sum := func(name string, s []api.CostSlice) map[string]api.CostSlice {
		t.Helper()
		var c int64
		m := map[string]api.CostSlice{}
		for _, x := range s {
			c += x.CostMicros
			m[x.Key] = x
		}
		if c != want {
			t.Fatalf("%s adds to %d, want %d: %+v", name, c, want, s)
		}
		return m
	}
	agents := sum("agents", rep.Agents)
	if agents["quill"].CostMicros != 1_000_000 || agents["atlas"].Runs != 4 {
		t.Fatalf("agents: %+v", agents)
	}
	projects := sum("projects", rep.Projects)
	if projects[other.Key].CostMicros != 750_000 || projects["PAY"].CostMicros != want-750_000 {
		t.Fatalf("projects: %+v", projects)
	}
	purposes := sum("purposes", rep.Purposes)
	if purposes["ticket"].CostMicros != 3_500_000 || purposes["ticket"].Runs != 2 || purposes["chat"].CostMicros != chatCost+400_000 || purposes["planning"].UnpricedRuns != 1 {
		t.Fatalf("purposes: %+v", purposes)
	}
	models := sum("models", rep.Models)
	if models["codex/a"].CostMicros != micros(a1) || models["codex/b"].CostMicros != micros(a2)+500 || models["codex/z"].CostMicros != 400_000 {
		t.Fatalf("chat model split: %+v", models)
	}
	if _, ok := models["codex/y"]; ok {
		t.Fatal("contradictory replies must not be attributed")
	}
	if models["codex/a"].Tokens != 15 || models["codex/b"].Tokens != 15+10 || models["codex/b"].Label != "b" || deref(models["codex/b"].Detail) != "codex" {
		t.Fatalf("chat model tokens: %+v", models)
	}

	if len(rep.Days) != 3 || rep.Days[0].Date.Format("2006-01-02") != "2020-03-01" {
		t.Fatalf("days: %+v", rep.Days)
	}
	var daySum int64
	for _, d := range rep.Days {
		daySum += d.CostMicros
		var parts int64
		for _, p := range d.Agents {
			parts += p.CostMicros
		}
		if parts != d.CostMicros {
			t.Fatalf("day %s agents add to %d, want %d", d.Date, parts, d.CostMicros)
		}
	}
	if daySum != want {
		t.Fatalf("days add to %d, want %d", daySum, want)
	}
	// Day 1: the parent run and the chat's no-reply remainder (on its start day).
	if rep.Days[0].CostMicros != 2_500_000+500 || rep.Days[0].Runs != 2 || rep.Days[2].CostMicros != micros(a2)+400_000 {
		t.Fatalf("day buckets: %+v", rep.Days)
	}

	if len(rep.TopTickets) != 1 {
		t.Fatalf("top tickets: %+v", rep.TopTickets)
	}
	top := rep.TopTickets[0]
	if top.Key != "PAY-214" || top.CostMicros != 3_500_000 || top.Points == nil || *top.Points != 8 || len(top.Runs) != 2 || top.Runs[0].Id != child.ID || top.Runs[0].ParentRunId == nil || *top.Runs[0].ParentRunId != parent.ID || top.Runs[0].Agent != "quill" {
		t.Fatalf("top ticket: %+v", top)
	}
	if rep.Estimated.Tickets != 1 || rep.Estimated.Points != 8 || rep.Estimated.CostMicros != 3_500_000 {
		t.Fatalf("estimated: %+v", rep.Estimated)
	}

	// Filters compose with the range; an unknown project matches nothing.
	if got := get("?from=2020-03-01&to=2020-03-03&project="+other.Key, 200); got.Total.CostMicros != 750_000 || len(got.TopTickets) != 0 {
		t.Fatalf("project filter: %+v", got.Total)
	}
	if got := get("?from=2020-03-01&to=2020-03-03&project=NOPE", 200); got.Total.CostMicros != 0 || len(got.Days) != 3 {
		t.Fatalf("unknown project: %+v", got)
	}

	// Chips: all-time spend matches the run counters exactly.
	var ticketCounter int64
	if err := pool.QueryRow(ctx, `SELECT coalesce(sum(cost_usd_micros), 0)::bigint FROM runs WHERE ticket_id = $1`, ticket.ID).Scan(&ticketCounter); err != nil {
		t.Fatal(err)
	}
	if got := get("?ticket=PAY-214", 200); got.Total.CostMicros != ticketCounter || len(got.Days) != 0 {
		t.Fatalf("ticket chip: %d, counters %d", got.Total.CostMicros, ticketCounter)
	}
	if got := get("?chat="+chatID.String(), 200); got.Total.CostMicros != chatCost+400_000 || got.Total.Runs != 2 {
		t.Fatalf("chat chip: %+v", got.Total)
	}

	// Epic: the runs of every ticket in it, sub agents included; an unknown epic spends nothing.
	if ticket.EpicID == nil {
		t.Fatal("PAY-214 has no epic in the demo seed")
	}
	var epicCounter int64
	if err := pool.QueryRow(ctx, `SELECT coalesce(sum(r.cost_usd_micros), 0)::bigint FROM runs r JOIN tickets t ON t.id = r.ticket_id WHERE t.epic_id = $1`, *ticket.EpicID).Scan(&epicCounter); err != nil {
		t.Fatal(err)
	}
	if got := get("?epic="+ticket.EpicID.String(), 200); got.Total.CostMicros != epicCounter || epicCounter < ticketCounter {
		t.Fatalf("epic chip: %d, counters %d (ticket %d)", got.Total.CostMicros, epicCounter, ticketCounter)
	}
	if got := get("?epic="+uuid.NewString(), 200); got.Total.CostMicros != 0 {
		t.Fatalf("unknown epic: %+v", got.Total)
	}

	get("?ticket=NOPE-1", 404)
	get("?chat="+uuid.NewString(), 404)
	get("?from=2020-03-01", 422)
	get("?from=2020-03-03&to=2020-03-01", 422)
	get("?from=2020-01-01&to=2021-06-01", 422)
}
