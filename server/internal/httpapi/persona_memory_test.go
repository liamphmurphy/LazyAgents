package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// Chat memory tools: about picks the person's memory or the chat persona's
// own, move_to moves an entry between them, a chat without a persona has no
// persona memory, and no chat reaches another persona's entries. The memory
// API lists everything, labelled with persona_id, and filters by about and
// persona_id.
func TestPersonaMemoryTools(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	po := personas.Owner{WorkspaceID: ws, UserID: me.ID}
	gwen, err := (&personas.Service{Pool: pool}).Create(ctx, po, personas.NewInput{Name: "Gwen"})
	if err != nil {
		t.Fatal(err)
	}
	coach, err := (&personas.Service{Pool: pool}).Create(ctx, po, personas.NewInput{Name: "Coach"})
	if err != nil {
		t.Fatal(err)
	}
	// A chat run token, for a chat with this persona (nil: the built-in Assistant).
	chatToken := func(persona *uuid.UUID) string {
		t.Helper()
		chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY", Title: "Memory", PersonaID: persona, Message: &runs.ChatMessageInput{Body: "hi"}})
		if err != nil {
			t.Fatal(err)
		}
		chat, err = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
		if err != nil || chat.CurrentRunID == nil {
			t.Fatalf("chat run: %v", err)
		}
		if persona == nil && chat.PersonaID != nil {
			t.Fatalf("chat got a persona: %v", chat.PersonaID)
		}
		if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
			t.Fatal(err)
		}
		tok, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	call := func(token, name string, args any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	ok := func(token, name string, args any) map[string]any {
		t.Helper()
		code, out := call(token, name, args)
		if code != 200 {
			t.Fatalf("%s %v: %d %v", name, args, code, out)
		}
		return out
	}
	count := func(token string, args map[string]any) int {
		t.Helper()
		return len(ok(token, "search_memory", args)["memories"].([]any))
	}

	g, c := chatToken(&gwen.ID), chatToken(&coach.ID)
	// The default persona may be the built-in Assistant; force a chat without one.
	if err := q.ClearDefaultPersona(ctx, store.ClearDefaultPersonaParams{WorkspaceID: ws, UserID: me.ID, KeepID: uuid.Nil}); err != nil {
		t.Fatal(err)
	}
	plain := chatToken(nil)

	ok(g, "save_memory", map[string]any{"title": "Lives with two cats", "type": "fact"})
	self := ok(g, "save_memory", map[string]any{"title": "Auburn hair and glasses", "about": "persona"})
	if self["about"] != "persona" {
		t.Fatalf("persona save = %v", self)
	}
	moment := ok(g, "save_memory", map[string]any{"title": "Walk by the river", "type": "moment", "about": "persona"})
	if code, _ := call(g, "save_memory", map[string]any{"title": "A moment", "type": "moment"}); code != 422 {
		t.Errorf("moment in the person's memory: %d", code)
	}
	if code, _ := call(plain, "save_memory", map[string]any{"title": "Me", "about": "persona"}); code != 422 {
		t.Errorf("persona memory in a chat without a persona: %d", code)
	}
	if code, _ := call(g, "save_memory", map[string]any{"title": "x", "about": "someone"}); code != 422 {
		t.Errorf("bad about: %d", code)
	}
	if code, _ := call(g, "save_memory", map[string]any{"title": "Token", "body": "AKIAABCDEFGHIJKLMNOP", "about": "persona"}); code != 422 {
		t.Errorf("secret in persona memory: %d", code)
	}

	if n := count(g, map[string]any{}); n != 1 {
		t.Errorf("person search in gwen's chat = %d, want 1", n)
	}
	if n := count(g, map[string]any{"about": "persona"}); n != 2 {
		t.Errorf("gwen's own memory = %d, want 2", n)
	}
	if n := count(c, map[string]any{"about": "persona"}); n != 0 {
		t.Errorf("coach sees %d of gwen's entries", n)
	}
	if n := count(plain, map[string]any{}); n != 1 {
		t.Errorf("plain chat person memory = %d", n)
	}

	// Gwen's chat reads its own entries by id; Coach's chat can't read, change or forget them.
	sid := self["id"].(string)
	if out := ok(g, "read_memory", map[string]any{"id": sid}); out["about"] != "persona" {
		t.Errorf("read own = %v", out)
	}
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{{"read_memory", map[string]any{"id": sid}}, {"update_memory", map[string]any{"id": sid, "title": "x"}}, {"forget_memory", map[string]any{"id": sid}}} {
		if code, _ := call(c, tc.tool, tc.args); code != 404 {
			t.Errorf("coach %s on gwen's entry: %d", tc.tool, code)
		}
	}

	// Move the person's "who Gwen is" entry into Gwen's own memory, and back.
	who := ok(g, "save_memory", map[string]any{"title": "Gwen's own memory: who she is", "body": "Warm, curious."})
	wid := who["id"].(string)
	moved := ok(g, "update_memory", map[string]any{"id": wid, "move_to": "persona"})
	if m := moved["memory"].(map[string]any); m["about"] != "persona" {
		t.Fatalf("moved = %v", moved)
	}
	if n := count(g, map[string]any{"about": "persona"}); n != 3 {
		t.Errorf("gwen's memory after move = %d, want 3", n)
	}
	if code, _ := call(plain, "update_memory", map[string]any{"id": wid, "move_to": "person"}); code != 404 {
		t.Errorf("plain chat moved gwen's entry: %d", code)
	}
	if code, _ := call(g, "update_memory", map[string]any{"id": moment["id"], "move_to": "person"}); code != 422 {
		t.Errorf("moment moved to the person's memory: %d", code)
	}
	if out := ok(g, "update_memory", map[string]any{"id": wid, "move_to": "person"}); out["memory"].(map[string]any)["about"] != "person" {
		t.Errorf("move back = %v", out)
	}
	ok(g, "update_memory", map[string]any{"id": wid, "move_to": "persona"})

	// The memory API: everything by default, labelled; filtered by about and persona_id.
	list := func(query string) (int, []map[string]any) {
		t.Helper()
		rec := get(handler, "/api/v1/memories"+query)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", query, rec.Code, rec.Body)
		}
		var body struct {
			Memories []map[string]any `json:"memories"`
			Total    int              `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Total, body.Memories
	}
	for query, want := range map[string]int{"": 4, "?about=person": 1, "?about=persona": 3, "?persona_id=" + gwen.ID.String(): 3,
		"?persona_id=" + coach.ID.String(): 0, "?about=persona&type=moment": 1} {
		if n, _ := list(query); n != want {
			t.Errorf("list %q total = %d, want %d", query, n, want)
		}
	}
	if _, ms := list("?about=persona&type=moment&sort=created"); len(ms) != 1 || ms[0]["persona_id"] != gwen.ID.String() {
		t.Errorf("moments = %v", ms)
	}
	if rec := get(handler, "/api/v1/memories?about=nobody"); rec.Code == http.StatusOK {
		t.Errorf("bad about accepted")
	}

	// Adding a moment by hand: to a persona only.
	rec := sendJSON(handler, http.MethodPost, "/api/v1/memories", fmt.Sprintf(`{"title":"Pinned by me","type":"moment","persona_id":%q}`, coach.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("create moment: %d %s", rec.Code, rec.Body)
	}
	if rec := sendJSON(handler, http.MethodPost, "/api/v1/memories", `{"title":"Mine","type":"moment"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("person moment: %d %s", rec.Code, rec.Body)
	}
	if rec := sendJSON(handler, http.MethodPost, "/api/v1/memories", fmt.Sprintf(`{"title":"x","persona_id":%q}`, uuid.New())); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown persona: %d %s", rec.Code, rec.Body)
	}
	if n, _ := list("?persona_id=" + coach.ID.String()); n != 1 {
		t.Errorf("coach moments = %d", n)
	}

	// The manager deletes any of the person's entries; delete-all can be narrowed.
	if rec := sendJSON(handler, http.MethodDelete, "/api/v1/memories/"+sid, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete persona entry: %d", rec.Code)
	}
	if rec := sendJSON(handler, http.MethodDelete, "/api/v1/memories?confirm=true&about=person", ""); rec.Code != http.StatusOK {
		t.Errorf("delete person memory: %d %s", rec.Code, rec.Body)
	}
	if n, _ := list(""); n != 3 {
		t.Errorf("after deleting the person's memory: %d left, want gwen's 2 and coach's 1", n)
	}
}
