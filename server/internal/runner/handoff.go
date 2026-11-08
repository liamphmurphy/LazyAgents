package runner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// handoffTranscript transfers the service-visible conversation across harnesses,
// including tool outcomes and approved/rejected actions. Native session files
// remain harness-specific. Events have already passed through secret redaction.
func handoffTranscript(ctx context.Context, q *store.Queries, runID uuid.UUID) (string, error) {
	var recent []string
	var seq int64
	for {
		events, err := q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: runID, Seq: seq})
		if err != nil {
			return "", err
		}
		for _, event := range events {
			seq = event.Seq
			switch event.Type {
			case "message", "tool_call", "tool_result", "error", "lifecycle":
				var payload map[string]any
				if json.Unmarshal(event.Payload, &payload) != nil {
					continue
				}
				// Startup metadata and usage are not conversation history.
				if event.Type == "lifecycle" && payload["trigger"] != "approval" {
					continue
				}
				raw, _ := json.Marshal(map[string]any{"type": event.Type, "payload": payload})
				recent = append(recent, harnessContext(string(raw), 4000))
				for len(recent) > 60 || len(strings.Join(recent, "\n")) > 48000 {
					recent = recent[1:]
				}
			}
		}
		if len(events) < 1000 {
			break
		}
	}
	return strings.Join(recent, "\n"), nil
}
