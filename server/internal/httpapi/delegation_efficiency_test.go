package httpapi

import (
	"context"
	"math"
	"net/http"
	"testing"
)

func TestDelegationEfficiencyChecksCostAndTimings(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	var ws, ticket, agentA, agentB string
	if err := e.pool.QueryRow(ctx, `SELECT workspace_id, id FROM tickets ORDER BY created_at LIMIT 1`).Scan(&ws, &ticket); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `SELECT (array_agg(id ORDER BY handle))[1], (array_agg(id ORDER BY handle))[2] FROM agents WHERE workspace_id=$1`, ws).Scan(&agentA, &agentB); err != nil {
		t.Fatal(err)
	}
	insert := func(agent, parent, checks string, cost int, setup, harness, chk *int) string {
		t.Helper()
		var id string
		err := e.pool.QueryRow(ctx, `INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, harness, model, purpose,
			parent_run_id, checks, cost_usd_micros, setup_ms, harness_ms, checks_ms, started_at, ended_at)
			VALUES ($1,$2,$3,'managed','completed','{}','claude-code','m','ticket',NULLIF($4,'')::uuid,NULLIF($5,'')::jsonb,$6,$7,$8,$9,
			now() - interval '1 hour' + (random() * interval '1 minute'), now()) RETURNING id`,
			ws, ticket, agent, parent, checks, cost, setup, harness, chk).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	n := func(v int) *int { return &v }
	// P1 delegates once, to A (checked and passed, timed). P2 delegates to A (older run: unchecked,
	// untimed) and to B (checked, failed). A parent's cost is split evenly across its children.
	p1 := insert(agentB, "", "", 1000, nil, nil, nil)
	p2 := insert(agentB, "", "", 600, nil, nil, nil)
	insert(agentA, p1, `{"passed":true}`, 10, n(1000), n(2000), n(500))
	insert(agentA, p2, "", 20, nil, nil, nil)
	insert(agentB, p2, `{"passed":false}`, 30, n(3000), n(4000), n(1500))

	out := e.want("GET", "/delegation-efficiency", nil, http.StatusOK)
	rows := map[string]map[string]any{}
	for _, a := range out["agents"].([]any) {
		m := a.(map[string]any)
		rows[m["agent_id"].(string)] = m
	}
	a, b := rows[agentA], rows[agentB]
	if a == nil || b == nil {
		t.Fatalf("rows %v", out)
	}
	num := func(m map[string]any, k string) float64 {
		t.Helper()
		v, ok := m[k].(float64)
		if !ok {
			t.Fatalf("%s missing in %v", k, m)
		}
		return v
	}
	check := func(m map[string]any, k string, want float64) {
		t.Helper()
		if got := num(m, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%v %s = %v, want %v", m["handle"], k, got, want)
		}
	}
	// Agent A: 2 delegations, only one checked (and it passed): the unchecked one is not a failure.
	check(a, "delegations", 2)
	check(a, "checked", 1)
	check(a, "first_pass", 1)
	check(a, "first_pass_rate", 1)
	check(a, "parent_cost_usd_micros", 1000+300)
	check(a, "avg_setup_seconds", 1)
	check(a, "avg_harness_seconds", 2)
	check(a, "avg_checks_seconds", 0.5)
	// Agent B: one checked failure; shares P2's cost with A.
	check(b, "delegations", 1)
	check(b, "checked", 1)
	check(b, "first_pass", 0)
	check(b, "first_pass_rate", 0)
	check(b, "parent_cost_usd_micros", 300)
	check(b, "avg_harness_seconds", 4)
	if total := num(a, "parent_cost_usd_micros") + num(b, "parent_cost_usd_micros"); total > 1600 {
		t.Errorf("attributed parent cost %v exceeds the distinct parent total 1600", total)
	}

	// Nothing checked: the rate is 0, not a division by zero.
	if _, err := e.pool.Exec(ctx, `UPDATE runs SET checks = NULL WHERE parent_run_id IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	out = e.want("GET", "/delegation-efficiency", nil, http.StatusOK)
	for _, x := range out["agents"].([]any) {
		m := x.(map[string]any)
		if m["checked"].(float64) != 0 || m["first_pass_rate"].(float64) != 0 {
			t.Errorf("unchecked row %v", m)
		}
	}
}
