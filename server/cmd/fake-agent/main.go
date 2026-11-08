// Command fake-agent plays an agent over the real Agent API (MCP), so the
// whole loop can be demoed and tested without an LLM or API keys:
//
//	LAZYAGENTS_TOKEN=la_… go run ./cmd/fake-agent -url http://localhost:8000/mcp -ticket PAY-214
//
// It claims the ticket, reads its context, posts a plan, runs "tests",
// requests db:migrate approval and waits for a person to approve it in the
// UI, then runs the migration through the broker and completes the run.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func main() {
	url := flag.String("url", "http://localhost:8000/mcp", "Vesper MCP endpoint")
	ticket := flag.String("ticket", "PAY-214", "ticket to work on (must be assigned to the token's agent)")
	sql := flag.String("sql", "CREATE TABLE IF NOT EXISTS ledger_entries_demo (id bigint, posted_at date) PARTITION BY RANGE (posted_at);",
		"SQL to request approval for (db:migrate)")
	pace := flag.Duration("pace", 700*time.Millisecond, "pause between steps so people can watch")
	flag.Parse()
	token := os.Getenv("LAZYAGENTS_TOKEN")
	if token == "" {
		log.Fatal("set LAZYAGENTS_TOKEN (mint one on the agent's Tokens tab)")
	}

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "fake-agent", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: *url, HTTPClient: &http.Client{Transport: bearer{token}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	call := func(name string, args map[string]any) map[string]any {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			log.Fatalf("%s: %s", name, res.Content[0].(*mcp.TextContent).Text)
		}
		out := map[string]any{}
		raw, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(raw, &out)
		time.Sleep(*pace)
		return out
	}
	say := func(format string, a ...any) { fmt.Printf("→ "+format+"\n", a...) }

	who := call("whoami", nil)
	say("I'm %s (@%s), %s session", who["name"], who["handle"], who["session"])
	claimed := call("claim_ticket", map[string]any{"ticket": *ticket})
	say("claimed %s", *ticket)
	// Wiki memory delivered with the claim (docs/dev/plan.md §12.3).
	if mem, _ := claimed["memory"].([]any); len(mem) > 0 {
		for _, m := range mem {
			page := m.(map[string]any)
			say("memory: %s (%s)", page["ref"], page["reason"])
		}
	}
	tk := call("get_ticket", map[string]any{"ticket": *ticket})
	say("%s: %s", *ticket, tk["title"])
	for _, a := range tk["artifacts"].([]any) {
		doc := a.(map[string]any)
		if doc["relation"] != "context" {
			continue
		}
		r := call("read_artifact", map[string]any{"artifact": doc["id"]})
		say("read %s %q (%d sections)", strings.ToUpper(doc["type"].(string)), r["title"], len(r["outline"].([]any)))
	}

	steps := []string{
		"Create partitioned parent table and monthly partitions",
		"Add dual-write trigger on ledger_entries",
		"Backfill in 50k-row batches with checksum verification",
		"Apply migration to staging (requires approval)",
		"Open a PR and hand over for review",
	}
	call("post_plan", map[string]any{"steps": steps})
	say("posted a %d-step plan", len(steps))
	call("set_branch", map[string]any{"branch": "atlas/" + strings.ToLower(*ticket) + "-partition-ledger"})
	for i := 1; i <= 3; i++ {
		call("update_step", map[string]any{"step": i, "state": "current"})
		call("update_step", map[string]any{"step": i, "state": "done"})
		say("step %d done", i)
	}
	call("log_event", map[string]any{"kind": "test_run", "payload": map[string]any{"passed": 312, "failed": 0, "duration_s": 252, "command": "make test"}})
	say("tests: 312 passed")
	call("report_usage", map[string]any{"tool_calls": 186, "input_tokens": 1800000, "output_tokens": 300000, "cost_usd": 4.18})

	call("update_step", map[string]any{"step": 4, "state": "current"})
	ap := call("request_approval", map[string]any{
		"scope": "db:migrate", "action": "Apply 0042_partition_ledger_entries to staging",
		"detail": map[string]any{"sql": *sql}, "risk": "staging · 18.2M rows · est. 4 min, no table lock",
	})
	if ap["state"] == "pending" {
		say("waiting for a person to approve db:migrate in Vesper…")
		for {
			w := call("wait_for_approval", map[string]any{"approval_id": ap["approval_id"], "timeout_s": 50})
			if w["state"] == "pending" {
				continue
			}
			if w["state"] != "approved" {
				say("approval %s (%v); handing back to a person", w["state"], w["note"])
				call("comment", map[string]any{"body": "The migration wasn't approved, so I've stopped before touching staging."})
				call("complete_run", map[string]any{"outcome": "Stopped: db:migrate was denied", "status": "todo"})
				return
			}
			say("approved%s", noteOf(w))
			break
		}
		res := call("run_gated", map[string]any{"approval_id": ap["approval_id"]})
		say("broker: %v (%v ms)", res["summary"], res["duration_ms"])
		if res["ok"] != true {
			say("migration failed: %v", res["error"])
			call("update_step", map[string]any{"step": 4, "state": "failed"})
			call("await_human", map[string]any{"reason": "The staging migration failed; see the timeline"})
			return
		}
	}
	call("update_step", map[string]any{"step": 4, "state": "done"})
	call("create_artifact", map[string]any{"title": "Staging migration report — " + *ticket,
		"body": "# Staging migration report\n\n## Result\n\nApplied through the db:migrate broker.\n\n## Next\n\nPR ready for review.\n"})
	call("check_criterion", map[string]any{"n": 4})
	// Remember what was learned, linked to the ticket and the ADR, so the
	// next run on a related ticket starts with it.
	call("write_note", map[string]any{"path": "agents/" + who["handle"].(string) + "/Partition rollback gotchas", "mode": "append",
		"body": "- " + *ticket + ": applied the partitioning from [[ADR-012]] to staging through the broker. Keep the legacy index until the rollback window closes. See [[" + *ticket + "]].\n"})
	say("wrote a memory note")
	call("update_step", map[string]any{"step": 5, "state": "done"})
	call("complete_run", map[string]any{"outcome": "Applied the migration to staging; ready for review"})
	say("done: %s is In Review", *ticket)
}

func noteOf(w map[string]any) string {
	if n, _ := w["note"].(string); n != "" {
		return ": “" + n + "”"
	}
	return ""
}
