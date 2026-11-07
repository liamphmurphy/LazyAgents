package runner_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func TestRunnerRecordsEnvironmentOnSetupFailure(t *testing.T) {
	e := setup(t)
	svc := &agents.Service{Pool: e.pool}
	if err := svc.SetOverride(e.ctx, e.ws, e.me.ID, "atlas", "PAY", agentdef.Override{Setup: []string{"exit 79"}}); err != nil {
		t.Fatal(err)
	}
	e.dispatch("PAY-214")
	e.until("workspace setup failed", e.state("PAY-214=failed"))
	run := e.run("PAY-214")
	events, err := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		var payload struct {
			State  string                `json:"state"`
			Report api.EnvironmentReport `json:"report"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.State != "environment_snapshot" {
			continue
		}
		if payload.Report.Status != "failed" || payload.Report.Error == nil || len(payload.Report.Caches) != 4 {
			t.Fatalf("%+v", payload.Report)
		}
		return
	}
	t.Fatal("failed setup did not record its environment")
}

func TestManagedClaimDoesNotRepeatWorkspaceContext(t *testing.T) {
	e := setup(t)
	e.dispatch("PAY-214")
	e.until("waiting for approval", e.state("PAY-214=waiting"))
	run := e.run("PAY-214")
	events, err := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type != "tool_result" {
			continue
		}
		var payload struct {
			Tool   string `json:"tool"`
			Output string `json:"output"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(payload.Tool, "claim_ticket") {
			continue
		}
		var claim map[string]any
		if err := json.Unmarshal([]byte(payload.Output), &claim); err != nil {
			t.Fatalf("%v %s", err, payload.Output)
		}
		if claim["instructions"] != nil || claim["memory"] != nil || claim["run_id"] == nil {
			t.Fatalf("claim repeated context: %v", claim)
		}
		return
	}
	t.Fatal("claim result not observed")
}
