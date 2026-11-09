package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

// feedDeltas streams text into a fresh replyScanner in chunk-sized deltas,
// the way Claude's text_delta events arrive.
func feedDeltas(text string, chunk int) int {
	var s replyScanner
	n := 0
	for i := 0; i < len(text); i += chunk {
		n += len(s.feed(text[i:min(len(text), i+chunk)]))
	}
	return n
}

func BenchmarkReplyScanner(b *testing.B) {
	prose := strings.Repeat("Here is a short paragraph of reply text that wraps onto its own line.\n\n", 200) +
		"```chart\n{\"type\":\"bar\"}\n```\n"
	// A minified app or SVG body is one long line inside its fence.
	longLine := "```app\n<!doctype html><script>" + strings.Repeat("let x=document.getElementById('a');x.textContent='hi';", 2000) + "</script>\n```\n"
	for _, c := range []struct {
		name string
		text string
	}{{"prose", prose}, {"long_line_110KB", longLine}} {
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(c.text)))
			b.ReportAllocs()
			for b.Loop() {
				feedDeltas(c.text, 24)
			}
		})
	}
}

func claudeBenchLines(b *testing.B) map[string][]byte {
	b.Helper()
	mustJSON := func(v any) []byte {
		out, err := json.Marshal(v)
		if err != nil {
			b.Fatal(err)
		}
		return out
	}
	file := strings.Repeat("func handler(w http.ResponseWriter, r *http.Request) {\n\tw.Write([]byte(\"ok\"))\n}\n", 250) // ~20 KB
	read := strings.Repeat("     1\tpackage main // a line of a file the agent read back\n", 2000)                        // ~110 KB
	return map[string][]byte{
		"stream_delta": mustJSON(map[string]any{"type": "stream_event", "session_id": "s1", "parent_tool_use_id": nil,
			"event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "Looking at the handler now.\n"}}}),
		"assistant_tool_use_20KB": mustJSON(map[string]any{"type": "assistant", "session_id": "s1", "parent_tool_use_id": nil,
			"message": map[string]any{"id": "msg_1", "role": "assistant", "model": "claude", "content": []any{
				map[string]any{"type": "text", "text": "Writing the handler."},
				map[string]any{"type": "tool_use", "id": "tu_1", "name": "Write", "input": map[string]any{"file_path": "/w/main.go", "content": file}},
			}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}}),
		"user_tool_result_110KB": mustJSON(map[string]any{"type": "user", "session_id": "s1", "parent_tool_use_id": nil,
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu_1", "content": read, "is_error": false},
			}}}),
	}
}

func BenchmarkClaudeParserLine(b *testing.B) {
	lines := claudeBenchLines(b)
	for _, name := range []string{"stream_delta", "assistant_tool_use_20KB", "user_tool_result_110KB"} {
		line := lines[name]
		b.Run(name, func(b *testing.B) {
			p := ClaudeCode{}.NewParser("")
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()
			for b.Loop() {
				p.Line(line)
			}
		})
	}
}
