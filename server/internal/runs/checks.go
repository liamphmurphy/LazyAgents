package runs

import (
	"fmt"
	"slices"
	"strings"
)

// Checks is what the runner measured about a delegated run's result before
// waking the main agent. It's stored on the child run and rendered into the
// main agent's resume message.
type Checks struct {
	HeadSHA      string   `json:"head_sha,omitempty"`
	ReportedTurn int      `json:"reported_turn,omitempty"`
	Ran          bool     `json:"ran"`
	Error        string   `json:"error,omitempty"` // why the checks couldn't run
	ChildState   string   `json:"child_state"`
	BaseSHA      string   `json:"base_sha,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Uncommitted  []string `json:"uncommitted,omitempty"`
	OutOfBounds  []string `json:"out_of_bounds,omitempty"`
	DiffStat     string   `json:"diff_stat,omitempty"`
	TestCommand  string   `json:"test_command,omitempty"`
	TestExit     *int     `json:"test_exit,omitempty"`
	TestTail     string   `json:"test_tail,omitempty"`
	Passed       bool     `json:"passed"`
	// Branch is the child's git branch; Merged says the runner merged it into
	// the parent's branch. When it didn't, MergeConflicts lists the files that
	// conflicted (the merge was aborted) and MergeNote says why it was skipped
	// or failed. An unmerged branch is kept for the main agent to inspect.
	Branch         string   `json:"branch,omitempty"`
	Merged         bool     `json:"merged"`
	MergeConflicts []string `json:"merge_conflicts,omitempty"`
	MergeNote      string   `json:"merge_note,omitempty"`
}

// Evaluate fills the derived fields from the measured ones.
func (k *Checks) Evaluate(contract Contract) {
	k.OutOfBounds = nil
	for _, f := range append(slices.Clone(k.ChangedFiles), k.Uncommitted...) {
		if !contract.PathAllowed(f) && !slices.Contains(k.OutOfBounds, f) {
			k.OutOfBounds = append(k.OutOfBounds, f)
		}
	}
	k.Passed = k.Ran && k.ChildState == "completed" && len(k.OutOfBounds) == 0 && len(k.Uncommitted) == 0 &&
		k.Error == "" && (k.TestCommand == "" || (k.TestExit != nil && *k.TestExit == 0))
}

const maxListed = 30

func list(files []string) string {
	var b strings.Builder
	for i, f := range files {
		if i == maxListed {
			fmt.Fprintf(&b, "- … and %d more\n", len(files)-maxListed)
			break
		}
		b.WriteString("- " + f + "\n")
	}
	return b.String()
}

// Report is the text appended to the main agent's resume message.
func (k Checks) Report() string {
	var b strings.Builder
	b.WriteString("## Automated checks\n\n")
	if !k.Ran {
		b.WriteString("The server couldn't run the checks")
		if k.Error != "" {
			b.WriteString(": " + k.Error)
		}
		b.WriteString(". Verify the result yourself (git status, git diff, tests).\n")
		k.writeMerge(&b)
		return b.String()
	}
	if k.Error != "" {
		b.WriteString("Partial: " + k.Error + ".\n\n")
	}
	if k.Passed {
		b.WriteString("**PASSED**: ")
	} else {
		b.WriteString("**FAILED**: ")
	}
	fmt.Fprintf(&b, "sub agent run %s; %d file(s) changed since it started.\n", k.ChildState, len(k.ChangedFiles))
	if k.DiffStat != "" {
		b.WriteString("\n```\n" + k.DiffStat + "\n```\n")
	}
	if len(k.Uncommitted) > 0 {
		b.WriteString("\nUncommitted changes (the sub agent didn't commit them):\n" + list(k.Uncommitted))
	}
	if len(k.OutOfBounds) > 0 {
		b.WriteString("\nChanged outside the allowed paths:\n" + list(k.OutOfBounds))
	}
	if k.TestCommand != "" {
		switch {
		case k.TestExit == nil:
			fmt.Fprintf(&b, "\nTest command `%s` did not finish.\n", k.TestCommand)
		case *k.TestExit == 0:
			fmt.Fprintf(&b, "\nTest command `%s` passed.\n", k.TestCommand)
		default:
			fmt.Fprintf(&b, "\nTest command `%s` failed (exit %d):\n```\n%s\n```\n", k.TestCommand, *k.TestExit, k.TestTail)
		}
	}
	if k.Passed {
		b.WriteString("\nThese checks don't judge correctness against the acceptance criteria: review the diff for that.\n")
	}
	k.writeMerge(&b)
	return b.String()
}

func (k Checks) writeMerge(b *strings.Builder) {
	switch {
	case k.Merged:
		fmt.Fprintf(b, "\n**MERGED** branch `%s` into your branch (git merge --no-ff).\n", k.Branch)
	case len(k.MergeConflicts) > 0:
		fmt.Fprintf(b, "\n**NOT MERGED**: merging branch `%s` conflicted (the merge was aborted, your branch is unchanged). Conflicting files:\n%s"+
			"Resolve it yourself with `git merge %s`, or delegate again with continue_run_id to have the sub agent rebase its work.\n", k.Branch, list(k.MergeConflicts), k.Branch)
	case k.Branch != "":
		fmt.Fprintf(b, "\n**NOT MERGED**: branch `%s` was kept. %s Inspect it (git diff, git log) and merge it with `git merge %s` if it's good, or send fixes back with continue_run_id.\n", k.Branch, k.MergeNote, k.Branch)
	}
}
