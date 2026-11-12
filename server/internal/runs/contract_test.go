package runs_test

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runs"
)

func TestContractNormalizeAndPaths(t *testing.T) {
	c, err := runs.Contract{AcceptanceCriteria: []string{" a ", ""}, AllowedPaths: []string{"internal/runs/", "docs/*.md", "go.mod"}, TestCommand: " go test ./... ", BaseSHA: "x"}.Normalize()
	if err != nil || len(c.AcceptanceCriteria) != 1 || c.AcceptanceCriteria[0] != "a" || c.TestCommand != "go test ./..." || c.BaseSHA != "" {
		t.Fatalf("normalize: %+v %v", c, err)
	}
	for file, want := range map[string]bool{
		"internal/runs/x.go": true, "internal/runs/sub/x.go": true, "internal/runsx/x.go": false,
		"docs/a.md": true, "docs/sub/a.md": false, "go.mod": true, "go.sum": false,
	} {
		if got := c.PathAllowed(file); got != want {
			t.Errorf("PathAllowed(%q) = %v, want %v", file, got, want)
		}
	}
	if !(runs.Contract{}).PathAllowed("anything") {
		t.Error("an empty contract should allow every path")
	}
	for _, bad := range []string{"/etc/passwd", "../x", "a/[b"} {
		if _, err := (runs.Contract{AllowedPaths: []string{bad}}).Normalize(); err == nil {
			t.Errorf("accepted allowed path %q", bad)
		}
	}
}

func TestChildPromptRendersContract(t *testing.T) {
	plain := runs.ChildPrompt("PAY-1", "atlas", "Do it", runs.Contract{})
	if strings.Contains(plain, "Acceptance criteria") || strings.Contains(plain, "Allowed files") || strings.Contains(plain, "## Tests") {
		t.Fatalf("empty contract rendered sections:\n%s", plain)
	}
	p := runs.ChildPrompt("PAY-1", "atlas", "Do it", runs.Contract{AcceptanceCriteria: []string{"it works"}, AllowedPaths: []string{"docs/"}, TestCommand: "make test"})
	for _, want := range []string{"## Task\n\nDo it", "- it works", "- docs/", "`make test`", "@atlas delegated part of PAY-1", "own git worktree", "in parallel", "AGENTS.md"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
	// Lifecycle rules live in the sub agent's AGENTS.md only.
	for _, bad := range []string{"claim_ticket", "complete_run", "get_ticket", "blocked"} {
		if strings.Contains(plain, bad) {
			t.Errorf("prompt restates %q:\n%s", bad, plain)
		}
	}
}

func TestChecksEvaluateAndReport(t *testing.T) {
	zero, one := 0, 1
	contract := runs.Contract{AllowedPaths: []string{"docs/"}, TestCommand: "make test"}
	ok := runs.Checks{Ran: true, ChildState: "completed", ChangedFiles: []string{"docs/a.md"}, TestCommand: "make test", TestExit: &zero}
	ok.Evaluate(contract)
	if !ok.Passed || !strings.Contains(ok.Report(), "**PASSED**") {
		t.Fatalf("expected pass: %+v\n%s", ok, ok.Report())
	}
	cases := map[string]runs.Checks{
		"out of bounds":   {Ran: true, ChildState: "completed", ChangedFiles: []string{"main.go"}, TestCommand: "make test", TestExit: &zero},
		"uncommitted":     {Ran: true, ChildState: "completed", Uncommitted: []string{"docs/a.md"}, TestCommand: "make test", TestExit: &zero},
		"test failed":     {Ran: true, ChildState: "completed", TestCommand: "make test", TestExit: &one, TestTail: "boom"},
		"test unfinished": {Ran: true, ChildState: "completed", TestCommand: "make test"},
		"child failed":    {Ran: true, ChildState: "failed", TestCommand: "make test", TestExit: &zero},
		"partial":         {Ran: true, Error: "no base", ChildState: "completed", TestCommand: "make test", TestExit: &zero},
		"not run":         {ChildState: "completed", Error: "git status failed"},
	}
	for name, k := range cases {
		k.Evaluate(contract)
		if k.Passed || !strings.Contains(k.Report(), "Automated checks") || strings.Contains(k.Report(), "**PASSED**") {
			t.Errorf("%s: passed=%v\n%s", name, k.Passed, k.Report())
		}
	}
	if r := (func() string { k := cases["test failed"]; k.Evaluate(contract); return k.Report() })(); !strings.Contains(r, "boom") || !strings.Contains(r, "exit 1") {
		t.Errorf("failure output missing:\n%s", r)
	}
}
