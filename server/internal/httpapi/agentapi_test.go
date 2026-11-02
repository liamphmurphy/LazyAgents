package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// TestAgentAPIOverMCP drives PAY-214 the way an unmanaged Claude Code
// session would: through a real MCP client against the full HTTP handler.
func TestAgentAPIOverMCP(t *testing.T) {
	pool := testdb.New(t)
	staging := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	sec := &secrets.Service{Q: q, Box: box}
	if _, err := sec.Set(ctx, ws, me.ID, secrets.SetInput{Name: "db:staging", Value: staging.Config().ConnString()}); err != nil {
		t.Fatal(err)
	}
	tr := &tracker.Service{Pool: pool}
	arts := &artifacts.Service{Pool: pool}
	srv := &Server{
		Version: "test", Pool: pool, Q: q, Tracker: tr, Artifacts: arts, Secrets: sec,
		Agents: &agents.Service{Pool: pool}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Runs: &runs.Service{Pool: pool, Secrets: sec, Tracker: tr, Artifacts: arts, Poll: 20 * time.Millisecond},
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	atlas, _ := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	tok, _, _ := agents.IssueToken(ctx, q, atlas.ID, "unmanaged", "test", nil, nil, nil)

	// No token → 401, before any MCP handshake.
	if res, _ := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(`{}`)); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", res.StatusCode)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{tok, http.DefaultTransport}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	tools, err := sess.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) < 20 {
		t.Fatalf("tools: %d %v", len(tools.Tools), err)
	}

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s returned error: %v", name, res.Content[0].(*mcp.TextContent).Text)
		}
		out := map[string]any{}
		raw, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(raw, &out)
		return out
	}
	callErr := func(name string, args map[string]any) string {
		t.Helper()
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("%s should have failed", name)
		}
		return res.Content[0].(*mcp.TextContent).Text
	}

	who := call("whoami", nil)
	if who["handle"] != "atlas" || who["session"] != "unmanaged" {
		t.Fatalf("whoami = %v", who)
	}
	if msg := callErr("claim_ticket", map[string]any{"ticket": "PAY-203"}); !strings.Contains(msg, "isn't assigned") {
		t.Fatalf("claiming someone else's ticket: %s", msg)
	}
	call("claim_ticket", map[string]any{"ticket": "PAY-214"})
	tk := call("get_ticket", map[string]any{"ticket": "PAY-214"})
	docs := tk["artifacts"].([]any)
	if len(docs) != 4 || len(tk["criteria"].([]any)) != 5 {
		t.Fatalf("get_ticket: %d docs %d criteria", len(docs), len(tk["criteria"].([]any)))
	}
	prdID := docs[0].(map[string]any)["id"].(string)
	sec4 := call("read_artifact", map[string]any{"artifact": prdID, "section": "reconciliation-performance"})
	if !strings.HasPrefix(sec4["markdown"].(string), "## Reconciliation performance") {
		t.Fatalf("section read: %q", sec4["markdown"])
	}
	if adr := call("read_artifact", map[string]any{"artifact": "ADR-012"}); adr["title"] != "Postgres declarative partitioning" {
		t.Fatalf("ADR by number: %v", adr["title"])
	}
	call("post_plan", map[string]any{"steps": []string{"Create partitions", "Dual-write", "Apply to staging"}})
	call("update_step", map[string]any{"step": 3, "state": "current"})

	ap := call("request_approval", map[string]any{
		"scope": "db:migrate", "action": "Apply 0042_partition_ledger_entries to staging",
		"detail": map[string]any{"sql": "CREATE TABLE ledger_part_test (id int);"}, "risk": "No table lock",
	})
	if ap["state"] != "pending" {
		t.Fatalf("approval = %v", ap)
	}
	// While waiting, the agent can't keep working.
	if msg := callErr("set_branch", map[string]any{"branch": "atlas/x"}); !strings.Contains(msg, "waiting") {
		t.Fatalf("work while waiting: %s", msg)
	}

	// A person approves over the human REST API.
	go func() {
		time.Sleep(100 * time.Millisecond)
		body := bytes.NewBufferString(`{"note":"Ship it"}`)
		res, err := http.Post(ts.URL+"/api/v1/approvals/"+ap["approval_id"].(string)+"/approve", "application/json", body)
		if err == nil {
			res.Body.Close()
		}
	}()
	if w := call("wait_for_approval", map[string]any{"approval_id": ap["approval_id"], "timeout_s": 10}); w["state"] != "approved" {
		t.Fatalf("wait = %v", w)
	}
	gated := call("run_gated", map[string]any{"approval_id": ap["approval_id"]})
	if gated["ok"] != true {
		t.Fatalf("run_gated = %v", gated)
	}
	if _, err := staging.Exec(ctx, "SELECT * FROM ledger_part_test"); err != nil {
		t.Fatalf("staging table missing: %v", err)
	}
	call("log_event", map[string]any{"kind": "test_run", "payload": map[string]any{"passed": 312, "failed": 0, "duration_s": 252}})
	call("report_usage", map[string]any{"tool_calls": 186, "input_tokens": 1800000, "output_tokens": 300000, "cost_usd": 4.18})
	call("complete_run", map[string]any{"outcome": "Applied to staging"})

	// The human view reflects it all.
	res, err := http.Get(ts.URL + "/api/v1/tickets/PAY-214")
	if err != nil {
		t.Fatal(err)
	}
	var ticket struct {
		Status string `json:"status"`
		Run    struct {
			State    string  `json:"state"`
			CostUSD  float64 `json:"cost_usd"`
			Plan     []any   `json:"plan"`
			ToolCall int     `json:"tool_calls"`
		} `json:"run"`
	}
	_ = json.NewDecoder(res.Body).Decode(&ticket)
	res.Body.Close()
	if ticket.Status != "in_review" || ticket.Run.State != "completed" || ticket.Run.ToolCall != 186 || len(ticket.Run.Plan) != 3 {
		t.Fatalf("ticket after run = %+v", ticket)
	}

	// The REST mirror works with the same token; a revoked token stops at once.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/agent/tools/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusOK {
		t.Fatalf("REST mirror: %d", res.StatusCode)
	}
	tid := mustTokenID(t, q, atlas.ID)
	if err := agents.RevokeToken(ctx, q, atlas.ID, tid); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest("POST", ts.URL+"/api/v1/agent/tools/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked REST: %d", res.StatusCode)
	}
	if _, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "whoami"}); err == nil {
		t.Fatal("revoked token still works over MCP")
	}
}

func mustTokenID(t *testing.T, q *store.Queries, agentID [16]byte) [16]byte {
	t.Helper()
	toks, err := q.ListAgentTokens(context.Background(), agentID)
	if err != nil || len(toks) == 0 {
		t.Fatal("no token")
	}
	return toks[0].ID
}
