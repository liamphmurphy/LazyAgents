package harness

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestCodexUnknownCostAndConfiguredPrice(t *testing.T) {
	line := []byte(`{"type":"turn.completed","usage":{"input_tokens":1000000,"cached_input_tokens":250000,"output_tokens":100000}}`)
	p := (Codex{}).NewParser("unpriced-model")
	events := p.Line(line)
	if p.Result().Usage.CostSource != "unknown" || events[0].Payload["cost_known"] != false {
		t.Fatal("unpriced turn must be unknown")
	}
	if _, ok := events[0].Payload["cost_usd"]; ok {
		t.Fatal("unpriced turn displayed a fabricated zero cost")
	}
	p = (Codex{Prices: map[string]Price{"unpriced-model": {Input: 2, CachedInput: .2, Output: 10}}}).NewParser("unpriced-model")
	p.Line(line)
	if got := p.Result().Usage; got.CostSource != "computed" || got.CostUSD != 2.55 {
		t.Fatalf("configured price: %+v", got)
	}
}

func TestNativeSkillsAndPolicyHooks(t *testing.T) {
	for _, h := range []Harness{ClaudeCode{}, Codex{}} {
		inv, err := h.Invocation(ws, Turn{Spec: spec("repo:read", "allowed"), Prompt: "go"})
		if err != nil {
			t.Fatal(err)
		}
		if len(inv.Files[toolHookPath]) == 0 || inv.Env["LAZYAGENTS_POLICY_TOKEN"] != ws.Token || inv.Env["LAZYAGENTS_POLICY_URL"] != "http://api:8000/api/v1/agent/tool-policy" {
			t.Fatalf("%s missing authenticated policy", h.Name())
		}
	}
}

func TestClaudeCostAvailability(t *testing.T) {
	for _, tt := range []struct {
		line  string
		known bool
	}{
		{`{"type":"result","is_error":false,"total_cost_usd":0}`, true},
		{`{"type":"result","is_error":false}`, false},
	} {
		p := (ClaudeCode{}).NewParser("")
		events := p.Line([]byte(tt.line))
		if events[0].Payload["cost_known"] != tt.known {
			t.Fatalf("availability=%v", events[0].Payload)
		}
	}
}

func TestCodexMCPFailureNormalization(t *testing.T) {
	p := (Codex{}).NewParser("test")
	for _, line := range []string{
		`{"type":"item.completed","item":{"id":"a","type":"mcp_tool_call","server":"lazyagents","tool":"whoami","status":"completed","error":{"message":"approval required"}}}`,
		`{"type":"item.completed","item":{"id":"b","type":"mcp_tool_call","server":"lazyagents","tool":"whoami","status":"completed","result":{"isError":true,"content":[]}}}`,
	} {
		ev := p.Line([]byte(line))
		if len(ev) != 1 || ev[0].Payload["is_error"] != true {
			t.Fatalf("MCP failure lost: %+v", ev)
		}
	}
}

func TestClaudeSpotsRichBlocksAsTheyStream(t *testing.T) {
	p := (ClaudeCode{}).NewParser("test")
	delta := func(text string) []Event {
		line, _ := json.Marshal(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": text}}})
		return p.Line(line)
	}
	if ev := delta("Here you go.\n\n```app"); ev != nil {
		t.Fatalf("seen before the tag was complete: %+v", ev)
	}
	ev := delta("\n<!doctype html>\n")
	if len(ev) != 1 || ev[0].Type != "lifecycle" || ev[0].Payload["state"] != "reply_block" || ev[0].Payload["kind"] != "app" {
		t.Fatalf("app block: %+v", ev)
	}
	if ev := delta("```mermaid\n```\n```go\n```app\n```\n"); len(ev) != 0 {
		t.Fatalf("fences inside a block: %+v", ev)
	}
	if ev := delta("```mermaid\n"); len(ev) != 1 || ev[0].Payload["kind"] != "mermaid" {
		t.Fatalf("mermaid block: %+v", ev)
	}
	child := `{"type":"stream_event","parent_tool_use_id":"x","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"\n` + "```" + `\n` + "```" + `app\n"}}}`
	if ev := p.Line([]byte(child)); ev != nil {
		t.Fatalf("subagent text: %+v", ev)
	}
}

func TestCodexResumeUsageUsesDeltas(t *testing.T) {
	baseline := Usage{SessionID: "same", SessionInputTokens: 131141, SessionOutputTokens: 446, SessionCachedTokens: 112256}
	p := (Codex{Baseline: baseline}).NewParser("unpriced-model")
	p.Line([]byte(`{"type":"thread.started","thread_id":"same"}`))
	p.Line([]byte(`{"type":"turn.completed","usage":{"input_tokens":194826,"cached_input_tokens":175488,"output_tokens":624}}`))
	u := p.Result().Usage
	if u.InputTokens != 63685 || u.OutputTokens != 178 || u.CachedTokens != 63232 || u.SessionInputTokens != 194826 {
		t.Fatalf("session totals counted twice: %+v", u)
	}
	// A fresh session must not subtract the previous harness/session's usage.
	p = (Codex{Baseline: baseline}).NewParser("unpriced-model")
	p.Line([]byte(`{"type":"thread.started","thread_id":"new"}`))
	p.Line([]byte(`{"type":"turn.completed","usage":{"input_tokens":200,"cached_input_tokens":100,"output_tokens":30}}`))
	if got := p.Result().Usage; got.InputTokens != 200 || got.OutputTokens != 30 {
		t.Fatalf("new session reused baseline: %+v", got)
	}
}

func TestCodexHookTrustNoticeIsNotAnError(t *testing.T) {
	p := (Codex{}).NewParser("test")
	line := []byte("{\"type\":\"error\",\"message\":\"`--dangerously-bypass-hook-trust` is enabled. Enabled hooks may run without review for this invocation.\"}")
	events := p.Line(line)
	if len(events) != 1 || events[0].Type != "lifecycle" || events[0].Payload["state"] != "harness_notice" {
		t.Fatalf("warning classified as failure: %+v", events)
	}
	if len(p.Line(line)) != 0 {
		t.Fatal("duplicate notice was emitted")
	}
	if p.Result().Error != "" || p.Result().IsError {
		t.Fatal("notice poisoned the turn result")
	}
	events = p.Line([]byte(`{"type":"error","message":"Actual hook execution failed"}`))
	if len(events) != 1 || events[0].Type != "error" || p.Result().Error != "Actual hook execution failed" {
		t.Fatal("real errors must remain visible")
	}
}

func TestClaudeResumeCostUsesDeltas(t *testing.T) {
	baseline := Usage{SessionID: "same", SessionCostUSD: 0.88}
	p := (ClaudeCode{Baseline: baseline}).NewParser("")
	p.Line([]byte(`{"type":"system","subtype":"init","session_id":"same"}`))
	ev := p.Line([]byte(`{"type":"result","is_error":false,"total_cost_usd":1.04,"usage":{"input_tokens":10,"cache_read_input_tokens":200,"output_tokens":5}}`))
	u := p.Result().Usage
	if math.Abs(u.CostUSD-0.16) > 1e-9 || u.SessionCostUSD != 1.04 || u.SessionID != "same" || u.InputTokens != 210 {
		t.Fatalf("session cost counted twice: %+v", u)
	}
	if c, _ := ev[0].Payload["cost_usd"].(float64); math.Abs(c-0.16) > 1e-9 {
		t.Fatalf("usage event shows session total: %v", ev[0].Payload)
	}
	// A fresh session (or a lower total) must not subtract the previous session's cost.
	for _, init := range []string{
		`{"type":"system","subtype":"init","session_id":"new"}`,
		`{"type":"system","subtype":"init","session_id":"same"}`,
	} {
		p = (ClaudeCode{Baseline: baseline}).NewParser("")
		p.Line([]byte(init))
		p.Line([]byte(`{"type":"result","is_error":false,"total_cost_usd":0.5}`))
		if got := p.Result().Usage; got.CostUSD != 0.5 {
			t.Fatalf("baseline wrongly applied after %s: %+v", init, got)
		}
	}
}

// How a reply is split into deltas mustn't change which blocks are seen.
func TestReplyScannerIgnoresDeltaBoundaries(t *testing.T) {
	text := "Intro\n  ```chart  \n{}\n```\n```go\n```app\n```\n" + "```app\n<html>" + strings.Repeat("x", 5000) + "\n```\n```recommend\n"
	kinds := func(chunk int) []any {
		var s replyScanner
		var out []any
		for i := 0; i < len(text); i += chunk {
			for _, e := range s.feed(text[i:min(len(text), i+chunk)]) {
				out = append(out, e.Payload["kind"])
			}
		}
		return out
	}
	want := kinds(len(text))
	if fmt.Sprint(want) != "[chart app recommend]" {
		t.Fatalf("whole reply: %v", want)
	}
	for _, chunk := range []int{1, 2, 3, 7, 64} {
		if got := kinds(chunk); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("chunk %d: %v, want %v", chunk, got, want)
		}
	}
}

func TestClaudeAssistantPlainStringContent(t *testing.T) {
	p := (ClaudeCode{}).NewParser("test")
	ev := p.Line([]byte(`{"type":"assistant","message":{"role":"assistant","content":"Done."}}`))
	if len(ev) != 1 || ev[0].Type != "message" || ev[0].Payload["text"] != "Done." || p.Result().Final != "Done." {
		t.Fatalf("string content: %+v", ev)
	}
	for _, line := range []string{
		`{"type":"user","message":{"role":"user","content":"a prompt"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":5}]}}`,
		`{"type":"assistant","message":"oops"}`,
		`{"type":"assistant","message":{"content":null}}`,
	} {
		if ev := p.Line([]byte(line)); ev != nil {
			t.Errorf("%s: %+v", line, ev)
		}
	}
}
