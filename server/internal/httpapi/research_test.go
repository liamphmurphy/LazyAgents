package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const planReply = "Here's a plan.\n\n```research-plan\nLocal TTS\n- Quality | Which sounds best?\n- Latency | How fast?\n```\n"

func (e *chatEnv) planMessage(t *testing.T, chat string) string {
	t.Helper()
	m, err := e.q.AppendChatMessage(t.Context(), store.AppendChatMessageParams{SessionID: uuid.MustParse(chat), Role: "assistant", BodyMd: planReply, ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	return m.ID.String()
}

func planBody(msg string, extra map[string]any) map[string]any {
	b := map[string]any{"message_id": msg, "title": "Local TTS", "max_sources": 40, "max_minutes": 30,
		"questions": []map[string]any{{"label": "Quality", "question": "Which sounds best?"}, {"label": "Latency", "question": "How fast?"}}}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func (e *chatEnv) researchRuns(t *testing.T, job string) []uuid.UUID {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT run_id FROM research_tasks WHERE job_id = $1 ORDER BY seq`, job)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func (e *chatEnv) setRunState(t *testing.T, id uuid.UUID, state string) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), `UPDATE runs SET state = $2 WHERE id = $1`, id, state); err != nil {
		t.Fatal(err)
	}
}

func (e *chatEnv) job(t *testing.T, chat string) map[string]any {
	t.Helper()
	return e.want("GET", "/chats/"+chat+"/research", nil, 200)["jobs"].([]any)[0].(map[string]any)
}

var oneFinding = []runs.ResearchFinding{{Claim: "Kokoro is fast", Sources: []runs.ResearchSource{{Title: "Repo", URL: "https://github.com/hexgrad/kokoro"}}}}

func TestResearchStartsOneReadOnlyRunPerQuestionAndRejectsBadPlans(t *testing.T) {
	e := newChatEnv(t)
	chat := e.newChat(nil)["id"].(string)
	msg := e.planMessage(t, chat)

	e.invalid("POST", "/chats/"+chat+"/research", planBody(msg, map[string]any{"max_sources": 500}), "max_sources", "between")
	e.invalid("POST", "/chats/"+chat+"/research", planBody(msg, map[string]any{"max_minutes": 1}), "max_minutes", "between")
	e.invalid("POST", "/chats/"+chat+"/research", planBody(msg, map[string]any{"max_cost_usd": -1}), "max_cost_usd", "omitted")
	e.want("POST", "/chats/"+chat+"/research", planBody(msg, map[string]any{"report_length": "epic"}), http.StatusBadRequest)
	e.want("POST", "/chats/"+chat+"/research", planBody(msg, map[string]any{"report_tone": "limerick"}), http.StatusBadRequest)

	job := e.want("POST", "/chats/"+chat+"/research", planBody(msg, nil), http.StatusCreated)
	if job["state"] != "running" || len(job["tasks"].([]any)) != 2 || job["max_cost_usd"] != nil || job["report_length"] != "standard" || job["report_tone"] != "report" {
		t.Fatalf("%v", job)
	}
	e.want("POST", "/chats/"+chat+"/research", planBody(msg, nil), http.StatusConflict)

	ids := e.researchRuns(t, job["id"].(string))
	var purpose, state string
	var ticket *uuid.UUID
	var spec []byte
	if err := e.pool.QueryRow(t.Context(), `SELECT purpose, state, ticket_id, resolved_spec FROM runs WHERE id = $1`, ids[0]).Scan(&purpose, &state, &ticket, &spec); err != nil {
		t.Fatal(err)
	}
	if purpose != "research" || state != "queued" || ticket != nil {
		t.Fatalf("%s %s %v", purpose, state, ticket)
	}
	for _, want := range []string{`"readonly": "true"`, `"web_search": "true"`, `"max_runtime": "30m"`} {
		if !containsBytes(spec, want) {
			t.Errorf("spec lacks %s: %s", want, spec)
		}
	}
	// A reply with no plan can't start anything.
	plain, _ := e.q.AppendChatMessage(t.Context(), store.AppendChatMessageParams{SessionID: uuid.MustParse(chat), Role: "assistant", BodyMd: "no plan", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	e.invalid("POST", "/chats/"+chat+"/research", planBody(plain.ID.String(), nil), "message_id", "doesn't propose")
}

func containsBytes(b []byte, s string) bool { return indexOf(string(b), s) >= 0 }

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func TestResearchRunsThroughToAReport(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	chat := e.newChat(nil)["id"].(string)
	created := e.want("POST", "/chats/"+chat+"/research", planBody(e.planMessage(t, chat), map[string]any{"report_length": "detailed", "report_tone": "essay"}), http.StatusCreated)
	if created["report_length"] != "detailed" || created["report_tone"] != "essay" {
		t.Fatalf("job %v", created)
	}
	job := created["id"].(string)
	ids := e.researchRuns(t, job)

	if _, err := e.runs.SubmitResearchFindings(ctx, ids[0], oneFinding); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.SubmitResearchFindings(ctx, ids[0], []runs.ResearchFinding{{Claim: "no source"}}); err == nil {
		t.Fatal("accepted a finding with no source")
	}
	// Not finished until every researcher is.
	e.setRunState(t, ids[0], "completed")
	if err := e.runs.AdvanceResearch(ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.job(t, chat)["state"]; s != "running" {
		t.Fatalf("state %v", s)
	}
	e.setRunState(t, ids[1], "failed")
	if err := e.runs.AdvanceResearch(ctx); err != nil {
		t.Fatal(err)
	}
	j := e.job(t, chat)
	if j["state"] != "writing" {
		t.Fatalf("job %v", j)
	}
	if c := e.want("GET", "/chats/"+chat, nil, 200); c["state"] != "running" {
		t.Fatalf("chat %v", c)
	}
	var prompt string
	if err := e.pool.QueryRow(ctx, `SELECT t.input_message FROM run_turns t JOIN chat_sessions c ON c.current_run_id = t.run_id WHERE c.id = $1 ORDER BY t.seq DESC LIMIT 1`, chat).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if indexOf(prompt, "Kokoro is fast") < 0 || indexOf(prompt, "untrusted") < 0 || indexOf(prompt, "3,500 to 5,000 words") < 0 || indexOf(prompt, "as an essay") < 0 {
		t.Fatalf("prompt %q", prompt)
	}

	key := e.want("GET", "/chats/"+chat, nil, 200)["project"].(string)
	var agent store.Agent
	var err error
	if agent, err = agents.EnsureAssistant(ctx, e.q, e.me.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	report := runs.ResearchReport{BottomLine: "Use Kokoro.", KeyFindings: []runs.ResearchKeyFinding{{Text: "Fast", Sources: []int{1}}},
		Sources: []runs.ResearchNumberedSource{{N: 1, Title: "Repo", URL: "https://github.com/hexgrad/kokoro"}}, BodyMD: "# Report\n\nFast [1].\n"}
	caller := agents.Caller{Agent: agent}
	if _, _, err := e.runs.SaveResearchReport(ctx, caller, uuid.MustParse(chat), key, report); err == nil {
		t.Fatal("accepted a report without the required sections")
	}
	report.BodyMD += "\n## Where sources disagree\nNothing.\n\n## Not covered\nPricing.\n"
	if _, _, err := e.runs.SaveResearchReport(ctx, caller, uuid.MustParse(chat), key, report); err != nil {
		t.Fatal(err)
	}
	j = e.job(t, chat)
	if j["state"] != "done" || j["artifact_id"] == nil || j["report"].(map[string]any)["bottom_line"] != "Use Kokoro." {
		t.Fatalf("job %v", j)
	}
}

func TestResearchSpendLimitStopsResearchersUnlimitedDoesNot(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	run := func(extra map[string]any) (string, []uuid.UUID) {
		chat := e.newChat(nil)["id"].(string)
		job := e.want("POST", "/chats/"+chat+"/research", planBody(e.planMessage(t, chat), extra), http.StatusCreated)["id"].(string)
		ids := e.researchRuns(t, job)
		if _, err := e.runs.SubmitResearchFindings(ctx, ids[0], oneFinding); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE runs SET cost_usd_micros = 9000000 WHERE id = $1`, ids[0]); err != nil {
			t.Fatal(err)
		}
		return chat, ids
	}
	capped, ids := run(map[string]any{"max_cost_usd": 6})
	unlimited, _ := run(nil)
	if err := e.runs.AdvanceResearch(ctx); err != nil {
		t.Fatal(err)
	}
	if j := e.job(t, capped); j["state"] != "writing" || j["stop_reason"] != "spend" {
		t.Fatalf("capped %v", j)
	}
	var state string
	_ = e.pool.QueryRow(ctx, `SELECT state FROM runs WHERE id = $1`, ids[1]).Scan(&state)
	if state != "cancelled" {
		t.Fatalf("researcher %s", state)
	}
	if j := e.job(t, unlimited); j["state"] != "running" {
		t.Fatalf("unlimited %v", j)
	}
}

func TestResearchStopAndFinish(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	start := func() (string, string, []uuid.UUID) {
		chat := e.newChat(nil)["id"].(string)
		job := e.want("POST", "/chats/"+chat+"/research", planBody(e.planMessage(t, chat), nil), http.StatusCreated)["id"].(string)
		return chat, job, e.researchRuns(t, job)
	}
	chat, job, _ := start()
	if j := e.want("POST", "/chats/"+chat+"/research/"+job+"/stop", nil, 200); j["state"] != "stopped" {
		t.Fatalf("%v", j)
	}
	e.want("POST", "/chats/"+chat+"/research/"+job+"/stop", nil, http.StatusConflict)

	chat, job, ids := start()
	if _, err := e.runs.SubmitResearchFindings(ctx, ids[1], oneFinding); err != nil {
		t.Fatal(err)
	}
	e.want("POST", "/chats/"+chat+"/research/"+job+"/finish", nil, 200)
	if j := e.job(t, chat); j["state"] != "writing" {
		t.Fatalf("%v", j)
	}

	// Nothing found: no report to write.
	chat, job, _ = start()
	e.want("POST", "/chats/"+chat+"/research/"+job+"/finish", nil, 200)
	if j := e.job(t, chat); j["state"] != "failed" || j["error"] == nil {
		t.Fatalf("%v", j)
	}
	e.want("POST", "/chats/"+uuid.NewString()+"/research/"+job+"/stop", nil, http.StatusNotFound)
}

func TestResearchRunsQueueBehindOtherWork(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	chat := e.newChat(nil)["id"].(string)
	job := e.want("POST", "/chats/"+chat+"/research", planBody(e.planMessage(t, chat), nil), http.StatusCreated)["id"].(string)
	ids := e.researchRuns(t, job)
	other := e.newChat(map[string]any{"message": map[string]any{"body": "hi"}})["current_run_id"].(string)
	runner := "r1"
	claimed, err := e.q.ClaimQueuedRun(ctx, &runner)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID.String() != other || claimed.ID == ids[0] {
		t.Fatalf("claimed %s, want the chat run %s", claimed.ID, other)
	}
}

// A report written in a space's chat joins the space's sources.
func TestResearchReportInASpaceBecomesASource(t *testing.T) {
	e := newChatEnv(t)
	ctx := t.Context()
	space := e.want("POST", "/chat-spaces", map[string]any{"name": "Voice", "project": "PAY"}, http.StatusCreated)["id"].(string)
	chat := e.newChat(map[string]any{"space_id": space})["id"].(string)
	job := e.want("POST", "/chats/"+chat+"/research", planBody(e.planMessage(t, chat), nil), http.StatusCreated)["id"].(string)
	ids := e.researchRuns(t, job)
	if _, err := e.runs.SubmitResearchFindings(ctx, ids[0], oneFinding); err != nil {
		t.Fatal(err)
	}
	e.setRunState(t, ids[0], "completed")
	e.setRunState(t, ids[1], "completed")
	if err := e.runs.AdvanceResearch(ctx); err != nil {
		t.Fatal(err)
	}
	var prompt string
	if err := e.pool.QueryRow(ctx, `SELECT t.input_message FROM run_turns t JOIN chat_sessions c ON c.current_run_id = t.run_id WHERE c.id = $1 ORDER BY t.seq DESC LIMIT 1`, chat).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if indexOf(prompt, `in the space "Voice"`) < 0 {
		t.Fatalf("write-up prompt doesn't mention the space: %q", prompt)
	}

	agent, err := agents.EnsureAssistant(ctx, e.q, e.me.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	report := runs.ResearchReport{BottomLine: "Use Kokoro.", KeyFindings: []runs.ResearchKeyFinding{{Text: "Fast", Sources: []int{1}}},
		Sources: []runs.ResearchNumberedSource{{N: 1, Title: "Repo [main]", URL: "https://github.com/hexgrad/kokoro"}},
		BodyMD:  "Fast [1].\n\n## Where sources disagree\nNothing.\n\n## Not covered\nPricing.\n"}
	_, rs, err := e.runs.SaveResearchReport(ctx, agents.Caller{Agent: agent}, uuid.MustParse(chat), "PAY", report)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Space != "Voice" || !rs.Added {
		t.Fatalf("report space %+v", rs)
	}
	list := e.want("GET", "/chat-spaces/"+space+"/sources", nil, http.StatusOK)["sources"].([]any)
	if len(list) != 1 {
		t.Fatalf("sources %v", list)
	}
	src := list[0].(map[string]any)
	if src["kind"] != "research" || src["title"] != "Local TTS" || src["chat_id"] != chat || src["delivery"] != "inline" {
		t.Fatalf("source %v", src)
	}
	var body string
	if err := e.pool.QueryRow(ctx, `SELECT body FROM chat_space_sources WHERE id = $1`, src["id"]).Scan(&body); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Local TTS", "**Bottom line:** Use Kokoro.", "Fast [1].", "## Sources", `1. [Repo \[main\]](https://github.com/hexgrad/kokoro)`} {
		if indexOf(body, want) < 0 {
			t.Errorf("source body lacks %q:\n%s", want, body)
		}
	}
}
