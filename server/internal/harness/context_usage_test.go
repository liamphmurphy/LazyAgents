package harness

import (
	"strings"
	"testing"
)

func TestClaudeContextTokensAndToolOutput(t *testing.T) {
	big := strings.Repeat("x", 9000) // larger than the event clip
	lines := []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200},"content":[{"type":"tool_use","id":"r1","name":"Read","input":{}},{"type":"tool_use","id":"b1","name":"Bash","input":{}},{"type":"tool_use","id":"m1","name":"mcp__lazyagents__get_run","input":{}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"` + big + `"},{"type":"tool_result","tool_use_id":"b1","content":[{"type":"text","text":"hello"}]},{"type":"tool_result","tool_use_id":"m1","content":"abc"}]}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":5,"cache_read_input_tokens":3000,"cache_creation_input_tokens":40},"content":[{"type":"tool_use","id":"g1","name":"Grep","input":{}}]}}`,
		// A sub agent's request and results: neither its context nor its output count.
		`{"type":"assistant","parent_tool_use_id":"task1","message":{"usage":{"input_tokens":1,"cache_read_input_tokens":99999},"content":[{"type":"tool_use","id":"s1","name":"Read","input":{}}]}}`,
		`{"type":"user","parent_tool_use_id":"task1","message":{"content":[{"type":"tool_result","tool_use_id":"s1","content":"ignored"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"g1","content":"12"}]}}`,
		`{"type":"result","is_error":false,"result":"ok","usage":{"input_tokens":1,"output_tokens":1}}`,
	}
	p := ClaudeCode{}.NewParser("")
	ctxByTool := map[string]any{}
	for _, l := range lines {
		for _, ev := range p.Line([]byte(l)) {
			if ev.Type == "tool_call" {
				ctxByTool[ev.Payload["id"].(string)] = ev.Payload["context_tokens"]
			}
		}
	}
	// Each main-agent tool call carries its request's prompt size; a sub
	// agent's call carries none.
	want := map[string]any{"r1": int64(1210), "b1": int64(1210), "m1": int64(1210), "g1": int64(3045), "s1": nil}
	for id, w := range want {
		if ctxByTool[id] != w {
			t.Errorf("tool_call %s context_tokens = %v, want %v", id, ctxByTool[id], w)
		}
	}
	u := p.Result().Usage
	if u.ContextTokens != 3045 {
		t.Errorf("ContextTokens = %d, want 3045 (the second main-agent request)", u.ContextTokens)
	}
	if u.ExploreOutputBytes != 9002 || u.ShellOutputBytes != 5 || u.OtherOutputBytes != 3 {
		t.Errorf("output bytes explore=%d shell=%d other=%d, want 9002/5/3", u.ExploreOutputBytes, u.ShellOutputBytes, u.OtherOutputBytes)
	}
}

func TestCodexContextTokensUnknown(t *testing.T) {
	p := Codex{}.NewParser("gpt-5")
	p.Line([]byte(`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":10}}`))
	if got := p.Result().Usage.ContextTokens; got != 0 {
		t.Errorf("codex ContextTokens = %d, want 0 (unknown)", got)
	}
}
