package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/chatmodels"
	"github.com/liamphmurphy/lazyagents/server/internal/runner"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Opt-in: the REAL opencode 2.2.1 binary driven by the real runner (local
// backend) against a localhost STUB provider, so no paid or remote model call is
// possible. Skipped unless LAZYAGENTS_REAL_OPENCODE=1 (and a database, like the
// other runner tests). It proves the real wire through the runner: session id,
// tool events, partial usage, MCP calls into LazyAgents, resume, chat history
// and SIGINT cancellation. It does NOT prove a real provider, real tokens or
// cost.
//
// There is no production base-URL knob. The test builds a tiny launcher and
// registers it as the "opencode" binary (RUNNER_OPENCODE_BIN equivalent): it
// merges provider.anthropic.options.baseURL into the opencode.json the adapter
// wrote, points the proxy variables at a dead port (NO_PROXY keeps loopback
// direct), and execs the real binary. The runner's guards stay intact: the
// OnlyBins local backend, a PATH without any real claude/codex/opencode, the
// allowlisted opencode environment and a fake provider key.
//
//	LAZYAGENTS_REAL_OPENCODE=1 [LAZYAGENTS_REAL_OPENCODE_BIN=/path/to/opencode] \
//	  TEST_DATABASE_URL=... go test ./internal/runner -run TestRealOpenCodeAgainstStub -v

const launcherSrc = `package main

import (
	"encoding/json"
	"os"
	"syscall"
)

var real, base string

func main() {
	if len(os.Args) > 1 && os.Args[1] != "--version" {
		var cfg map[string]any
		if b, err := os.ReadFile(os.Getenv("OPENCODE_CONFIG")); err == nil && json.Unmarshal(b, &cfg) == nil {
			cfg["provider"] = map[string]any{"anthropic": map[string]any{"options": map[string]any{"baseURL": base + "/v1"}}}
			b, _ = json.MarshalIndent(cfg, "", "  ")
			_ = os.WriteFile(os.Getenv("OPENCODE_CONFIG"), b, 0o600)
		}
	}
	env := append(os.Environ(),
		"HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9",
		"http_proxy=http://127.0.0.1:9", "https_proxy=http://127.0.0.1:9",
		"NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost")
	argv := append([]string{"opencode"}, os.Args[1:]...)
	if err := syscall.Exec(real, argv, env); err != nil {
		os.Stderr.WriteString("launcher: " + err.Error() + "\n")
		os.Exit(127)
	}
}
`

type stubReq struct {
	Model string
	Key   string
	Tools []string
	Msgs  int
	Text  string // all text seen in the request (system + messages)
}

type stubProvider struct {
	*httptest.Server
	mu           sync.Mutex
	reqs         []stubReq
	slowStarted  chan struct{}
	disconnected chan struct{}
	once         sync.Once
}

var ticketKey = regexp.MustCompile(`PAY-\d+`)

func newStubProvider(t *testing.T) *stubProvider {
	s := &stubProvider{slowStarted: make(chan struct{}), disconnected: make(chan struct{})}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *stubProvider) requests() []stubReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubReq(nil), s.reqs...)
}

type sseWriter struct {
	w http.ResponseWriter
}

func (s sseWriter) ev(typ string, d map[string]any) {
	d["type"] = typ
	b, _ := json.Marshal(d)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", typ, b)
	s.w.(http.Flusher).Flush()
}

func (s *stubProvider) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/messages") {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Model    string            `json:"model"`
		System   json.RawMessage   `json:"system"`
		Messages []json.RawMessage `json:"messages"`
		Tools    []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	req := stubReq{Model: body.Model, Key: r.Header.Get("x-api-key"), Msgs: len(body.Messages)}
	for _, t := range body.Tools {
		req.Tools = append(req.Tools, t.Name)
	}
	var all strings.Builder
	all.Write(body.System)
	toolResults, userMsgs := 0, 0
	for _, m := range body.Messages {
		all.Write(m)
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		_ = json.Unmarshal(m, &msg)
		if msg.Role == "user" {
			if strings.Contains(string(msg.Content), `"tool_result"`) {
				toolResults++
			} else {
				userMsgs++
			}
		}
	}
	req.Text = all.String()
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	n := len(s.reqs)
	s.mu.Unlock()

	w.Header().Set("content-type", "text/event-stream")
	out := sseWriter{w}
	out.ev("message_start", map[string]any{"message": map[string]any{"id": fmt.Sprintf("msg_%d", n), "type": "message", "role": "assistant",
		"model": body.Model, "content": []any{}, "usage": map[string]any{"input_tokens": 1000 + n, "output_tokens": 1}}})
	text := func(txt string) {
		out.ev("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		out.ev("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": txt}})
		out.ev("content_block_stop", map[string]any{"index": 0})
		out.ev("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 20 + n}})
		out.ev("message_stop", map[string]any{})
	}
	call := func(name string, input map[string]any) {
		b, _ := json.Marshal(input)
		out.ev("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": fmt.Sprintf("toolu_%d", n), "name": name, "input": map[string]any{}}})
		out.ev("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(b)}})
		out.ev("content_block_stop", map[string]any{"index": 0})
		out.ev("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "tool_use"}, "usage": map[string]any{"output_tokens": 20 + n}})
		out.ev("message_stop", map[string]any{})
	}
	switch {
	case strings.Contains(body.Model, "haiku"): // session title request
		text("Stub title")
	case strings.Contains(req.Text, "CHATSTUB"):
		text(fmt.Sprintf("STUB_CHAT_REPLY user_messages=%d", userMsgs))
	default:
		key := ticketKey.FindString(req.Text)
		if key == "PAY-224" { // the cancellation scenario: stream until the client goes away
			s.once.Do(func() { close(s.slowStarted) })
			out.ev("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
			for i := 0; ; i++ {
				select {
				case <-r.Context().Done():
					close(s.disconnected)
					return
				case <-time.After(200 * time.Millisecond):
					out.ev("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": fmt.Sprintf("slow-%d ", i)}})
				}
			}
		}
		// MCP tools are not advertised on the first step of a turn (verified
		// against 2.2.1), so step one is a (policy-blocked) read.
		switch toolResults {
		case 0:
			call("read", map[string]any{"path": "AGENTS.md"})
		case 1:
			call("lazyagents_claim_ticket", map[string]any{"ticket": key})
		case 2:
			call("lazyagents_post_plan", map[string]any{"steps": []string{"Read context", "Report"}})
		case 3:
			call("lazyagents_complete_run", map[string]any{"outcome": "Handed back by the stub provider", "status": "todo"})
		default:
			text("All done.")
		}
	}
}

// realOpenCode builds the launcher and returns an env wired to the real CLI.
func realOpenCodeEnv(t *testing.T) (env, *stubProvider) {
	t.Helper()
	if os.Getenv("LAZYAGENTS_REAL_OPENCODE") != "1" {
		t.Skip("set LAZYAGENTS_REAL_OPENCODE=1 to run the real opencode binary against the localhost stub provider")
	}
	real := os.Getenv("LAZYAGENTS_REAL_OPENCODE_BIN")
	if real == "" {
		var err error
		if real, err = exec.LookPath("opencode"); err != nil {
			t.Skip("opencode not on PATH and LAZYAGENTS_REAL_OPENCODE_BIN not set")
		}
	}
	real, _ = filepath.EvalSymlinks(real)
	stub := newStubProvider(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(launcherSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module launcher\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "opencode-launcher")
	build := exec.Command("go", "build", "-ldflags", "-X main.real="+real+" -X main.base="+stub.URL, "-o", launcher, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build launcher: %v\n%s", err, out)
	}
	e := setup(t) // replaces PATH with the fake-only PATH and fails if a real CLI is on it
	local := e.runner.Backend.(*runner.Local)
	if !local.OnlyBins || !local.OpenCodeIsolated {
		t.Fatal("the OnlyBins/OpenCodeIsolated guards must stay on")
	}
	local.Bins["opencode"] = launcher
	// A host key must never reach the stub or the process.
	t.Setenv("ANTHROPIC_API_KEY", "host-anthropic-must-not-leak")
	e.putSecrets("harness:anthropic")
	e.runs.OpenCodeIsolated = true
	e.runs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		return []chatmodels.Model{{ID: "claude-sonnet-5-5", Default: true}, {ID: "claude-opus-5-5"}}, nil
	}
	e.useOpenCode(ocSonnet)
	return e, stub
}

func mainRequests(stub *stubProvider) (out []stubReq) {
	for _, r := range stub.requests() {
		if !strings.Contains(r.Model, "haiku") {
			out = append(out, r)
		}
	}
	return out
}

func TestRealOpenCodeAgainstStubTicketTurn(t *testing.T) {
	e, stub := realOpenCodeEnv(t)
	e.dispatch("PAY-214")
	defer func() {
		if t.Failed() {
			evs, _ := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: e.run("PAY-214").ID, Seq: 0})
			for _, ev := range evs {
				if ev.Type == "tool_result" || ev.Type == "message" {
					t.Logf("%s %s", ev.Type, ev.Payload)
				}
			}
		}
	}()
	e.until("completed through complete_run", e.state("PAY-214=completed"))
	run := e.run("PAY-214")
	evs := e.eventTypes(run.ID)
	for _, want := range []string{"lifecycle:harness_started", "tool_call:read", "tool_result:read", "tool_call:lazyagents_claim_ticket", "tool_result:lazyagents_claim_ticket", "tool_call:lazyagents_complete_run", "usage"} {
		if !strings.Contains(evs, want) {
			t.Errorf("missing %s in %s", want, evs)
		}
	}
	if run.HarnessSessionID == nil || !strings.HasPrefix(*run.HarnessSessionID, "ses_") {
		t.Fatalf("session %v", run.HarnessSessionID)
	}
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if len(turns) != 1 || turns[0].ExitCode == nil || *turns[0].ExitCode != 0 {
		t.Fatalf("turns %+v", turns)
	}
	// Usage on the real wire is a lower bound (no final step_finish) with an
	// unknown cost, never a fabricated one.
	if run.InputTokens == 0 || run.CostSource == nil || *run.CostSource != "unknown" || run.CostUsdMicros != 0 {
		t.Fatalf("usage %d cost %v/%d", run.InputTokens, run.CostSource, run.CostUsdMicros)
	}
	mains := mainRequests(stub)
	if len(mains) != 5 {
		t.Fatalf("%d model requests, want 5 (read, claim, plan, complete, final)", len(mains))
	}
	for _, r := range stub.requests() {
		if r.Key != anthropicSecret {
			t.Errorf("provider saw key %q, want only the run's own provider secret", r.Key)
		}
	}
	// Documented 2.2.1 behaviour: MCP tools appear from the second step.
	has := func(r stubReq, tool string) bool {
		for _, n := range r.Tools {
			if n == tool {
				return true
			}
		}
		return false
	}
	if has(mains[0], "lazyagents_claim_ticket") || !has(mains[1], "lazyagents_claim_ticket") {
		t.Errorf("MCP tool advertisement changed: step1=%v step2=%v", mains[0].Tools, mains[1].Tools)
	}
	if !strings.Contains(mains[0].Text, "PAY-214") {
		t.Errorf("the run prompt never reached the model")
	}
	if !strings.Contains(mains[0].Text, "lazyagents:start") && !strings.Contains(mains[0].Text, "LazyAgents run") {
		t.Errorf("the generated AGENTS.md is not in the system prompt")
	}
}

func TestRealOpenCodeAgainstStubChatResumes(t *testing.T) {
	e, stub := realOpenCodeEnv(t)
	chat := e.startChat("CHATSTUB first")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if first.HarnessSessionID == nil || !strings.HasPrefix(*first.HarnessSessionID, "ses_") {
		t.Fatalf("session %v", first.HarnessSessionID)
	}
	second := e.sayAs(chat.ID, "CHATSTUB second", "", "")
	run := e.chatRun(chat.ID)
	if run.ID != first.ID || deref(run.HarnessSessionID) != deref(first.HarnessSessionID) {
		t.Fatalf("chat did not resume the same session: %+v", run)
	}
	if !strings.Contains(second.BodyMd, "STUB_CHAT_REPLY user_messages=2") {
		t.Fatalf("the resumed turn did not carry the first turn's history: %q", second.BodyMd)
	}
	mains := mainRequests(stub)
	if len(mains) != 2 || mains[1].Msgs <= mains[0].Msgs {
		t.Fatalf("requests %+v", mains)
	}
	spec, err := runs.Spec(run)
	if err != nil || spec.HarnessOptions["readonly"] != "true" {
		t.Fatalf("chat spec %+v %v", spec, err)
	}
	for _, s := range spec.Scopes {
		if s.Mode != agentdef.Denied {
			t.Fatalf("chat scope %s is %v", s.Scope, s.Mode)
		}
	}
}

func TestRealOpenCodeAgainstStubCancel(t *testing.T) {
	e, stub := realOpenCodeEnv(t)
	e.dispatch("PAY-224")
	run := e.run("PAY-224")
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				e.runner.Tick(e.ctx)
				time.Sleep(30 * time.Millisecond)
			}
		}
	}()
	select {
	case <-stub.slowStarted:
	case <-time.After(60 * time.Second):
		t.Fatal("the real CLI never reached the provider")
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stub.disconnected:
	case <-time.After(30 * time.Second):
		t.Fatal("cancellation did not end the provider connection (a descendant is still streaming)")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		turns, _ := e.q.ListTurns(e.ctx, run.ID)
		if len(turns) > 0 && turns[0].Status == "done" {
			if turns[0].ExitCode == nil || *turns[0].ExitCode == 0 {
				t.Fatalf("cancelled turn exit %v", turns[0].ExitCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the cancelled turn never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(stop)
	<-done
	e.runner.Wait()
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	if r := e.run("PAY-224"); r.State != "cancelled" {
		t.Fatalf("state %s", r.State)
	}
}
