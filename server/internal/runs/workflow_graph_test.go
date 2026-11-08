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
	got := ids(map[string]workflowStepView{"start": {"done"}, "brief": {"done"}, "work": {"running"}})
	if len(got) != 0 {
		t.Fatalf("running parent: %v", got)
	}
	got = ids(map[string]workflowStepView{"start": {"done"}, "brief": {"stale"}, "work": {"stale"}, "ok": {"stale"}})
	if !slices.Equal(got, []string{"brief"}) {
		t.Fatalf("after request changes: %v", got)
	}
}

func TestRenderWorkflowTemplate(t *testing.T) {
	got := renderWorkflowTemplate("{{ input }}/{{a.output}}/{{b}}", map[string]string{"input": "i", "a": "x"})
	if got != "i/x/" {
		t.Fatal(got)
	}
}
