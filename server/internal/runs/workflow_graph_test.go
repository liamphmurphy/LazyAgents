package runs

import (
	"slices"
	"strings"
	"testing"
)

func testWorkflow() WorkflowGraph {
	return WorkflowGraph{
		Nodes: []WorkflowNode{
			{ID: "start", Type: NodeTrigger, Title: "Start"},
			{ID: "brief", Type: NodeEnrich, Title: "Brief"},
			{ID: "work", Type: NodeAgent, Title: "Work", Config: WorkflowConfig{Prompt: "Do {{brief.output}} for {{input}}"}},
			{ID: "ok", Type: NodeGate, Title: "OK", Config: WorkflowConfig{RetryFrom: "brief", Show: []string{"work"}}},
			{ID: "page", Type: NodeWiki, Title: "Page", Config: WorkflowConfig{Path: "Notes/Result"}},
		},
		Edges: []WorkflowEdge{{"start", "brief"}, {"brief", "work"}, {"work", "ok"}, {"ok", "page"}},
	}
}

func hasIssue(issues []WorkflowIssue, want string) bool {
	return slices.ContainsFunc(issues, func(i WorkflowIssue) bool { return strings.Contains(i.Message, want) })
}

func TestValidateWorkflowAcceptsAGatedDAG(t *testing.T) {
	if issues := ValidateWorkflow(testWorkflow()); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestValidateWorkflowRejects(t *testing.T) {
	cases := map[string]struct {
		edit func(*WorkflowGraph)
		want string
	}{
		"cycle": {func(g *WorkflowGraph) { g.Edges = append(g.Edges, WorkflowEdge{"work", "brief"}) }, "cycle"},
		"no gate before wiki": {func(g *WorkflowGraph) {
			g.Nodes = slices.DeleteFunc(g.Nodes, func(n WorkflowNode) bool { return n.Type == NodeGate })
			g.Edges = []WorkflowEdge{{"start", "brief"}, {"brief", "work"}, {"work", "page"}}
		}, "human gate"},
		"unknown variable":    {func(g *WorkflowGraph) { g.Nodes[2].Config.Prompt = "{{ghost.output}}" }, "{{ghost}}"},
		"downstream variable": {func(g *WorkflowGraph) { g.Nodes[1].Config.Prompt = "{{work.output}}" }, "{{work}}"},
		"two triggers": {func(g *WorkflowGraph) {
			g.Nodes = append(g.Nodes, WorkflowNode{ID: "again", Type: NodeTrigger, Title: "Again"})
		}, "exactly one trigger"},
		"retry target after gate": {func(g *WorkflowGraph) { g.Nodes[3].Config.RetryFrom = "page" }, "re-run"},
		"output has a child":      {func(g *WorkflowGraph) { g.Edges = append(g.Edges, WorkflowEdge{"page", "ok"}) }, "cycle"},
		"disconnected":            {func(g *WorkflowGraph) { g.Edges = g.Edges[:3] }, "Connect"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := testWorkflow()
			c.edit(&g)
			if issues := ValidateWorkflow(g); !hasIssue(issues, c.want) {
				t.Fatalf("want %q in %+v", c.want, issues)
			}
		})
	}
}

func TestApplyWorkflowOps(t *testing.T) {
	g := testWorkflow()
	out, err := ApplyWorkflowOps(g, []WorkflowOp{
		{Op: "add_node", Node: &WorkflowNode{ID: "check", Type: NodeAgent, Title: "Check", Config: WorkflowConfig{Prompt: "Check"}}},
		{Op: "remove_edge", From: "work", To: "ok"},
		{Op: "add_edge", From: "work", To: "check"},
		{Op: "add_edge", From: "check", To: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidateWorkflow(out); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if n := out.Nodes[len(out.Nodes)-1]; n.X == 0 {
		t.Fatalf("new node wasn't placed: %+v", n)
	}
	if len(g.Nodes) != 5 {
		t.Fatal("the original graph changed")
	}
	if _, err := ApplyWorkflowOps(g, []WorkflowOp{{Op: "remove_node", NodeID: "ghost"}}); err == nil {
		t.Fatal("removing an unknown node should fail")
	}
}

func TestWorkflowReady(t *testing.T) {
	g := testWorkflow()
	ids := func(latest map[string]workflowStepView) []string {
		var out []string
		for _, n := range workflowReady(g, latest) {
			out = append(out, n.ID)
		}
		return out
	}
	if got := ids(map[string]workflowStepView{}); !slices.Equal(got, []string{"start"}) {
		t.Fatalf("empty: %v", got)
	}
	got := ids(map[string]workflowStepView{"start": {State: "done"}, "brief": {State: "done"}, "work": {State: "running"}})
	if len(got) != 0 {
		t.Fatalf("running parent: %v", got)
	}
	got = ids(map[string]workflowStepView{"start": {State: "done"}, "brief": {State: "stale"}, "work": {State: "stale"}, "ok": {State: "stale"}})
	if !slices.Equal(got, []string{"brief"}) {
		t.Fatalf("after request changes: %v", got)
	}
}

// The shape DnD Map Generator migrates to: verify is the check, approve is tied to it.
func checkedWorkflow() WorkflowGraph {
	return WorkflowGraph{
		Nodes: []WorkflowNode{
			{ID: "start", Type: NodeTrigger, Title: "Start"},
			{ID: "generate", Type: NodeAgent, Title: "Generate", Config: WorkflowConfig{Prompt: "Draw", Images: true}},
			{ID: "verify", Type: NodeAgent, Title: "Verify", Config: WorkflowConfig{Prompt: "Check", Check: true, RetryFrom: "generate", MaxAttempts: 2}},
			{ID: "approve", Type: NodeGate, Title: "Approve", Config: WorkflowConfig{RetryFrom: "generate", RequiresCheck: "verify"}},
			{ID: "library", Type: NodeLibrary, Title: "Save", Config: WorkflowConfig{Title: "Map"}},
		},
		Edges: []WorkflowEdge{{"start", "generate"}, {"generate", "verify"}, {"verify", "approve"}, {"approve", "library"}},
	}
}

func TestValidateWorkflowChecks(t *testing.T) {
	if issues := ValidateWorkflow(checkedWorkflow()); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	cases := map[string]struct {
		edit func(*WorkflowGraph)
		want string
	}{
		"check on a gate":         {func(g *WorkflowGraph) { g.Nodes[3].Config.Check = true }, "Only agent steps"},
		"check generates images":  {func(g *WorkflowGraph) { g.Nodes[2].Config.Images = true }, "can't also generate"},
		"retry from itself":       {func(g *WorkflowGraph) { g.Nodes[2].Config.RetryFrom = "verify" }, "failed check must re-run"},
		"retry from a later node": {func(g *WorkflowGraph) { g.Nodes[2].Config.RetryFrom = "approve" }, "failed check must re-run"},
		"too many attempts":       {func(g *WorkflowGraph) { g.Nodes[2].Config.MaxAttempts = 6 }, "Attempts must be"},
		"tied to a plain step":    {func(g *WorkflowGraph) { g.Nodes[3].Config.RequiresCheck = "generate" }, "tied to a check"},
		"tied to no such node":    {func(g *WorkflowGraph) { g.Nodes[3].Config.RequiresCheck = "ghost" }, "tied to a check"},
		"tied on an agent":        {func(g *WorkflowGraph) { g.Nodes[1].Config.RequiresCheck = "verify" }, "Only gates"},
		"tied check isn't a parent": {func(g *WorkflowGraph) {
			g.Nodes = append(g.Nodes, WorkflowNode{ID: "polish", Type: NodeAgent, Title: "Polish", Config: WorkflowConfig{Prompt: "Polish"}})
			g.Edges = []WorkflowEdge{{"start", "generate"}, {"generate", "verify"}, {"verify", "polish"}, {"polish", "approve"}, {"approve", "library"}}
		}, "connected directly"},
		"changes skip the check": {func(g *WorkflowGraph) {
			g.Nodes = append(g.Nodes, WorkflowNode{ID: "extra", Type: NodeAgent, Title: "Extra", Config: WorkflowConfig{Prompt: "More"}})
			g.Edges = append(g.Edges, WorkflowEdge{"verify", "extra"}, WorkflowEdge{"extra", "approve"})
			g.Nodes[3].Config.RetryFrom = "extra"
		}, "could never unlock"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := checkedWorkflow()
			c.edit(&g)
			if issues := ValidateWorkflow(g); !hasIssue(issues, c.want) {
				t.Fatalf("want %q in %+v", c.want, issues)
			}
		})
	}
	// propose_workflow_edit validates through the same path.
	if _, err := proposedWorkflow(checkedWorkflow(), []WorkflowOp{{Op: "update_node", Node: &WorkflowNode{ID: "approve", Type: NodeGate, Title: "Approve", Config: WorkflowConfig{RequiresCheck: "generate"}}}}); err == nil || !strings.Contains(err.Error(), "tied to a check") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkflowReadyHoldsBackAFailedCheck(t *testing.T) {
	ready := func(g WorkflowGraph, check string) []string {
		var out []string
		for _, n := range workflowReady(g, map[string]workflowStepView{"start": {State: "done"}, "generate": {State: "done"}, "verify": {State: "done", Check: check}}) {
			out = append(out, n.ID)
		}
		return out
	}
	g := checkedWorkflow()
	for _, check := range []string{"failed", ""} {
		if got := ready(g, check); !slices.Equal(got, []string{"approve"}) {
			t.Fatalf("%q: a tied gate opens: %v", check, got)
		}
	}
	// A gate not tied to the check, and any output, stays shut.
	g.Nodes[3].Config.RequiresCheck = ""
	for _, check := range []string{"failed", ""} {
		if got := ready(g, check); len(got) != 0 {
			t.Fatalf("%q: %v", check, got)
		}
	}
	if got := ready(g, "passed"); !slices.Equal(got, []string{"approve"}) {
		t.Fatalf("passed: %v", got)
	}
	g = checkedWorkflow()
	g.Nodes = append(g.Nodes, WorkflowNode{ID: "post", Type: NodeChat, Title: "Post"})
	g.Edges = append(g.Edges, WorkflowEdge{"verify", "post"})
	if got := ready(g, "failed"); !slices.Equal(got, []string{"approve"}) {
		t.Fatalf("an output after a failed check ran: %v", got)
	}
}

func TestRenderWorkflowTemplate(t *testing.T) {
	got := renderWorkflowTemplate("{{ input }}/{{a.output}}/{{b}}", map[string]string{"input": "i", "a": "x"})
	if got != "i/x/" {
		t.Fatal(got)
	}
}
