package runs_test

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runs"
)

func TestContractNormalizeAndPaths(t *testing.T) {
	c, err := runs.Contract{AcceptanceCriteria: []string{" a ", ""}, AllowedPaths: []string{"internal/runs/", "docs/*.md", "go.mod"}, TestCommand: " go test ./... ", BaseSHA: "x", PrevHeadSHA: "y", Diverged: true}.Normalize()
	if err != nil || len(c.AcceptanceCriteria) != 1 || c.AcceptanceCriteria[0] != "a" || c.TestCommand != "go test ./..." || c.BaseSHA != "" || c.PrevHeadSHA != "" || c.Diverged {
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
	plain := runs.ChildPrompt("PAY-1", "atlas", "Do it", runs.Contract{}, true)
	if strings.Contains(plain, "Acceptance criteria") || strings.Contains(plain, "Allowed files") || strings.Contains(plain, "## Tests") {
		t.Fatalf("empty contract rendered sections:\n%s", plain)
	}
	p := runs.ChildPrompt("PAY-1", "atlas", "Do it", runs.Contract{AcceptanceCriteria: []string{"it works"}, AllowedPaths: []string{"docs/"}, TestCommand: "make test"}, true)
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
	// A sub agent that can't write is told so, and isn't told to commit.
	ro := runs.ChildPrompt("PAY-1", "atlas", "Review it", runs.Contract{}, false)
	if !strings.Contains(ro, "You can't change files in this run (repo:write is not allowed)") || strings.Contains(ro, "commit") {
		t.Errorf("read-only prompt:\n%s", ro)
	}
	if strings.Contains(plain, "can't change files") {
		t.Errorf("writable prompt says read-only:\n%s", plain)
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

func TestContractGlobs(t *testing.T) {
	c := runs.Contract{AllowedPaths: []string{"*_test.go", "**/testdata/*.json", "docs/*.md", "./web/"}}
	for file, want := range map[string]bool{
		"x_test.go": true, "internal/runs/x_test.go": true, "internal/runs/x.go": false, "x_test.go.orig": false,
		"testdata/a.json": true, "internal/runs/testdata/a.json": true, "internal/runs/testdata/sub/a.json": false, "internal/testdatax/a.json": false,
		"docs/a.md": true, "docs/sub/a.md": false, "web/app.tsx": true, "./web/app.tsx": true,
	} {
		if got := c.PathAllowed(file); got != want {
			t.Errorf("PathAllowed(%q) = %v, want %v", file, got, want)
		}
	}
	// A glob without a "/" may match in any directory, so it overlaps everything;
	// a plain root file name does not.
	if !runs.PathsOverlap([]string{"*_test.go"}, []string{"web/"}) || !runs.PathsOverlap([]string{"**/testdata/*.json"}, []string{"docs/"}) {
		t.Error("directory-less patterns should overlap any region")
	}
	if runs.PathsOverlap([]string{"docs/*.md"}, []string{"web/"}) {
		t.Error("disjoint directories overlap")
	}
	if (runs.Contract{AllowedPaths: []string{"go.mod"}}).PathAllowed("sub/go.mod") || runs.PathsOverlap([]string{"go.mod"}, []string{"web/"}) {
		t.Error("a plain name matched outside the root")
	}
}

func TestChecksRepro(t *testing.T) {
	zero, one := 0, 1
	contract := runs.Contract{AllowedPaths: []string{"*_test.go"}, ReproCommand: "go test -run X"}
	base := runs.Checks{Ran: true, ChildState: "completed", ChangedFiles: []string{"a/x_test.go"}, ReproCommand: "go test -run X"}
	failing := base
	failing.ReproExit, failing.ReproTail = &one, "FAIL X"
	failing.Evaluate(contract)
	failing.Merged = true
	if r := failing.Report(); !failing.Passed || !strings.Contains(r, "The reviewer's failing tests are merged into your branch. `go test -run X` fails now (exit 1)") || !strings.Contains(r, "FAIL X") || !strings.Contains(r, "recheck_review") {
		t.Fatalf("failing repro: %+v\n%s", failing, r)
	}
	for name, exit := range map[string]*int{"passes": &zero, "timed out": nil} {
		k := base
		k.ReproExit = exit
		k.Evaluate(contract)
		if k.Passed || !strings.Contains(k.Report(), "doesn't demonstrate the findings; the branch was not merged") {
			t.Errorf("%s: passed=%v\n%s", name, k.Passed, k.Report())
		}
	}
	// A lead can't set the reviewer's fields through delegate.
	if c, err := contract.Normalize(); err != nil || c.ReproCommand != "" || c.ReproCoversAll {
		t.Fatalf("normalize kept repro fields: %+v %v", c, err)
	}
}

func TestChecksBounceable(t *testing.T) {
	one, zero := 1, 0
	base := func() runs.Checks { return runs.Checks{Ran: true, ChildState: "completed"} }
	for name, tc := range map[string]struct {
		k    func() runs.Checks
		want bool
	}{
		"uncommitted":     {func() runs.Checks { k := base(); k.Uncommitted = []string{"a.go"}; return k }, true},
		"out of bounds":   {func() runs.Checks { k := base(); k.OutOfBounds = []string{"x/a.go"}; return k }, true},
		"failing tests":   {func() runs.Checks { k := base(); k.TestCommand, k.TestExit = "go test", &one; return k }, true},
		"tests timed out": {func() runs.Checks { k := base(); k.TestCommand = "go test"; return k }, true},
		"passing":         {func() runs.Checks { k := base(); k.Passed = true; return k }, false},
		"checks broken": {func() runs.Checks {
			k := base()
			k.Error = "git status failed"
			k.TestCommand, k.TestExit = "go test", &one
			return k
		}, false},
		"not run": {func() runs.Checks { return runs.Checks{ChildState: "completed", Uncommitted: []string{"a.go"}} }, false},
		// A reviewer's committed failing tests are expected to fail.
		"reviewer's failing tests": {func() runs.Checks {
			k := base()
			k.ReproCommand, k.ReproExit, k.TestCommand, k.TestExit = "go test -run Bug", &one, "go test", &one
			return k
		}, false},
		"reviewer's repro already passes": {func() runs.Checks { k := base(); k.ReproCommand, k.ReproExit = "go test -run Bug", &zero; return k }, false},
		"reviewer's uncommitted tests": {func() runs.Checks {
			k := base()
			k.ReproCommand, k.ReproExit, k.Uncommitted = "go test -run Bug", &one, []string{"bug_test.go"}
			return k
		}, true},
		// A reviewer or QA (a verdict) doesn't own the code its test command tests.
		"verdict: failing tests":    {func() runs.Checks { k := base(); k.TestCommand, k.TestExit = "go test", &one; return k }, false},
		"verdict: uncommitted file": {func() runs.Checks { k := base(); k.Uncommitted = []string{"notes.md"}; return k }, true},
	} {
		if got := tc.k().Bounceable(strings.HasPrefix(name, "verdict: ")); got != tc.want {
			t.Errorf("%s: Bounceable = %v, want %v", name, got, tc.want)
		}
	}
}
