package db_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

type kanbanFixture struct {
	ws, project string
}

func seedKanban(t *testing.T, pool *pgxpool.Pool) kanbanFixture {
	t.Helper()
	ctx := context.Background()
	var f kanbanFixture
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&f.ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO projects(workspace_id,key,name,color) VALUES($1,'PRJ','P','#000') RETURNING id`, f.ws).Scan(&f.project); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f kanbanFixture) ticket(t *testing.T, pool *pgxpool.Pool, n int, status string, epic any) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO tickets(workspace_id,project_id,number,key,title,status,epic_id) VALUES($1,$2,$3,$4,'T',$5,$6) RETURNING id`,
		f.ws, f.project, n, fmt.Sprintf("PRJ-%d", n), status, epic).Scan(&id)
	if err != nil {
		t.Fatalf("insert ticket %d (%s): %v", n, status, err)
	}
	return id
}

func (f kanbanFixture) epic(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,$3) RETURNING id`, f.ws, f.project, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestKanbanMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	// Seed one ticket per pre-00025 status, then migrate forward.
	gooseTo(t, pool, false, 24)
	f := seedKanban(t, pool)
	oldStatuses := []string{"backlog", "todo", "in_progress", "in_review", "done"}
	ids := map[string]string{}
	for i, s := range oldStatuses {
		ids[s] = f.ticket(t, pool, i+1, s, nil)
	}
	var oldEpic string
	if err := pool.QueryRow(ctx, `INSERT INTO epics(workspace_id,project_id,name) VALUES($1,$2,'Old') RETURNING id`, f.ws, f.project).Scan(&oldEpic); err != nil {
		t.Fatal(err)
	}

	gooseTo(t, pool, true, 25)

	for _, s := range oldStatuses {
		var got string
		var rev int
		var points *int
		if err := pool.QueryRow(ctx, `SELECT status, revision, estimate_points FROM tickets WHERE id=$1`, ids[s]).Scan(&got, &rev, &points); err != nil {
			t.Fatal(err)
		}
		if got != s {
			t.Errorf("status %q became %q after the migration", s, got)
		}
		if rev != 1 || points != nil {
			t.Errorf("%s: revision=%d estimate_points=%v, want 1 and NULL", s, rev, points)
		}
	}
	var epicStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM epics WHERE id=$1`, oldEpic).Scan(&epicStatus); err != nil {
		t.Fatal(err)
	}
	if epicStatus != "open" {
		t.Errorf("existing epic status = %q, want open", epicStatus)
	}

	// canceled is now valid; down maps it to done (closed, never dispatched).
	canceled := f.ticket(t, pool, 10, "canceled", nil)
	mustExec(t, pool, `UPDATE tickets SET estimate_points=5 WHERE id=$1`, ids["todo"])
	mustExec(t, pool, `INSERT INTO epic_criteria(epic_id,position,text) VALUES($1,1,'c')`, oldEpic)

	gooseTo(t, pool, false, 24)
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, canceled).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "done" {
		t.Errorf("canceled ticket after down = %q, want done", got)
	}
	for _, s := range oldStatuses {
		if err := pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, ids[s]).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != s {
			t.Errorf("status %q became %q after down", s, got)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE (table_name='tickets' AND column_name IN ('revision','estimate_points','estimate_rationale','estimated_by_type','estimated_by_id','estimated_at')) OR (table_name='epics' AND column_name='status') OR table_name='epic_criteria'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d 00025 columns remain after down", n)
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET status='canceled' WHERE id=$1`, canceled); err == nil {
		t.Error("old schema accepted canceled after down")
	}
	// And up again works.
	gooseTo(t, pool, true, 25)
}

func TestKanbanConstraints(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	f := seedKanban(t, pool)
	ticket := f.ticket(t, pool, 1, "backlog", nil)

	for _, s := range []string{"backlog", "todo", "in_progress", "in_review", "done", "canceled"} {
		if _, err := pool.Exec(ctx, `UPDATE tickets SET status=$2 WHERE id=$1`, ticket, s); err != nil {
			t.Errorf("status %q rejected: %v", s, err)
		}
	}
	for _, s := range []string{"blocked", "in review", "Done", ""} {
		if _, err := pool.Exec(ctx, `UPDATE tickets SET status=$2 WHERE id=$1`, ticket, s); err == nil {
			t.Errorf("invalid status %q accepted", s)
		}
	}

	for _, p := range []int{1, 2, 3, 5, 8, 13} {
		if _, err := pool.Exec(ctx, `UPDATE tickets SET estimate_points=$2 WHERE id=$1`, ticket, p); err != nil {
			t.Errorf("estimate %d rejected: %v", p, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET estimate_points=NULL WHERE id=$1`, ticket); err != nil {
		t.Errorf("clearing the estimate rejected: %v", err)
	}
	for _, p := range []int{-1, 0, 4, 21, 100} {
		if _, err := pool.Exec(ctx, `UPDATE tickets SET estimate_points=$2 WHERE id=$1`, ticket, p); err == nil {
			t.Errorf("estimate %d outside the scale accepted", p)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE tickets SET estimated_by_type='robot' WHERE id=$1`, ticket); err == nil {
		t.Error("estimated_by_type 'robot' accepted")
	}

	epic := f.epic(t, pool, "E")
	if _, err := pool.Exec(ctx, `UPDATE epics SET status='closed' WHERE id=$1`, epic); err == nil {
		t.Error("epic status 'closed' accepted")
	}
	mustExec(t, pool, `UPDATE epics SET status='done' WHERE id=$1`, epic)
}

func TestEpicCriteria(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	f := seedKanban(t, pool)
	epic, other := f.epic(t, pool, "E"), f.epic(t, pool, "Other")

	add := func(e string, pos int) error {
		_, err := pool.Exec(ctx, `INSERT INTO epic_criteria(epic_id,position,text) VALUES($1,$2,'c')`, e, pos)
		return err
	}
	for pos := 1; pos <= 3; pos++ {
		if err := add(epic, pos); err != nil {
			t.Fatal(err)
		}
	}
	if err := add(epic, 2); err == nil {
		t.Error("duplicate (epic_id, position) accepted")
	}
	if err := add(other, 1); err != nil {
		t.Errorf("same position in another epic rejected: %v", err)
	}
	if err := add(epic, 0); err == nil {
		t.Error("position 0 accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO epic_criteria(epic_id,position,text) VALUES($1,9,'')`, epic); err == nil {
		t.Error("empty criterion text accepted")
	}

	// Delete the first criterion and close the gap in one statement (deferrable unique).
	mustExec(t, pool, `DELETE FROM epic_criteria WHERE epic_id=$1 AND position=1`, epic)
	mustExec(t, pool, `UPDATE epic_criteria SET position = position - 1 WHERE epic_id=$1 AND position > 1`, epic)
	rows, err := pool.Query(ctx, `SELECT position FROM epic_criteria WHERE epic_id=$1 ORDER BY position`, epic)
	if err != nil {
		t.Fatal(err)
	}
	var positions []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, p)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	if !slices.Equal(positions, []int{1, 2}) {
		t.Errorf("positions after compaction = %v, want [1 2]", positions)
	}

	// Criteria go with their epic.
	mustExec(t, pool, `DELETE FROM epics WHERE id=$1`, epic)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM epic_criteria WHERE epic_id=$1`, epic).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d epic_criteria rows survived their epic", n)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM epic_criteria WHERE epic_id=$1`, other).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("other epic's criteria = %d, want 1", n)
	}
}

func TestKanbanNotifyPayload(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	f := seedKanban(t, pool)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `LISTEN lazyagents_changes`); err != nil {
		t.Fatal(err)
	}

	// expect reads notifications until one of type typ arrives and checks its ids.
	expect := func(what, typ string, want ...string) {
		t.Helper()
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		for {
			n, err := conn.Conn().WaitForNotification(wctx)
			if err != nil {
				t.Fatalf("%s: no %s notification: %v", what, typ, err)
			}
			var p struct {
				Type string   `json:"type"`
				IDs  []string `json:"ids"`
			}
			if err := json.Unmarshal([]byte(n.Payload), &p); err != nil {
				t.Fatal(err)
			}
			if p.Type != typ {
				continue
			}
			slices.Sort(p.IDs)
			slices.Sort(want)
			if !slices.Equal(p.IDs, want) {
				t.Fatalf("%s: %s ids = %v, want %v", what, typ, p.IDs, want)
			}
			return
		}
	}
	// drain discards pending notifications so the next expect sees only new ones.
	drain := func() {
		for {
			wctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			_, err := conn.Conn().WaitForNotification(wctx)
			cancel()
			if err != nil {
				return
			}
		}
	}

	epic := f.epic(t, pool, "E")
	expect("epic insert", "epic.updated", epic)
	other := f.epic(t, pool, "Other")
	expect("second epic insert", "epic.updated", other)

	ticket := f.ticket(t, pool, 1, "backlog", epic)
	expect("ticket insert", "ticket.updated", ticket)
	drain()

	mustExec(t, pool, `UPDATE tickets SET status='in_progress' WHERE id=$1`, ticket)
	expect("ticket status change", "ticket.updated", ticket)
	drain()
	mustExec(t, pool, `UPDATE tickets SET estimate_points=3 WHERE id=$1`, ticket)
	expect("ticket estimate (epic rollup)", "epic.updated", epic)
	drain()
	mustExec(t, pool, `UPDATE tickets SET epic_id=$2 WHERE id=$1`, ticket, other)
	expect("ticket moved between epics", "epic.updated", epic, other)
	drain()

	mustExec(t, pool, `INSERT INTO acceptance_criteria(ticket_id,position,text) VALUES($1,1,'c')`, ticket)
	expect("ticket criterion insert (ticket id)", "ticket.updated", ticket)
	drain()

	var crit string
	if err := pool.QueryRow(ctx, `INSERT INTO epic_criteria(epic_id,position,text) VALUES($1,1,'c') RETURNING id`, other).Scan(&crit); err != nil {
		t.Fatal(err)
	}
	expect("epic criterion insert (epic id)", "epic.updated", other)
	mustExec(t, pool, `UPDATE epic_criteria SET done=true WHERE id=$1`, crit)
	expect("epic criterion check", "epic.updated", other)
	mustExec(t, pool, `UPDATE epics SET status='done' WHERE id=$1`, epic)
	expect("epic update", "epic.updated", epic)

	// A bulk change over the cap sends empty ids ("refetch all"), never a partial list.
	for i := 2; i <= 52; i++ {
		f.ticket(t, pool, i, "todo", nil)
	}
	drain()
	mustExec(t, pool, `UPDATE tickets SET priority='high' WHERE project_id=$1`, f.project)
	expect("bulk ticket update", "ticket.updated")

	mustExec(t, pool, `DELETE FROM tickets WHERE id=$1`, ticket)
	expect("ticket delete", "ticket.updated", ticket)
}
