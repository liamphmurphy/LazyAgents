package runner_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/designs"
)

// A ticket run receives the design frames linked to its ticket: each frame's
// PNG and its Inspect output (React) as context files. A frame the renderer
// refuses still gets its markdown, with a note, and the run goes on.
func TestTicketRunReceivesLinkedDesignFrames(t *testing.T) {
	e := setup(t)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	var mu sync.Mutex
	var calls []string
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		calls = append(calls, r.URL.Path+" "+in["node"].(string))
		mu.Unlock()
		switch {
		case r.Header.Get("Authorization") != "Bearer t":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/render" && in["node"] == "n_cart":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"node: n_cart has no size"}`))
		case r.URL.Path == "/render":
			w.Header().Set("X-Design-Width", "393")
			w.Header().Set("X-Design-Height", "852")
			_, _ = w.Write(png)
		case r.URL.Path == "/inspect":
			_ = json.NewEncoder(w).Encode(map[string]any{"node": map[string]any{"id": in["node"]}, "mode": "dark", "box": map[string]any{"w": 393, "h": 852},
				"code": "export function Payment() {\n  return <div className=\"bg-panel\" />;\n}\n", "tokens": []any{map[string]any{"name": "bg-panel", "value": "#16151F", "used_for": []any{"Payment fill"}}},
				"type": []any{}, "assets": []any{}})
		}
	}))
	t.Cleanup(worker.Close)
	svc := &designs.Service{Pool: e.pool, Runs: e.runs, Worker: &designs.Worker{URL: worker.URL, Token: "t"}}
	e.runner.Designs = svc

	cv, chat, err := svc.Create(e.ctx, e.ws, e.me.ID, "Checkout flow", "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../docs/design/fixtures/acceptance-design.json")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(string(raw), "LAZYAGENTS-150", "PAY-214")
	cur, _ := e.q.LatestChatCanvasVersion(e.ctx, cv.ID)
	if _, err := (&canvas.Service{Pool: e.pool}).Save(e.ctx, chat.ID, cv.ID, cur.Seq, body, "Send to ticket", canvas.Actor{Type: "user", ID: e.me.ID}); err != nil {
		t.Fatal(err)
	}

	e.dispatch("PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	ws := filepath.Join(e.dir, "work", "runs", e.run("PAY-214").ID.String(), "workspace")
	dir := filepath.Join(ws, ".lazyagents/context/designs")
	got, err := os.ReadFile(filepath.Join(dir, "checkout-flow-payment.png"))
	if err != nil || string(got) != string(png) {
		t.Fatalf("payment png: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(dir, "checkout-flow-payment.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Design frame: Payment", "node n_pay", "`.lazyagents/context/designs/checkout-flow-payment.png` (393×852 at 1×)", "- `web/app/checkout/pay.tsx`", "| `--bg-panel` | #16151F | Payment fill |", "```tsx\nexport function Payment()"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("payment md lacks %q:\n%s", want, md)
		}
	}
	cart, err := os.ReadFile(filepath.Join(dir, "checkout-flow-cart.md"))
	if err != nil || !strings.Contains(string(cart), "couldn't be drawn (node: n_cart has no size)") || !strings.Contains(string(cart), "```tsx") {
		t.Fatalf("cart md: %v\n%s", err, cart)
	}
	if _, err := os.Stat(filepath.Join(dir, "checkout-flow-cart.png")); !os.IsNotExist(err) {
		t.Fatalf("a failed render left a png: %v", err)
	}
	agentsMD, _ := os.ReadFile(filepath.Join(ws, "AGENTS.md"))
	if !strings.Contains(string(agentsMD), "designs/checkout-flow-payment.md") {
		t.Errorf("AGENTS.md doesn't list the frame:\n%s", agentsMD)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(calls, ","), "/inspect n_pay") {
		t.Fatalf("worker calls: %v", calls)
	}
}
