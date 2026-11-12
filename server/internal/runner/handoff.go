package runner

import (
	"cmp"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Handoff tail limits: entries kept, and bytes per field of an entry.
const (
	handoffTailEvents = 15
	handoffCommandMax = 200
	handoffOutputMax  = 300
)

// handoffTranscript renders the last few things a run did as text, one line
// each, so a fresh session on another harness can pick up work in flight: a
// tool call with the start of its output, an assistant message, an error or an
// approval. The run's state itself comes from the brief. Events have already
// passed through secret redaction.
func handoffTranscript(ctx context.Context, q *store.Queries, runID uuid.UUID) (string, error) {
	type entry struct{ line string }
	var recent []*entry
	calls := map[string]*entry{}
	add := func(line string) *entry {
		e := &entry{line}
		recent = append(recent, e)
		if len(recent) > handoffTailEvents {
			recent = recent[1:]
		}
		return e
	}
	var seq int64
	for {
		events, err := q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: runID, Seq: seq})
		if err != nil {
			return "", err
		}
		for _, event := range events {
			seq = event.Seq
			var p map[string]any
			if json.Unmarshal(event.Payload, &p) != nil {
				continue
			}
			text := func(k string) string { s, _ := p[k].(string); return s }
			switch event.Type {
			case "message":
				// Thinking is one harness's private scratch work, not the conversation.
				if p["kind"] == "thinking" || text("text") == "" {
					continue
				}
				add(cmp.Or(text("role"), "assistant") + ": " + oneLine(text("text"), handoffOutputMax))
			case "tool_call":
				e := add("tool " + text("tool") + ": " + oneLine(toolInput(p["input"]), handoffCommandMax))
				if id := text("id"); id != "" {
					calls[id] = e
				}
			case "tool_result":
				out := oneLine(text("output"), handoffOutputMax)
				if text("output") == "" {
					out = "(no output)"
				}
				if p["is_error"] == true {
					out = "error: " + out
				}
				if e, ok := calls[text("id")]; ok {
					delete(calls, text("id"))
					e.line += " → " + out
				} else {
					add("tool " + text("tool") + " result → " + out)
				}
			case "error":
				add("error: " + oneLine(text("message"), handoffOutputMax))
			case "lifecycle":
				// Startup metadata and usage are not conversation history.
				if p["trigger"] == "approval" {
					add("resumed after an approval: " + oneLine(text("message"), handoffOutputMax))
				}
			}
		}
		if len(events) < 1000 {
			break
		}
	}
	lines := make([]string, len(recent))
	for i, e := range recent {
		lines[i] = "- " + e.line
	}
	return strings.Join(lines, "\n"), nil
}

// toolInput is the part of a tool call's input worth showing: the shell
// command when there is one, otherwise the input as JSON.
func toolInput(in any) string {
	if m, ok := in.(map[string]any); ok {
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	}
	if in == nil {
		return ""
	}
	raw, _ := json.Marshal(in)
	return string(raw)
}

// oneLine collapses whitespace and clips s to at most limit bytes on a rune boundary.
func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit] + "…"
}
