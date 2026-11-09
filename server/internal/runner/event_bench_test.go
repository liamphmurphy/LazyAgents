package runner

import (
	"strings"
	"testing"
)

func benchEventPayloads() (map[string]map[string]any, map[string]string) {
	env := map[string]string{
		"LAZYAGENTS_TOKEN": "vsp_" + strings.Repeat("a1B2", 10), "ANTHROPIC_API_KEY": "sk-ant-" + strings.Repeat("x9", 20),
		"GH_TOKEN": "ghp_" + strings.Repeat("Q", 36), "LAZYAGENTS_API": "http://127.0.0.1:8080", "HOOK_SOCKET": "/tmp/vesper/hook.sock",
		"DISABLE_TELEMETRY": "1", "codex_access": "", "codex_refresh": "", "codex_id": "",
	}
	file := strings.Repeat("func handler(w http.ResponseWriter, r *http.Request) {\n\tw.Write([]byte(\"ok\"))\n}\n", 250)
	return map[string]map[string]any{
		"message":         {"role": "assistant", "text": "Looking at the handler now, then I'll run the tests."},
		"tool_call_20KB":  {"id": "tu_1", "tool": "Write", "input": map[string]any{"file_path": "/w/main.go", "content": file}},
		"tool_result_4KB": {"id": "tu_1", "tool": "Read", "is_error": false, "output": file[:4000]},
	}, env
}

func BenchmarkEventPayload(b *testing.B) {
	payloads, env := benchEventPayloads()
	for _, name := range []string{"message", "tool_call_20KB", "tool_result_4KB"} {
		p := payloads[name]
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				// What watchSession does per streamed event before storing it.
				redactPayload(p, env)
			}
		})
	}
}
