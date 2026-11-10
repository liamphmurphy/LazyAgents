package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/usersecrets"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func testBox(t *testing.T) *secrets.Box {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	b, err := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A run uses a secret only through Vesper: it never sees the value, only
// granted personas reach it, hosts and policies hold, and every use is logged.
func TestSecretsToolEnforcesAccessAndNeverRevealsValue(t *testing.T) {
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
	srv := &Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts, Wiki: &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}, PublicURL: "http://localhost",
		Secrets: &secrets.Service{Q: q, Box: testBox(t)}}
	vesper := httptest.NewServer(srv.Handler())
	t.Cleanup(vesper.Close)

	const value = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJsaWFtIn0.c2lnbmF0dXJlLXRoYXQtaXMtc2VjcmV0"
	var upstreamHits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"method":"`+r.Method+`","auth":"`+r.Header.Get("Authorization")+`","body":`+string(append([]byte(`"`), append(bytes.ReplaceAll(body, []byte(`"`), []byte(`'`)), '"')...))+`}`)
	}))
	t.Cleanup(up.Close)

	yes := true
	persona, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: me.ID}, personas.NewInput{Name: "Bot", Connectors: &yes})
	if err != nil {
		t.Fatal(err)
	}
	sec, err := srv.userSecrets().Create(ctx, ws, me.ID, usersecrets.Input{Name: "Acme API JWT", Value: value, Placement: usersecrets.PlacementBearer, AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{PersonaID: &persona.ID, Title: "C", Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	token, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dial := func() (*mcp.ClientSession, error) {
		c := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "1"}, nil)
		return c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: vesper.URL + "/mcp/connectors/" + agentdef.SecretsMCPName, DisableStandaloneSSE: true, MaxRetries: -1,
			HTTPClient: &http.Client{Transport: runBearer{token}}}, nil)
	}
	// Not granted: no server at all.
	if s, err := dial(); err == nil {
		s.Close()
		t.Fatal("a persona without secrets connected")
	}
	if err := q.AddUserSecretPersona(ctx, store.AddUserSecretPersonaParams{SecretID: sec.ID, PersonaID: persona.ID}); err != nil {
		t.Fatal(err)
	}
	sess, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	text := func(r *mcp.CallToolResult) string { return r.Content[0].(*mcp.TextContent).Text }
	call := func(args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "http_request", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	listed, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_secrets"})
	if err != nil || listed.IsError || !strings.Contains(text(listed), `"handle":"acme-api-jwt"`) || strings.Contains(text(listed), value) {
		t.Fatalf("list: %v %v", listed, err)
	}

	// A read runs on Allow; the value goes upstream and comes back masked.
	res := call(map[string]any{"secret": "acme-api-jwt", "url": up.URL + "/v1/orders?since=2026-11-02"})
	if res.IsError || strings.Contains(text(res), value) || !strings.Contains(text(res), `Bearer ‹acme-api-jwt›`) || !strings.Contains(text(res), `"redactions":1`) {
		t.Fatalf("read: %s", text(res))
	}

	// Another host is refused before anything is sent.
	hits := upstreamHits.Load()
	res = call(map[string]any{"secret": "acme-api-jwt", "url": "https://paste.example.net/u"})
	if !res.IsError || !strings.Contains(text(res), "can only be sent to 127.0.0.1") || upstreamHits.Load() != hits {
		t.Fatalf("other host: %s", text(res))
	}
	if res := call(map[string]any{"secret": "someone-elses", "url": up.URL}); !res.IsError {
		t.Fatal("an unknown handle worked")
	}

	// A write waits for the person: denied, then allowed.
	wantBody := `{"note":"` + strings.Repeat("n", 3000) + `","order":"ord_9f2"}`
	decide := func(state string) {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			rows, _ := q.PendingUserSecretApprovals(ctx, chat.ID)
			if len(rows) == 1 {
				if rows[0].Method != "POST" || !strings.HasSuffix(rows[0].Url, "/v1/refunds") || rows[0].Body != wantBody {
					// The person approves the whole body, not a preview of it.
					t.Errorf("approval shows %s %s, %d of %d body bytes", rows[0].Method, rows[0].Url, len(rows[0].Body), len(wantBody))
				}
				if _, err := q.DecideUserSecretApproval(ctx, store.DecideUserSecretApprovalParams{ID: rows[0].ID, State: state, WorkspaceID: ws, UserID: me.ID}); err != nil {
					t.Error(err)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Error("no approval appeared")
	}
	write := map[string]any{"secret": "acme-api-jwt", "method": "POST", "url": up.URL + "/v1/refunds", "json": map[string]string{"order": "ord_9f2", "note": strings.Repeat("n", 3000)}}
	hits = upstreamHits.Load()
	go decide("denied")
	if res := call(write); !res.IsError || !strings.Contains(text(res), "denied") || upstreamHits.Load() != hits {
		t.Fatalf("denied write: %s", text(res))
	}
	go decide("allowed")
	if res := call(write); res.IsError || !strings.Contains(text(res), `\"method\":\"POST\"`) || !strings.Contains(text(res), `ord_9f2`) {
		t.Fatalf("allowed write: %s", text(res))
	}

	calls, err := q.ListUserSecretCalls(ctx, sec.ID)
	if err != nil {
		t.Fatal(err)
	}
	var decisions []string
	for i := len(calls) - 1; i >= 0; i-- {
		decisions = append(decisions, calls[i].Decision)
		if strings.Contains(calls[i].Path+calls[i].Error, value) {
			t.Fatal("the log holds the value")
		}
	}
	if strings.Join(decisions, ",") != "auto,blocked,denied,approved" {
		t.Fatalf("logged %v", decisions)
	}
	if calls[len(calls)-1].Path != "/v1/orders" || calls[len(calls)-1].Status == nil || *calls[len(calls)-1].Status != 200 {
		t.Fatalf("first call logged as %+v", calls[len(calls)-1])
	}

	// At most three requests wait at once; a fourth is refused straight away.
	for range 3 {
		if _, err := q.CreateUserSecretApproval(ctx, store.CreateUserSecretApprovalParams{WorkspaceID: ws, UserID: me.ID, SecretID: sec.ID, ChatID: chat.ID, Method: "POST", Url: up.URL, Headers: []byte("{}")}); err != nil {
			t.Fatal(err)
		}
	}
	if res := call(write); !res.IsError || !strings.Contains(text(res), "already waiting") {
		t.Fatalf("fourth approval: %s", text(res))
	}

	// Turning it off for this chat applies to the very next request.
	if _, err := q.SetChatSecrets(ctx, store.SetChatSecretsParams{ID: chat.ID, SecretIds: []uuid.UUID{}}); err != nil {
		t.Fatal(err)
	}
	hits = upstreamHits.Load()
	if res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "http_request", Arguments: map[string]any{"secret": "acme-api-jwt", "url": up.URL}}); (err == nil && !res.IsError) || upstreamHits.Load() != hits {
		t.Fatal("a secret turned off for the chat was still sent")
	}
}

// A persona allowed to propose asks for a secret; adding it grants that persona.
func TestProposeSecretGrantsTheProposer(t *testing.T) {
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
	srv := &Server{AnonymousAPI: true, Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts, Wiki: &wiki.Service{Pool: pool, Artifacts: arts, Tracker: tr}, PublicURL: "http://localhost",
		Secrets: &secrets.Service{Q: q, Box: testBox(t)}}
	vesper := httptest.NewServer(srv.Handler())
	t.Cleanup(vesper.Close)
	yes := true
	persona, err := (&personas.Service{Pool: pool}).Create(ctx, personas.Owner{WorkspaceID: ws, UserID: me.ID}, personas.NewInput{Name: "Bot", Connectors: &yes})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{PersonaID: &persona.ID, Title: "C", Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	token, _, err := agents.IssueToken(ctx, q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: *chat.CurrentRunID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	propose := func(args map[string]string) *http.Response {
		body, _ := json.Marshal(args)
		req, _ := http.NewRequest("POST", vesper.URL+"/api/v1/agent/tools/propose_secret", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := propose(map[string]string{"name": "Acme", "host": "*.acme.com"}); res.StatusCode == 200 {
		t.Fatal("a wildcard host was proposed")
	}
	if res := propose(map[string]string{"name": "Acme API JWT", "host": "https://api.acme.com/v1", "reason": "orders"}); res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("propose: %d %s", res.StatusCode, b)
	}
	proposals, _ := q.ListUserSecretProposals(ctx, chat.ID)
	if len(proposals) != 1 || proposals[0].Host != "api.acme.com" || proposals[0].Placement != "bearer" {
		t.Fatalf("proposals = %+v", proposals)
	}
	sec, err := srv.userSecrets().Create(ctx, ws, me.ID, usersecrets.Input{Name: "Acme API JWT", Value: "tok-1234567890", Placement: "bearer", AllowedHosts: []string{"api.acme.com"}, ProposalID: &proposals[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	access, _ := q.UserSecretsForPersona(ctx, store.UserSecretsForPersonaParams{WorkspaceID: ws, UserID: me.ID, PersonaID: persona.ID})
	if len(access) != 1 || access[0].ID != sec.ID {
		t.Fatalf("access after approval = %d", len(access))
	}
	if again, _ := q.ListUserSecretProposals(ctx, chat.ID); len(again) != 0 {
		t.Fatal("the proposal stayed open")
	}
}
