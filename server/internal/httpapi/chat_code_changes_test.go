package httpapi

import (
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A coding turn's file changes are stored on the assistant message and
// rendered as code_changes, left out when there are none.
func TestChatMessageCodeChanges(t *testing.T) {
	e, _ := newAvatarEnv(t)
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	ctx := t.Context()
	app := func(role string) store.AppendChatMessageRow {
		m, err := e.q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chatID, Role: role, BodyMd: "x", ContextRefs: []byte("[]"), Citations: []byte("[]")})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	app("user")
	changed, plain := app("assistant"), app("assistant")
	raw := `[{"path":"a b.go","old_path":"c.go","status":"renamed","additions":2,"deletions":1,"binary":false,"patch":"@@ -1 +1,2 @@\n-x\n+y\n+z\n","truncated":false},
	  {"path":"big.bin","status":"added","additions":0,"deletions":0,"binary":true,"truncated":false},
	  {"path":"huge.txt","status":"modified","additions":9000,"deletions":0,"binary":false,"truncated":true}]`
	if err := e.q.SetChatMessageCodeChanges(ctx, store.SetChatMessageCodeChangesParams{ID: changed.ID, CodeChanges: []byte(raw)}); err != nil {
		t.Fatal(err)
	}
	list := e.want("GET", "/chats/"+chatID.String()+"/messages", nil, 200)["messages"].([]any)
	if len(list) != 3 {
		t.Fatalf("messages %v", list)
	}
	if _, ok := list[0].(map[string]any)["code_changes"]; ok {
		t.Errorf("a user message has no code_changes: %v", list[0])
	}
	if list[2].(map[string]any)["id"] != plain.ID.String() {
		t.Fatalf("order: %v", list)
	}
	if _, ok := list[2].(map[string]any)["code_changes"]; ok {
		t.Errorf("an unchanged turn must omit code_changes: %v", list[2])
	}
	got, _ := list[1].(map[string]any)["code_changes"].([]any)
	if len(got) != 3 {
		t.Fatalf("code_changes %v", list[1])
	}
	first := got[0].(map[string]any)
	if first["path"] != "a b.go" || first["old_path"] != "c.go" || first["status"] != "renamed" || first["additions"] != float64(2) || first["deletions"] != float64(1) ||
		first["patch"] != "@@ -1 +1,2 @@\n-x\n+y\n+z\n" || first["truncated"] != false || first["binary"] != false {
		t.Errorf("first change %v", first)
	}
	if bin := got[1].(map[string]any); bin["binary"] != true || bin["patch"] != nil {
		t.Errorf("binary change %v", bin)
	}
	if big := got[2].(map[string]any); big["truncated"] != true || big["patch"] != nil {
		t.Errorf("truncated change %v", big)
	}
}
