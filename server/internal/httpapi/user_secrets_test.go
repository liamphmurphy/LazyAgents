package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// The API never returns a value, and its settings behave as the UI expects.
func TestApiSecretsAPI(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts}
	h := (&Server{Pool: pool, Q: q, Runs: rs, Tracker: tr, Artifacts: arts, Secrets: &secrets.Service{Q: q, Box: testBox(t)}}).Handler()
	do := func(method, path, body string, want int) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	persona, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: me.ID}, personas.NewInput{Name: "Bot"})
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	exp := time.Now().Add(12 * time.Hour).Unix()
	value := enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(`{"exp":`+strconv.FormatInt(exp, 10)+`}`)) + ".c2lnbmF0dXJlLXRoYXQtaXMtc2VjcmV0LVhZWlc"

	created := do("POST", "/api-secrets", `{"name":"Acme API JWT","value":"`+value+`","placement":"bearer","allowed_hosts":["api.acme.com"],"persona_ids":["`+persona.ID.String()+`"]}`, 200)
	var sec struct {
		ID         string    `json:"id"`
		Handle     string    `json:"handle"`
		Revision   int       `json:"revision"`
		PersonaIDs []string  `json:"persona_ids"`
		ExpiresAt  time.Time `json:"expires_at"`
		Write      string    `json:"write_policy"`
	}
	_ = json.Unmarshal([]byte(created), &sec)
	if sec.Handle != "acme-api-jwt" || len(sec.PersonaIDs) != 1 || sec.ExpiresAt.Unix() != exp || sec.Write != "ask" {
		t.Fatalf("created = %s", created)
	}
	list := do("GET", "/api-secrets", "", 200)
	if !strings.Contains(list, `"key_configured":true`) {
		t.Fatalf("list = %s", list)
	}
	for _, body := range []string{created, list} {
		if strings.Contains(body, "LVhZWlc") || strings.Contains(body, `"value"`) || strings.Contains(body, "eyJ") {
			t.Fatalf("response leaks the value: %s", body)
		}
	}

	// Changing how it's sent needs the value again; a stale revision is refused.
	do("PATCH", "/api-secrets/"+sec.ID, `{"revision":`+strconv.Itoa(sec.Revision)+`,"placement":"basic"}`, 422)
	patched := do("PATCH", "/api-secrets/"+sec.ID, `{"revision":`+strconv.Itoa(sec.Revision)+`,"allowed_hosts":["api.acme.com","https://staging.acme.com/x"],"read_policy":"ask"}`, 200)
	if !strings.Contains(patched, `"allowed_hosts":["api.acme.com","staging.acme.com"]`) || !strings.Contains(patched, `"read_policy":"ask"`) {
		t.Fatalf("patched = %s", patched)
	}
	do("PATCH", "/api-secrets/"+sec.ID, `{"revision":`+strconv.Itoa(sec.Revision)+`,"name":"stale"}`, 409)

	// It expires within a day, so the inbox says so.
	inbox := do("GET", "/inbox", "", 200)
	if !strings.Contains(inbox, `"kind":"secret_expiring"`) || !strings.Contains(inbox, "Expires within a day") {
		t.Fatalf("inbox = %s", inbox)
	}

	// Approvals: "always" only for reads, and it sets reads to Allow.
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{PersonaID: &persona.ID, Title: "C", Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	row, _ := q.GetUserSecret(ctx, store.GetUserSecretParams{ID: uuid.MustParse(sec.ID), WorkspaceID: ws, UserID: me.ID})
	ask := func(method string) string {
		a, err := q.CreateUserSecretApproval(ctx, store.CreateUserSecretApprovalParams{WorkspaceID: ws, UserID: me.ID, SecretID: row.ID, PersonaID: &persona.ID, ChatID: chat.ID,
			Method: method, Url: "https://api.acme.com/v1/x", Headers: []byte(`{"Accept":"application/json"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return a.ID.String()
	}
	write := ask("POST")
	pending := do("GET", "/chats/"+chat.ID.String()+"/api-secret-approvals", "", 200)
	if !strings.Contains(pending, `"placement":"bearer"`) || !strings.Contains(pending, `"secret_handle":"acme-api-jwt"`) || !strings.Contains(pending, `"Accept":"application/json"`) {
		t.Fatalf("pending = %s", pending)
	}
	if inbox := do("GET", "/inbox", "", 200); !strings.Contains(inbox, "Wants to send POST to api.acme.com") {
		t.Fatalf("inbox = %s", inbox)
	}
	do("POST", "/api-secret-approvals/"+write, `{"decision":"always"}`, 422)
	do("POST", "/api-secret-approvals/"+write, `{"decision":"deny"}`, 204)
	do("POST", "/api-secret-approvals/"+write, `{"decision":"allow"}`, 409)
	read := ask("GET")
	do("POST", "/api-secret-approvals/"+read, `{"decision":"always"}`, 204)
	if row, _ = q.GetUserSecret(ctx, store.GetUserSecretParams{ID: row.ID, WorkspaceID: ws, UserID: me.ID}); row.ReadPolicy != "allow" || row.WritePolicy != "ask" {
		t.Fatalf("after always: reads %s, writes %s", row.ReadPolicy, row.WritePolicy)
	}

	// Turning it off for the chat narrows only this chat.
	if err := q.LogUserSecretCall(ctx, store.LogUserSecretCallParams{SecretID: row.ID, ChatID: &chat.ID, PersonaID: &persona.ID, Method: "GET", Host: "api.acme.com", Path: "/v1/x", Decision: "auto"}); err != nil {
		t.Fatal(err)
	}
	used := do("GET", "/chats/"+chat.ID.String()+"/api-secrets", "", 200)
	if !strings.Contains(used, `"calls":1`) || !strings.Contains(used, `"enabled":true`) {
		t.Fatalf("used = %s", used)
	}
	do("POST", "/chats/"+chat.ID.String()+"/api-secrets/"+sec.ID+"/disable", "", 204)
	if used = do("GET", "/chats/"+chat.ID.String()+"/api-secrets", "", 200); !strings.Contains(used, `"enabled":false`) {
		t.Fatalf("after disable = %s", used)
	}
	if access, _ := q.UserSecretsForPersona(ctx, store.UserSecretsForPersonaParams{WorkspaceID: ws, UserID: me.ID, PersonaID: persona.ID}); len(access) != 1 {
		t.Fatal("turning it off for one chat removed the persona's access")
	}
	calls := do("GET", "/api-secrets/"+sec.ID+"/calls", "", 200)
	if !strings.Contains(calls, `"decision":"auto"`) || !strings.Contains(calls, `"persona_name":"Bot"`) {
		t.Fatalf("calls = %s", calls)
	}

	do("DELETE", "/api-secrets/"+sec.ID, "", 204)
	do("DELETE", "/api-secrets/"+sec.ID, "", 404)
}
