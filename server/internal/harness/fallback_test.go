package harness

import "testing"

func TestUsageLimitParsing(t *testing.T) {
	tests := []struct {
		name   string
		parser Parser
		lines  []string
		want   bool
	}{
		{"claude result", ClaudeCode{}.NewParser(""), []string{`{"type":"result","is_error":true,"result":"Credit balance is too low"}`}, true},
		{"claude assistant", ClaudeCode{}.NewParser(""), []string{`{"type":"assistant","error":"rate_limit","message":{"content":[{"type":"text","text":"You've hit your limit"}]}}`, `{"type":"result","is_error":false}`}, true},
		{"codex quota", Codex{}.NewParser("test"), []string{`{"type":"turn.failed","error":{"message":"usage_limit_reached"}}`}, true},
		{"codex rate limit", Codex{}.NewParser("test"), []string{`{"type":"error","message":"Rate limit reached"}`}, true},
		{"antigravity quota (recorded from agy 1.5.8)", Antigravity{}.NewParser(""), []string{`{"event":"result","result":{"conversation_id":"c","status":"ERROR","response":"","error":"agent executor error: generating and executing: Error 429, Message: You exceeded your current quota, please check your plan and billing details., Status: RESOURCE_EXHAUSTED, Details: []","finished_reason":"error"}}`}, true},
		{"antigravity other failure", Antigravity{}.NewParser(""), []string{`{"event":"result","result":{"conversation_id":"c","status":"ERROR","response":"","error":"agent executor error: generating and executing: Error 404, Message: , Status: 404 Not Found, Details: []","finished_reason":"error"}}`}, false},
		{"ordinary failure", Codex{}.NewParser("test"), []string{`{"type":"turn.failed","error":{"message":"Invalid API key"}}`}, false},
		{"tool text is not exhaustion", ClaudeCode{}.NewParser(""), []string{`{"type":"user","message":{"content":[{"type":"tool_result","content":"usage limit reached","is_error":true}]}}`}, false},
		{"normal message is not exhaustion", Codex{}.NewParser("test"), []string{`{"type":"item.completed","item":{"type":"agent_message","text":"Implemented usage limit handling"}}`}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, line := range tt.lines {
				tt.parser.Line([]byte(line))
			}
			if got := tt.parser.Result().UsageLimited; got != tt.want {
				t.Fatalf("UsageLimited=%v, want %v", got, tt.want)
			}
		})
	}
}
