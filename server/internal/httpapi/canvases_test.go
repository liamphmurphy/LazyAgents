package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// canvasToken is a run token for a chat with a turn in progress.
func (e *chatEnv) canvasToken(chat uuid.UUID) string {
	e.t.Helper()
	ctx := e.t.Context()
	if _, err := e.runs.PostChatMessage(ctx, e.me.WorkspaceID, e.me.ID, chat, runs.ChatMessageInput{Body: "draft it"}); err != nil {
		e.t.Fatal(err)
	}
	c, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat, WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		e.t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, e.q, c.AgentID, "run", "chat", c.CurrentRunID, nil, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return token
}

func (e *chatEnv) tool(token, name string, args any) (int, map[string]any) {
	e.t.Helper()
	raw, _ := json.Marshal(args)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/agent/tools/"+name, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestTextCanvasToolsAndReadAPI(t *testing.T) {
	e := newChatEnv(t)
	chat := uuid.MustParse(e.newChat(nil)["id"].(string))
	token := e.canvasToken(chat)

	code, out := e.tool(token, "canvas_create", map[string]any{"kind": "text", "title": " Retention\n policy ", "body": "Archive after 30 days.", "note": "First draft"})
	if code != 200 || out["version"] != float64(1) {
		t.Fatalf("create = %d %v", code, out)
	}
	id := out["canvas_id"].(string)
	base := "/chats/" + chat.String() + "/canvases"

	patch := func(version int, ops ...map[string]any) (int, map[string]any) {
		return e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": version, "ops": ops, "note": "Longer window"})
	}
	if code, out = patch(1, map[string]any{"find": "30", "replace": "60"}); code != 200 || out["version"] != float64(2) {
		t.Fatalf("patch = %d %v", code, out)
	}
	if code, out = patch(1, map[string]any{"find": "60", "replace": "90"}); code != 409 || !strings.Contains(out["message"].(string), "version 2") {
		t.Fatalf("stale patch = %d %v", code, out)
	}
	if code, out = patch(2, map[string]any{"find": "missing", "replace": "x"}); code != 422 {
		t.Fatalf("bad find = %d %v", code, out)
	}
	if code, out = patch(2, map[string]any{"find": "60", "replace": "60"}); code != 422 {
		t.Fatalf("no-op = %d %v", code, out)
	}

	if code, out = e.tool(token, "canvas_read", map[string]any{"canvas_id": id, "version": 1}); code != 200 || out["body"] != "Archive after 30 days." {
		t.Fatalf("read v1 = %d %v", code, out)
	}
	if code, out = e.tool(token, "canvas_list", map[string]any{}); code != 200 || out["canvases"].([]any)[0].(map[string]any)["version"] != float64(2) {
		t.Fatalf("list = %d %v", code, out)
	}

	list := e.want("GET", base, nil, 200)["canvases"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "Retention policy" || list[0].(map[string]any)["seq"] != float64(2) {
		t.Fatalf("api list = %v", list)
	}
	cur := e.want("GET", base+"/"+id, nil, 200)
	if cur["body"] != "Archive after 60 days." || cur["viewing_seq"] != float64(2) || cur["seq"] != float64(2) {
		t.Fatalf("api current = %v", cur)
	}
	versions := cur["versions"].([]any)
	if len(versions) != 2 || versions[0].(map[string]any)["seq"] != float64(2) || versions[0].(map[string]any)["note"] != "Longer window" {
		t.Fatalf("versions = %v", versions)
	}
	old := e.want("GET", base+"/"+id+"?seq=1", nil, 200)
	if old["body"] != "Archive after 30 days." || old["viewing_seq"] != float64(1) || old["seq"] != float64(2) {
		t.Fatalf("api v1 = %v", old)
	}
	e.want("GET", base+"/"+id+"?seq="+strconv.Itoa(9), nil, 404)

	// A canvas belongs to its chat.
	other := uuid.MustParse(e.newChat(nil)["id"].(string))
	otherToken := e.canvasToken(other)
	if code, out = e.tool(otherToken, "canvas_read", map[string]any{"canvas_id": id}); code != 404 {
		t.Fatalf("other chat read = %d %v", code, out)
	}
	if code, out = e.tool(otherToken, "canvas_patch", map[string]any{"canvas_id": id, "base_version": 2, "ops": []map[string]any{{"find": "60", "replace": "1"}}}); code != 404 {
		t.Fatalf("other chat patch = %d %v", code, out)
	}
	e.want("GET", "/chats/"+other.String()+"/canvases/"+id, nil, 404)
}

func TestSVGCanvasNeedsThePersonaCapabilityAndKeepsElementIDs(t *testing.T) {
	e := newChatEnv(t)
	persona := e.dockPersona("Designer")
	chat := e.dockChat(persona)
	token := e.canvasToken(chat)
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="1"/><text>Stop</text></svg>`
	set := func(on bool) {
		if _, err := e.pool.Exec(t.Context(), `UPDATE personas SET can_create_svgs = $2 WHERE id = $1`, persona, on); err != nil {
			t.Fatal(err)
		}
	}

	set(false)
	create := map[string]any{"kind": "svg", "title": "Run screen", "body": svg}
	if code, out := e.tool(token, "canvas_create", create); code != 409 || !strings.Contains(out["message"].(string), "SVG") {
		t.Fatalf("no capability = %d %v", code, out)
	}
	set(true)
	if code, out := e.tool(token, "canvas_create", map[string]any{"kind": "svg", "title": "Bad", "body": "<svg>"}); code != 422 {
		t.Fatalf("malformed = %d %v", code, out)
	}
	code, out := e.tool(token, "canvas_create", create)
	if code != 200 {
		t.Fatalf("create = %d %v", code, out)
	}
	id := out["canvas_id"]
	read := func() string {
		_, r := e.tool(token, "canvas_read", map[string]any{"canvas_id": id})
		return r["body"].(string)
	}
	body := read()
	if strings.Count(body, "data-vid=") != 2 {
		t.Fatalf("not stamped: %s", body)
	}
	var vids []string
	for _, part := range strings.Split(body, `data-vid="`)[1:] {
		vids = append(vids, part[:strings.Index(part, `"`)])
	}

	code, out = e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": 1, "ops": []map[string]any{
		{"op": "insert", "vid": vids[0], "position": "before", "markup": `<circle r="2"/>`},
		{"op": "set_text", "vid": vids[1], "text": "Halt"},
	}})
	if code != 200 {
		t.Fatalf("patch = %d %v", code, out)
	}
	body = read()
	if !strings.Contains(body, `data-vid="`+vids[0]+`"`) || !strings.Contains(body, `data-vid="`+vids[1]+`">Halt</text>`) || strings.Count(body, "data-vid=") != 3 {
		t.Fatalf("after patch: %s", body)
	}

	if code, out = e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": 2, "ops": []map[string]any{{"op": "remove", "vid": "nope"}}}); code != 422 {
		t.Fatalf("unknown element = %d %v", code, out)
	}

	set(false)
	if code, out = e.tool(token, "canvas_patch", map[string]any{"canvas_id": id, "base_version": 2, "ops": []map[string]any{{"op": "remove", "vid": vids[0]}}}); code != 409 {
		t.Fatalf("patch without capability = %d %v", code, out)
	}
}
