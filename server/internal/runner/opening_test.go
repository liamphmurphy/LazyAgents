package runner

import "testing"

func TestOpeningLine(t *testing.T) {
	type ev struct {
		typ string
		p   map[string]any
	}
	msg := func(text string) ev { return ev{"message", map[string]any{"role": "assistant", "text": text}} }
	call := ev{"tool_call", map[string]any{"tool": "list_runs"}}
	cases := []struct {
		name   string
		events []ev
		want   string
	}{
		{"straight answer", []ev{msg("Hello!")}, ""},
		{"opening then work", []ev{msg("  Let me check.  "), call, msg("Two are running."), call}, "Let me check."},
		{"tool call first", []ev{call, msg("Done.")}, ""},
		{"plan text skipped", []ev{{"message", map[string]any{"role": "assistant", "kind": "plan", "text": "plan"}}, call}, ""},
	}
	for _, tc := range cases {
		var o openingLine
		for _, e := range tc.events {
			o.see(e.typ, e.p)
		}
		if o.line != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, o.line, tc.want)
		}
	}
}
