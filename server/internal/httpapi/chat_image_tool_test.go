package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// generate_image queues an image only in a chat on a non-Codex harness whose
// persona has images on right now, with Codex available, twice per turn.
func TestGenerateImageTool(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	codexUp := true
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts, ModelDiscovery: func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		if h == agentdef.HarnessCodex && !codexUp {
			return nil, errors.New("down")
		}
		return []chatmodels.Model{{ID: h + "-m1"}, {ID: h + "-m2", Default: true}}, nil
	}}
	handler := (&Server{Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	call := func(token string, args any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/generate_image", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	images := true
	ps := &personas.Service{Pool: pool}
	owner := personas.Owner{WorkspaceID: ws, UserID: me.ID}
	p, err := ps.Create(ctx, owner, personas.NewInput{Name: "Illustrator", Images: &images})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rs.PostChatMessage(ctx, ws, me.ID, chat.ID, runs.ChatMessageInput{Body: "draw a fox"}); err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	runID := *chat.CurrentRunID
	assistant, _ := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: ws})
	token, _, err := agents.IssueToken(ctx, q, assistant.ID, "run", "chat", &runID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE runs SET state='running', harness='claude' WHERE id=$1", runID)
	exec("UPDATE run_turns SET status='running' WHERE run_id=$1", runID)
	jobs := func() []store.ChatImageJob {
		t.Helper()
		var out []store.ChatImageJob
		rows, err := pool.Query(ctx, "SELECT id FROM chat_image_jobs WHERE run_id=$1", runID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var j store.ChatImageJob
			if err := rows.Scan(&j.ID); err != nil {
				t.Fatal(err)
			}
			out = append(out, j)
		}
		return out
	}
	reject := func(label string, args any, code int, msg string) {
		t.Helper()
		before := len(jobs())
		got, out := call(token, args)
		if got != code || !strings.Contains(out["message"].(string), msg) || len(jobs()) != before {
			t.Fatalf("%s: %d %v (jobs %d -> %d)", label, got, out, before, len(jobs()))
		}
	}

	reject("empty prompt", map[string]any{"prompt": "  "}, 422, "1–4000")
	reject("too long", map[string]any{"prompt": strings.Repeat("x", 4001)}, 422, "1–4000")

	exec("UPDATE runs SET harness='codex' WHERE id=$1", runID)
	reject("codex chat", map[string]any{"prompt": "a fox"}, 409, "use your own image generation tool")
	exec("UPDATE runs SET harness='claude' WHERE id=$1", runID)

	codexUp = false
	reject("codex unavailable", map[string]any{"prompt": "a fox"}, 409, "Image generation needs Codex: ")
	codexUp = true

	code, out := call(token, map[string]any{"prompt": "a red fox, watercolor"})
	if code != 200 || !strings.Contains(out["tell_the_person"].(string), "don't describe it as finished") {
		t.Fatalf("queue: %d %v", code, out)
	}
	var model, state, prompt string
	if err := pool.QueryRow(ctx, "SELECT model, state, prompt FROM chat_image_jobs WHERE run_id=$1", runID).Scan(&model, &state, &prompt); err != nil || model != "codex-m2" || state != "queued" || prompt != "a red fox, watercolor" {
		t.Fatalf("job: %q %q %q %v", model, state, prompt, err)
	}
	if code, out = call(token, map[string]any{"prompt": "another"}); code != 200 {
		t.Fatalf("second: %d %v", code, out)
	}
	reject("third in a turn", map[string]any{"prompt": "a third"}, 409, "at most 2")

	// The Codex step shares the run's token but can't queue images for itself.
	exec("UPDATE chat_image_jobs SET state='running' WHERE run_id=$1", runID)
	reject("during a step", map[string]any{"prompt": "a loop"}, 409, "being made right now")
	exec("UPDATE chat_image_jobs SET state='done' WHERE run_id=$1", runID)

	// A new turn has its own allowance.
	exec("UPDATE run_turns SET status='done' WHERE run_id=$1", runID)
	exec("INSERT INTO run_turns (run_id, seq, trigger, status) VALUES ($1, 2, 'comment', 'running')", runID)
	if code, out = call(token, map[string]any{"prompt": "next turn"}); code != 200 {
		t.Fatalf("next turn: %d %v", code, out)
	}

	// Turning images off takes effect immediately, and so does a deleted persona.
	off := false
	if _, err := ps.Update(ctx, owner, p.ID, personas.Patch{Revision: p.Revision, Images: &off}); err != nil {
		t.Fatal(err)
	}
	reject("images off", map[string]any{"prompt": "a fox"}, 409, "can't generate images")
}
