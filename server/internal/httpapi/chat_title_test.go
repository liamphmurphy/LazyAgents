package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func TestSetChatTitleTool(t *testing.T) {
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
	handler := (&Server{Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	call := func(token string, args any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/set_chat_title", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rs.PostChatMessage(ctx, ws, me.ID, chat.ID, runs.ChatMessageInput{Body: "help me plan refunds"}); err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	assistant, err := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, q, assistant.ID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	title := func() string {
		c, _ := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
		return c.Title
	}

	if code, out := call(token, map[string]any{"title": strings.Repeat("x", 81)}); code != 422 {
		t.Fatalf("too long = %d %v", code, out)
	}
	if code, out := call(token, map[string]any{"title": "  \"\"  "}); code != 422 {
		t.Fatalf("empty = %d %v", code, out)
	}
	code, out := call(token, map[string]any{"title": "  \"Plan refund\n flow.\" "})
	if code != 200 || out["applied"] != true || out["title"] != "Plan refund flow" || title() != "Plan refund flow" {
		t.Fatalf("auto title = %d %v (%q)", code, out, title())
	}
	// A second call while the title is still not the person's is allowed only for the auto title; now it is the agent's own, so the person's rename must win.
	if _, err = pool.Exec(ctx, "UPDATE chat_sessions SET state='idle' WHERE id=$1", chat.ID); err != nil {
		t.Fatal(err)
	}
	cur, _ := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	renamed := "My own name"
	if _, err = rs.UpdateChat(ctx, ws, me.ID, chat.ID, runs.ChatPatchInput{Revision: cur.Revision, Title: &renamed}); err != nil {
		t.Fatal(err)
	}
	code, out = call(token, map[string]any{"title": "Agent rename"})
	if code != 200 || out["applied"] != false || out["title"] != renamed || title() != renamed {
		t.Fatalf("after rename = %d %v (%q)", code, out, title())
	}

	// A caller outside a chat is refused.
	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	other, _, err := agents.IssueToken(ctx, q, atlas.ID, "unmanaged", "title", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, out := call(other, map[string]any{"title": "Nope"}); code != 409 || title() != renamed {
		t.Fatalf("non-chat = %d %v", code, out)
	}
}
