package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
)

func TestGetHighlights(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	host := e.myDefaultPersona()["id"].(string)
	guest := e.want("POST", "/personas", map[string]any{"name": "Guest"}, 201)["id"].(string)
	var agent, project, stranger uuid.UUID
	for _, v := range []struct {
		dst *uuid.UUID
		sql string
	}{
		{&agent, `SELECT id FROM agents LIMIT 1`},
		{&project, `SELECT id FROM projects WHERE key='PAY' LIMIT 1`},
		{&stranger, `SELECT id FROM users WHERE id <> '` + e.me.ID.String() + `' LIMIT 1`},
	} {
		if err := e.pool.QueryRow(ctx, v.sql).Scan(v.dst); err != nil {
			t.Fatal(err)
		}
	}
	makeChat := func(owner uuid.UUID, pr any, purpose string) (uuid.UUID, uuid.UUID) {
		t.Helper()
		chat, run := uuid.New(), uuid.New()
		exec(`INSERT INTO runs (id,workspace_id,agent_id,mode,state,resolved_spec,harness,model,purpose) VALUES ($1,$2,$3,'managed','completed','{}','codex','current-model',$4)`, run, e.me.WorkspaceID, agent, purpose)
		exec(`INSERT INTO chat_sessions (id,workspace_id,created_by,project_id,persona_id,agent_id,harness,model,current_run_id,title) VALUES ($1,$2,$3,$4,$5,$6,'codex','current-model',$7,'Private title')`, chat, e.me.WorkspaceID, owner, pr, host, agent, run)
		return chat, run
	}
	chat, run := makeChat(e.me.ID, project, "chat")
	turn := uuid.New()
	exec(`INSERT INTO run_turns (id,run_id,seq,trigger,status,effort,effort_level,effort_reasons,process_started_at,ended_at) VALUES ($1,$2,1,'comment','done','high','xhigh',ARRAY['confirmation'],'2020-03-01 23:59:45Z','2020-03-01 23:59:55Z')`, turn, run)
	message := func(c, r uuid.UUID, seq int, tr any, speaker any, at string, role string) {
		t.Helper()
		exec(`INSERT INTO chat_messages (session_id,seq,role,body_md,run_id,run_turn_id,persona_id,harness,model,created_at) VALUES ($1,$2,$3,'Secret content',$4,$5,$6,'codex','reply-model',$7::timestamptz)`, c, seq, role, r, tr, speaker, at)
	}
	message(chat, run, 1, turn, guest, "2020-03-01 23:59:55Z", "assistant")
	message(chat, run, 2, turn, guest, "2020-03-02 00:00:01Z", "assistant") // Same turn, next day: still one sample.
	message(chat, run, 3, nil, nil, "2020-03-02 12:00:00Z", "assistant")    // Legacy; no guessed persona/effort.
	message(chat, run, 4, nil, host, "2020-03-02 13:00:00Z", "user")
	otherChat, otherRun := makeChat(stranger, project, "chat")
	message(otherChat, otherRun, 1, nil, host, "2020-03-02 12:00:00Z", "assistant")
	noProject, noProjectRun := makeChat(e.me.ID, nil, "chat")
	message(noProject, noProjectRun, 1, nil, host, "2020-03-02 12:00:00Z", "assistant")
	planningChat, planningRun := makeChat(e.me.ID, project, "planning")
	message(planningChat, planningRun, 1, nil, host, "2020-03-02 12:00:00Z", "assistant")

	get := func(query string) []api.HighlightsBucket {
		t.Helper()
		raw := e.want("GET", "/highlights?"+query, nil, 200)
		data, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var out api.GetHighlights200JSONResponse
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		return out.Buckets
	}
	count := func(rows []api.HighlightsBucket) int64 {
		var n int64
		for _, r := range rows {
			n += r.Turns
		}
		return n
	}
	rows := get("from=2020-03-01&to=2020-03-02&project=PAY")
	if count(rows) != 2 {
		t.Fatalf("want one Auto turn plus legacy reply, got %+v", rows)
	}
	var found bool
	for _, r := range rows {
		if r.Reasons == nil {
			t.Fatalf("reasons must be an array: %+v", r)
		}
		if r.EffortLevel != "" {
			found = true
			if r.PersonaId != guest || r.Persona != "Guest" || r.Model != "reply-model" || r.Effort != "high" || r.EffortLevel != "xhigh" || r.DurationMs != 10000 || r.TimedTurns != 1 || len(r.Reasons) != 1 {
				t.Fatalf("wrong routing attribution: %+v", r)
			}
		} else if r.PersonaId != "" || r.Effort != "" {
			t.Fatalf("legacy guessed: %+v", r)
		}
	}
	if !found {
		t.Fatal("Auto turn missing")
	}
	if n := count(get("from=2020-03-01&to=2020-03-02")); n != 3 {
		t.Fatalf("all projects: %d", n)
	}
	if n := count(get("from=2020-03-01&to=2020-03-01")); n != 1 {
		t.Fatalf("UTC first day: %d", n)
	}
	if n := count(get("from=2020-03-02&to=2020-03-02&project=PAY")); n != 1 {
		t.Fatalf("turn counted again next day: %d", n)
	}
	if n := count(get("from=2020-03-01&to=2020-03-02&project=MISSING")); n != 0 {
		t.Fatalf("missing project: %d", n)
	}
	for _, q := range []string{"from=2020-03-02&to=2020-03-01", "from=2020-01-01&to=2021-01-01"} {
		e.want("GET", "/highlights?"+q, nil, 422)
	}
	e.want("GET", "/highlights?from=invalid&to=2020-03-01", nil, 400)
	e.want("GET", "/highlights", nil, 400)
	exec(`DELETE FROM personas WHERE id=$1`, guest)
	for _, r := range get("from=2020-03-01&to=2020-03-02&project=PAY") {
		if r.PersonaId != "" || r.Persona != "Unknown persona" {
			t.Fatalf("deleted speaker attributed to host: %+v", r)
		}
	}

}
