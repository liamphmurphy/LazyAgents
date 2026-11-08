package runner_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// checkedGraph is Generate -> Verify (a check) -> Gate -> Post, as DnD Map
// Generator migrates to. edit adjusts the check and gate for each case.
func checkedGraph(edit func(check, gate *runs.WorkflowConfig)) runs.WorkflowGraph {
	check := runs.WorkflowConfig{Prompt: "Check it", Check: true}
	gate := runs.WorkflowConfig{}
	edit(&check, &gate)
	return runs.WorkflowGraph{Nodes: []runs.WorkflowNode{
		{ID: "start", Type: runs.NodeTrigger, Title: "Start"},
		{ID: "gen", Type: runs.NodeAgent, Title: "Generate", Config: runs.WorkflowConfig{Prompt: "Draw a map", Images: true}},
		{ID: "check", Type: runs.NodeAgent, Title: "Verify", Config: check},
		{ID: "ok", Type: runs.NodeGate, Title: "Approve", Config: gate},
		{ID: "post", Type: runs.NodeChat, Title: "Post"},
	}, Edges: []runs.WorkflowEdge{{From: "start", To: "gen"}, {From: "gen", To: "check"}, {From: "check", To: "ok"}, {From: "ok", To: "post"}}}
}

func (e wfImageEnv) notDelivered(t *testing.T, run store.WorkflowRun) {
	t.Helper()
	if _, reached := e.steps(run.ID)["post"]; reached {
		t.Fatal("the output ran")
	}
	for _, m := range e.chatMsgs(run.ChatID) {
		if strings.Contains(m.BodyMd, "looked at") {
			t.Fatalf("delivered: %q", m.BodyMd)
		}
	}
}

func decide(e wfImageEnv, run store.WorkflowRun, decision, note string) error {
	_, err := e.runs.DecideWorkflowGate(e.ctx, e.ws, e.me.ID, run.ID, runs.WorkflowDecision{NodeID: "ok", Decision: decision, Note: note})
	return err
}

// A failing check re-runs Generate with its findings, and the retry that
// passes reaches the gate; each attempt stays in the history.
func TestWorkflowCheckRetriesThenPasses(t *testing.T) {
	e := newWFImageEnv(t)
	t.Setenv("FAKE_IMAGE_CONTENT", "bad")
	run := e.startGraph(t, checkedGraph(func(check, gate *runs.WorkflowConfig) {
		check.RetryFrom, gate.RequiresCheck, gate.RetryFrom = "gen", "check", "gen"
	}))
	e.waitGate(run.ID, 1)
	steps := e.steps(run.ID)
	if steps["check"].Attempt != 2 || steps["check"].CheckResult != "passed" || steps["gen"].Attempt != 2 {
		t.Fatalf("%+v", steps)
	}
	calls := e.calls()
	if len(calls) != 4 || !strings.Contains(calls[2].Prompt, "The check Verify failed") || !strings.Contains(calls[2].Prompt, "FAIL: looked at 1") {
		t.Fatalf("the findings didn't reach the retry: %+v", calls)
	}
	history, err := e.q.ListWorkflowCheckAttempts(e.ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].CheckResult != "failed" || history[0].Attempt != 1 || history[1].CheckResult != "passed" || history[1].Attempt != 2 {
		t.Fatalf("%+v", history)
	}
	if err := decide(e, run, "approve", ""); err != nil {
		t.Fatal(err)
	}
	e.waitRun(run.ID, "done")
}

// Attempts run out: the run fails clearly and nothing is delivered.
func TestWorkflowCheckStopsAtItsLimit(t *testing.T) {
	e := newWFImageEnv(t)
	t.Setenv("FAKE_CHECK_VERDICT", "fail")
	run := e.waitRun(e.startGraph(t, checkedGraph(func(check, _ *runs.WorkflowConfig) {
		check.RetryFrom, check.MaxAttempts = "gen", 2
	})).ID, "failed")
	if !strings.HasPrefix(run.Error, "Verify:") || !strings.Contains(run.Error, "check failed") {
		t.Fatalf("%q", run.Error)
	}
	if n := len(e.calls()); n != 4 {
		t.Fatalf("calls = %d, want two attempts of two steps", n)
	}
	history, err := e.q.ListWorkflowCheckAttempts(e.ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].CheckResult != "failed" || history[1].CheckResult != "failed" {
		t.Fatalf("%+v", history)
	}
	if _, reached := e.steps(run.ID)["ok"]; reached {
		t.Fatal("a gate not tied to the check opened")
	}
	e.notDelivered(t, run)
}

// A missing or unreadable verdict fails the check, whatever the text says.
func TestWorkflowCheckNeedsAReadableVerdict(t *testing.T) {
	for _, verdict := range []string{"missing", "PASS!!"} {
		t.Run(verdict, func(t *testing.T) {
			e := newWFImageEnv(t)
			t.Setenv("FAKE_CHECK_VERDICT", verdict)
			run := e.waitRun(e.startGraph(t, checkedGraph(func(*runs.WorkflowConfig, *runs.WorkflowConfig) {})).ID, "failed")
			s := e.steps(run.ID)["check"]
			if s.CheckResult != "failed" || !strings.Contains(s.Note, "readable") || !strings.HasPrefix(s.Output, "PASS") {
				t.Fatalf("%+v", s)
			}
			e.notDelivered(t, run)
		})
	}
}

// Approve is refused while the tied check has failed. Request changes works
// and a passing re-run unlocks it.
func TestWorkflowGateTiedToAFailedCheck(t *testing.T) {
	e := newWFImageEnv(t)
	t.Setenv("FAKE_IMAGE_CONTENT", "bad")
	t.Setenv("FAKE_CHECK_VERDICT", "fail")
	run := e.startGraph(t, checkedGraph(func(_, gate *runs.WorkflowConfig) { gate.RetryFrom, gate.RequiresCheck = "gen", "check" }))
	e.waitGate(run.ID, 1)

	var conflict *apperr.ConflictError
	if err := decide(e, run, "approve", ""); !errors.As(err, &conflict) || !strings.Contains(err.Error(), "Verify didn't pass") {
		t.Fatalf("err = %v", err)
	}
	if s := e.steps(run.ID)["ok"]; s.State != "waiting" {
		t.Fatalf("%+v", s)
	}
	e.notDelivered(t, run)

	t.Setenv("FAKE_CHECK_VERDICT", "")
	if err := decide(e, run, "changes", "Add a lava river"); err != nil {
		t.Fatal(err)
	}
	e.waitGate(run.ID, 2)
	if s := e.steps(run.ID)["check"]; s.CheckResult != "passed" {
		t.Fatalf("%+v", s)
	}
	if err := decide(e, run, "approve", ""); err != nil {
		t.Fatal(err)
	}
	e.waitRun(run.ID, "done")
}

func TestWorkflowGateTiedToAFailedCheckCanBeRejected(t *testing.T) {
	e := newWFImageEnv(t)
	t.Setenv("FAKE_CHECK_VERDICT", "fail")
	run := e.startGraph(t, checkedGraph(func(_, gate *runs.WorkflowConfig) { gate.RetryFrom, gate.RequiresCheck = "gen", "check" }))
	e.waitGate(run.ID, 1)
	if err := decide(e, run, "reject", ""); err != nil {
		t.Fatal(err)
	}
	e.notDelivered(t, e.waitRun(run.ID, "stopped"))
}

// With Approve refused and no way to re-run, the run ends instead of waiting
// for a Reject that could be the only choice.
func TestWorkflowGateWithNoWayPastAFailedCheckFailsTheRun(t *testing.T) {
	e := newWFImageEnv(t)
	t.Setenv("FAKE_CHECK_VERDICT", "fail")
	run := e.waitRun(e.startGraph(t, checkedGraph(func(_, gate *runs.WorkflowConfig) { gate.RequiresCheck = "check" })).ID, "failed")
	if !strings.Contains(run.Error, "Verify didn't pass") {
		t.Fatalf("%q", run.Error)
	}
	if s := e.steps(run.ID)["ok"]; s.State != "failed" {
		t.Fatalf("%+v", s)
	}
	e.notDelivered(t, run)
}
