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
	// Integration is "passed" or "failed" when, after this run was merged, the
	// test commands of every delegated run merged in the same report were run
	// on the combined checkout; "" when it was the only one merged.
	// IntegrationCommand is the command that failed, IntegrationTail its output.
	Integration        string `json:"integration,omitempty"`
	IntegrationCommand string `json:"integration_command,omitempty"`
	IntegrationTail    string `json:"integration_tail,omitempty"`
	// ReproCommand is the reviewer's repro_command, run in its worktree: it
	// must fail (ReproExit non-zero) for the branch to merge. ReproExit is nil
	// when it didn't finish.
	ReproCommand string `json:"repro_command,omitempty"`
	ReproExit    *int   `json:"repro_exit,omitempty"`
	ReproTail    string `json:"repro_tail,omitempty"`
	// RecheckRequested is set by recheck_review until the runner has run
	// ReproCommand in the main agent's checkout. ReproFixed says it passed there,
	// at RecheckSHA, in the turn RecheckTurn; RecheckExit and RecheckTail are
	// that run's result (RecheckExit nil when it didn't finish).
	RecheckRequested bool   `json:"recheck_requested,omitempty"`
	RecheckTurn      int    `json:"recheck_turn,omitempty"`
	RecheckSHA       string `json:"recheck_sha,omitempty"`
	RecheckExit      *int   `json:"recheck_exit,omitempty"`
	RecheckTail      string `json:"recheck_tail,omitempty"`
	ReproFixed       bool   `json:"repro_fixed,omitempty"`
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
		k.Error == "" && (k.TestCommand == "" || (k.TestExit != nil && *k.TestExit == 0)) &&
		(k.ReproCommand == "" || (k.ReproExit != nil && *k.ReproExit != 0))
}

// RecheckReport is the "## Review recheck" section of the main agent's resume
// message after recheck_review.
// coversAll is the reviewer's repro_covers_all.
func (k Checks) RecheckReport(runID string, coversAll bool) string {
	var b strings.Builder
	b.WriteString("## Review recheck\n\n")
	switch {
	case k.ReproFixed && coversAll:
		fmt.Fprintf(&b, "**PASSED**: `%s` passes on your branch at `%s`, so the findings of review run %s are verified fixed; its changes_requested verdict no longer blocks Done.\n", k.ReproCommand, k.RecheckSHA, runID)
	case k.ReproFixed:
		fmt.Fprintf(&b, "**PASSED**: `%s` passes on your branch at `%s`. Review run %s didn't say its tests cover every blocking finding, so its changes_requested verdict still needs a re-review (continue_run_id) before Done.\n", k.ReproCommand, k.RecheckSHA, runID)
	case k.RecheckExit == nil:
		fmt.Fprintf(&b, "**FAILED**: `%s` did not finish on your branch", k.ReproCommand)
		if k.RecheckTail != "" {
			b.WriteString(":\n```\n" + k.RecheckTail + "\n```\n")
		} else {
			b.WriteString(".\n")
		}
		b.WriteString("Fix it, commit, and call recheck_review again.\n")
	default:
		fmt.Fprintf(&b, "**FAILED**: `%s` still fails on your branch at `%s` (exit %d):\n```\n%s\n```\nFix it, commit, and call recheck_review again.\n", k.ReproCommand, k.RecheckSHA, *k.RecheckExit, k.RecheckTail)
	}
	return b.String()
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
	if k.ReproCommand != "" {
		switch {
		case k.ReproExit == nil:
			fmt.Fprintf(&b, "\nThe reviewer's repro `%s` did not finish, so it doesn't demonstrate the findings; the branch was not merged. Read the report.\n", k.ReproCommand)
		case *k.ReproExit == 0:
			fmt.Fprintf(&b, "\n`%s` already passes, so it doesn't demonstrate the findings; the branch was not merged. Read the report.\n", k.ReproCommand)
		case k.Merged:
			fmt.Fprintf(&b, "\nThe reviewer's failing tests are merged into your branch. `%s` fails now (exit %d):\n```\n%s\n```\nMake it pass, then call recheck_review.\n", k.ReproCommand, *k.ReproExit, k.ReproTail)
		default:
			fmt.Fprintf(&b, "\nThe reviewer's repro `%s` fails (exit %d), but its tests weren't merged (see below).\n", k.ReproCommand, *k.ReproExit)
		}
	}
	if k.Passed {
		b.WriteString("\nThese checks don't judge correctness against the acceptance criteria: review the diff for that.\n")
	}
	k.writeMerge(&b)
	return b.String()
}

func (k Checks) writeMerge(b *strings.Builder) {
	switch k.Integration {
	case "passed":
		b.WriteString("\nIntegration with the other delegated runs: passed.\n")
	case "failed":
		fmt.Fprintf(b, "\nIntegration with the other delegated runs: **FAILED**. `%s` failed on your branch once this run was merged with the others:\n```\n%s\n```\n", k.IntegrationCommand, k.IntegrationTail)
	}
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
