package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// personaChatEnv is a person with two personas and a way to call agent tools as
// one of their chats.
type personaChatEnv struct {
	t       *testing.T
	q       *store.Queries
	rs      *runs.Service
	handler http.Handler
	me      store.User
	gwen    uuid.UUID
	coach   uuid.UUID
}

func newPersonaChatEnv(t *testing.T) *personaChatEnv {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	po := personas.Owner{WorkspaceID: me.WorkspaceID, UserID: me.ID}
	gwen, err := (&personas.Service{Pool: pool}).Create(ctx, po, personas.NewInput{Name: "Gwen"})
	if err != nil {
		t.Fatal(err)
	}
	coach, err := (&personas.Service{Pool: pool}).Create(ctx, po, personas.NewInput{Name: "Coach"})
	if err != nil {
		t.Fatal(err)
	}
	return &personaChatEnv{t: t, q: q, rs: rs, me: me, gwen: gwen.ID, coach: coach.ID,
		handler: (&Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()}
}

// chat starts a chat with persona and a reply saying body, and returns it
// with a run token for calling tools from it.
func (e *personaChatEnv) chat(persona uuid.UUID, temporary bool, body string) (store.ChatSession, string) {
	e.t.Helper()
	ctx := context.Background()
	chat, err := e.rs.StartChat(ctx, e.me.WorkspaceID, e.me.ID, runs.ChatInput{Title: "History", PersonaID: &persona, Temporary: temporary, Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "assistant", BodyMd: body, ContextRefs: []byte("[]"), Citations: []byte("[]")}); err != nil {
		e.t.Fatal(err)
	}
	chat, err = e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: e.me.WorkspaceID})
	if err != nil || chat.CurrentRunID == nil {
		e.t.Fatalf("chat run: %v", err)
	}
	if _, err = e.q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
		e.t.Fatal(err)
	}
	tok, _, err := agents.IssueToken(ctx, e.q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return chat, tok
}

func (e *personaChatEnv) call(token, name string, args any) (int, map[string]any) {
	e.t.Helper()
	raw, _ := json.Marshal(args)
	req := httptest.NewRequest("POST", "/api/v1/agent/tools/"+name, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// A temporary chat's persona can't reach the person's memory.
func TestTemporaryChatHasNoMemory(t *testing.T) {
	e := newPersonaChatEnv(t)
	_, normal := e.chat(e.gwen, false, "Kokoro")
	if code, out := e.call(normal, "save_memory", map[string]any{"title": "Lives with two cats"}); code != 200 {
		t.Fatalf("save in a normal chat: %d %v", code, out)
	}
	_, tmp := e.chat(e.gwen, true, "Kokoro")
	for name, args := range map[string]map[string]any{
		"search_memory": {},
		"save_memory":   {"title": "Has a dog"},
		"save_memory ":  {"title": "Auburn hair", "about": "persona"},
	} {
		if code, out := e.call(tmp, strings.TrimSpace(name), args); code != http.StatusConflict || !strings.Contains(fmt.Sprint(out["message"]), "temporary chat") {
			t.Errorf("%s in a temporary chat: %d %v", name, code, out)
		}
	}
}

// The sweep deletes a temporary chat once it has sat idle past the cutoff,
// and leaves normal chats and recently active temporary ones alone.
func TestIdleTemporaryChatsAreDeleted(t *testing.T) {
	e := newPersonaChatEnv(t)
	ctx := context.Background()
	tmp, _ := e.chat(e.gwen, true, "Kokoro")
	normal, _ := e.chat(e.gwen, false, "Kokoro")
	for _, c := range []store.ChatSession{tmp, normal} {
		if _, err := e.rs.CancelChat(ctx, e.me.WorkspaceID, e.me.ID, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	ws := e.me.WorkspaceID

	past := time.Now().Add(-time.Hour)
	if gone, err := e.rs.DeleteIdleTemporaryChat(ctx, ws, tmp.ID, past); err != nil || gone {
		t.Fatalf("recently active temporary chat deleted: %v %v", gone, err)
	}
	soon := time.Now().Add(time.Hour)
	expired, err := e.q.ExpiredTemporaryChats(ctx, store.ExpiredTemporaryChatsParams{Cutoff: &soon, RowLimit: 10})
	if err != nil || len(expired) != 1 || expired[0].ID != tmp.ID {
		t.Fatalf("expired = %v %v, want only the temporary chat", expired, err)
	}
	if gone, err := e.rs.DeleteIdleTemporaryChat(ctx, ws, normal.ID, soon); err != nil || gone {
		t.Fatalf("normal chat deleted: %v %v", gone, err)
	}
	if gone, err := e.rs.DeleteIdleTemporaryChat(ctx, ws, tmp.ID, soon); err != nil || !gone {
		t.Fatalf("idle temporary chat kept: %v %v", gone, err)
	}
	if _, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: tmp.ID, WorkspaceID: ws}); err == nil {
		t.Fatal("temporary chat still there")
	}
}

// The API starts a temporary chat, says so, and keeps it out of the person's
// own chat search.
func TestTemporaryChatAPI(t *testing.T) {
	e := newChatEnv(t)
	tmp := e.newChat(map[string]any{"temporary": true})
	if tmp["temporary"] != true {
		t.Fatalf("created %v", tmp)
	}
	normal := e.newChat(nil)
	if _, ok := normal["temporary"]; ok {
		t.Fatalf("normal chat says temporary: %v", normal)
	}
	e.addReply(t, tmp["id"].(string), "Kokoro in a temporary chat.")
	e.addReply(t, normal["id"].(string), "Kokoro in a normal chat.")
	hits := e.want("GET", "/chat-search?q=kokoro", nil, 200)["hits"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["chat_id"] != normal["id"] {
		t.Fatalf("hits %v, want only the normal chat", hits)
	}
}
