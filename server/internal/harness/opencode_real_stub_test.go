package harness

import (
	"strings"
	"testing"
)

// These parse captures from the REAL opencode 2.2.1 binary run against a
// localhost stub provider (testdata/opencode-real-stub-*.jsonl, no paid call;
// docs/contracts/opencode2/verification.md). The stub chose the model output;
// the envelope, event order and field names are the CLI's own.

func TestOpenCodeRealStubSuccessHasNoCompletionMarker(t *testing.T) {
	evs, res := ocRun(t, "opencode-real-stub-text.jsonl", 0)
	if res.IsError || !strings.HasPrefix(res.Final, "STUB_REPLY_") || res.SessionID == "" {
		t.Fatalf("%+v", res)
	}
	if got := types(evs); got != "lifecycle message" {
		t.Fatalf("events %s", got)
	}
	// The last step of a turn has no step_finish: no usage at all, and never a
	// guessed cost.
	if res.Usage != (Usage{}) {
		t.Fatalf("usage invented: %+v", res.Usage)
	}
	// A zero exit is part of the verdict: the same stream without it fails.
	if _, res = ocRun(t, "opencode-real-stub-text.jsonl", 1); !res.IsError {
		t.Fatalf("nonzero exit succeeded: %+v", res)
	}
	if _, res = ocRun(t, "opencode-real-stub-text.jsonl", -1); !res.IsError {
		t.Fatalf("stream without exit status succeeded: %+v", res)
	}
}

// step_finish usage is per step (not cumulative) and the stream omits the final
// step, so totals are a lower bound with an unknown cost.
func TestOpenCodeRealStubToolTurnUsageIsPartial(t *testing.T) {
	evs, res := ocRun(t, "opencode-real-stub-tools-blocked.jsonl", 0)
	if res.IsError || res.Final != "STUB_FINAL after tool result" {
		t.Fatalf("%+v", res)
	}
	want := "lifecycle tool_call:read tool_result:read usage tool_call:lazyagents_claim_ticket tool_result:lazyagents_claim_ticket usage message"
	if got := types(evs); got != want {
		t.Fatalf("events\n got %s\nwant %s", got, want)
	}
	u := res.Usage
	// Two step_finish events (1150 + 1170 input, 35 + 37 output, 5 + 5 cache reads).
	if u.InputTokens != 2320 || u.OutputTokens != 72 || u.CachedTokens != 10 || u.ToolCalls != 2 || u.CostSource != "unknown" || u.CostUSD != 0 {
		t.Fatalf("usage %+v", u)
	}
	last := evs[len(evs)-2].Payload
	if last["usage_partial"] != true || last["cost_known"] != false {
		t.Fatalf("partial usage not flagged: %v", last)
	}
	// A policy denial is a failed tool, not a failed run.
	for _, e := range evs {
		if e.Type == "tool_result" && e.Payload["tool"] == "read" {
			if e.Payload["is_error"] != true || !strings.Contains(e.Payload["output"].(string), "Blocked by configuration policy") {
				t.Fatalf("denial not surfaced: %v", e.Payload)
			}
		}
		if e.Type == "tool_result" && e.Payload["tool"] == "lazyagents_claim_ticket" && e.Payload["is_error"] != false {
			t.Fatalf("completed MCP call flagged: %v", e.Payload)
		}
	}
}

func TestOpenCodeRealStubToolStatuses(t *testing.T) {
	evs, res := ocRun(t, "opencode-real-stub-tools-completed.jsonl", 0)
	if res.IsError || res.Usage.ToolCalls != 9 {
		t.Fatalf("%+v", res)
	}
	status := map[string]bool{}
	for _, e := range evs {
		if e.Type == "tool_result" {
			status[e.Payload["tool"].(string)] = e.Payload["is_error"].(bool)
		}
	}
	for tool, isErr := range map[string]bool{"read": false, "write": false, "shell": false, "lazyagents_claim_ticket": false, "edit": true, "evil_probe": true} {
		if got, ok := status[tool]; !ok || got != isErr {
			t.Errorf("%s is_error=%v (seen %v), want %v", tool, got, ok, isErr)
		}
	}
	_, res = ocRun(t, "opencode-real-stub-shell-denied.jsonl", 0)
	if res.IsError || res.Usage.ToolCalls != 3 {
		t.Fatalf("%+v", res)
	}
}

// A resumed turn reports only its own steps: usage is not session-cumulative.
func TestOpenCodeRealStubResumeIsOwnTurn(t *testing.T) {
	_, res := ocRun(t, "opencode-real-stub-resume.jsonl", 0)
	if res.IsError || !strings.HasPrefix(res.Final, "STUB_REPLY_") || res.Usage.InputTokens != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestOpenCodeRealStubQuotaIsUsageLimited(t *testing.T) {
	_, res := ocRun(t, "opencode-real-stub-quota-error.jsonl", 1)
	if !res.IsError || !res.UsageLimited || !strings.Contains(res.Error, "provider.invalid-request") || res.Final != "" {
		t.Fatalf("%+v", res)
	}
}

// SIGINT/SIGTERM mid-stream: one step_start, then an error event, exit 130.
func TestOpenCodeRealStubCancellationFails(t *testing.T) {
	_, res := ocRun(t, "opencode-real-stub-cancel.jsonl", 130)
	if !res.IsError || res.UsageLimited || res.Final != "" {
		t.Fatalf("%+v", res)
	}
}
