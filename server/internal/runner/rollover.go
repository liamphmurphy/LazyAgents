package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// rolloverIntro opens the prompt of a turn that continues in a fresh session.
const rolloverIntro = "Your earlier session was closed to keep context small; this brief is your state."

// rolloverTaskRunes bounds each delegated task quoted in a rollover brief.
const rolloverTaskRunes = 1500

// shouldRollOver reports whether a main agent woken by its delegations should
// continue in a fresh session: the setting is on, it has a session to drop,
// and its last recorded context is at least the setting. An unknown context
// size (0, e.g. Codex) never rolls over.
func (r *Runner) shouldRollOver(c runCtx) bool {
	limit := r.Cfg.RolloverContextTokens
	return limit > 0 && c.run.HarnessSessionID != nil && c.run.ContextTokens > 0 && c.run.ContextTokens >= limit
}

// rolloverPrompt briefs a fresh session at a delegation wake: the run's state,
// its git facts, what each child reported this turn was asked to do, then
// wake (the wake message and the delegation report).
func (r *Runner) rolloverPrompt(ctx context.Context, c runCtx, seq int32, wake string, env map[string]string) (string, error) {
	brief, err := runs.RunBrief(ctx, r.q(), c.run)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(rolloverIntro + "\n\n" + brief)
	if git := r.gitBrief(ctx, c, env); git != "" {
		b.WriteString("\n" + git)
	}
	delegated, err := r.delegatedTasks(ctx, c.run, seq)
	if err != nil {
		return "", err
	}
	if delegated != "" {
		b.WriteString("\n## What you delegated\n" + delegated)
	}
	b.WriteString("\n## Delegation report\n\n" + strings.TrimLeft(wake, "\n"))
	return b.String(), nil
}

// delegatedTasks lists the task, acceptance criteria and test command of each
// child whose checks were reported in turn seq.
func (r *Runner) delegatedTasks(ctx context.Context, parent store.Run, seq int32) (string, error) {
	kids, err := r.q().ChildRuns(ctx, &parent.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, k := range kids {
		var checks runs.Checks
		if len(k.Checks) == 0 || json.Unmarshal(k.Checks, &checks) != nil || checks.ReportedTurn != int(seq) {
			continue
		}
		var contract runs.Contract
		if len(k.Contract) > 0 {
			_ = json.Unmarshal(k.Contract, &contract)
		}
		turns, err := r.q().ListTurns(ctx, k.ID)
		if err != nil {
			return "", err
		}
		task := ""
		if len(turns) > 0 {
			task = childTask(turns[0].InputMessage, contract)
		}
		handle := k.AgentID.String()
		if a, err := r.q().GetAgent(ctx, store.GetAgentParams{ID: k.AgentID, WorkspaceID: k.WorkspaceID}); err == nil {
			handle = "@" + a.Handle
		}
		fmt.Fprintf(&b, "\n### %s (run %s, %s)\n\n%s\n", handle, k.ID.String()[:8], k.State, clipTask(task))
		if len(contract.AcceptanceCriteria) > 0 {
			b.WriteString("\nAcceptance criteria:\n")
			for _, a := range contract.AcceptanceCriteria {
				b.WriteString("- " + a + "\n")
			}
		}
		if contract.TestCommand != "" {
			b.WriteString("\nTest command: `" + contract.TestCommand + "`\n")
		}
	}
	return b.String(), nil
}

// childTask recovers the task a delegated run was given from its first
// message (runs.ChildPrompt): the text after "## Task", without the sections
// the contract appended. A message without that heading is returned whole.
func childTask(input string, contract runs.Contract) string {
	const heading = "\n\n## Task\n\n"
	_, task, ok := strings.Cut(input, heading)
	if !ok {
		return strings.TrimSpace(input)
	}
	_, suffix, _ := strings.Cut(runs.ChildPrompt("", "", "", contract, true), heading)
	return strings.TrimSpace(strings.TrimSuffix(task, suffix))
}

// clipTask shortens a delegated task to rolloverTaskRunes, marking the cut.
func clipTask(task string) string {
	if clipped := clipRunes(task, rolloverTaskRunes); clipped != task {
		return clipped + "\n[truncated]"
	}
	return task
}
