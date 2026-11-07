package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ToolDecision is consumed by both harnesses' PreToolUse hooks. An approval
// authorizes the exact input once; it never widens the frozen definition.
type ToolDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type ToolCheck struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

// ToolScopes mirrors the shared tool rules. Shell matching is deliberately
// conservative: only parsed, recognized reads avoid the tests:run scope. Hooks
// are best-effort CLI enforcement, not an OS security boundary.
func ToolScopes(tool string, input map[string]any) []string {
	switch tool {
	case "Read", "Glob", "Grep", "read_file", "list_dir":
		return []string{"repo:read"}
	case "Edit", "Write", "NotebookEdit", "apply_patch":
		return []string{"repo:write"}
	case "Bash", "shell", "exec_command", "shell_command":
		command, _ := input["command"].(string)
		if command == "" {
			command, _ = input["cmd"].(string)
		}
		return shellScopes(command, 0)
	}
	// MCP tools enforce permissions in the service, not via a shell heuristic.
	if strings.HasPrefix(tool, "mcp__") || (tool == "TodoWrite" || tool == "Skill") {
		return nil
	}
	return []string{"tests:run"} // unknown execution tools require the execution scope
}

// WebTool reports a harness's built-in web search or page fetch tool. They
// touch no workspace files, so they're governed by agentdef.OptionWebSearch
// rather than a scope.
func WebTool(tool string) bool {
	switch tool {
	case "WebSearch", "WebFetch", "web_search":
		return true
	}
	return false
}

func (s *Service) CheckTool(ctx context.Context, c Caller, in ToolCheck) (ToolDecision, error) {
	deny := func(reason string) (ToolDecision, error) { return ToolDecision{Decision: "deny", Reason: reason}, nil }
	if c.RunID == nil {
		return deny("Tool policy requires a run-scoped token")
	}
	q := store.New(s.Pool)
	run, err := q.GetRun(ctx, store.GetRunParams{ID: *c.RunID, WorkspaceID: c.Agent.WorkspaceID})
	if err != nil {
		return ToolDecision{}, err
	}
	if run.AgentID != c.Agent.ID || run.State != "running" {
		return deny("Run is not running; end this turn and wait for a person")
	}
	spec, err := Spec(run)
	if err != nil {
		return ToolDecision{}, err
	}
	if WebTool(in.Tool) && spec.HarnessOptions[agentdef.OptionWebSearch] == "true" {
		return ToolDecision{Decision: "allow"}, nil
	}
	scopes := ToolScopes(in.Tool, in.Input)
	if spec.HarnessOptions["readonly"] == "true" && len(scopes) > 0 {
		if len(scopes) == 1 && scopes[0] == "repo:read" {
			return ToolDecision{Decision: "allow"}, nil
		}
		return deny("Planning runs may only read context")
	}
	// Never consume an approval for another scope before discovering a denial.
	for _, scope := range scopes {
		if spec.Mode(scope) == agentdef.Denied {
			return deny(scope + " is denied for this run")
		}
	}
	detail := map[string]any{"tool_name": in.Tool, "tool_input": in.Input}
	raw, _ := json.Marshal(detail)
	var approvedIDs []store.ApprovalRequest
	for _, scope := range scopes {
		if spec.Mode(scope) != agentdef.RequiresApproval {
			continue
		}
		approvals, err := q.ListApprovalsForRun(ctx, run.ID)
		if err != nil {
			return ToolDecision{}, err
		}
		approved := false
		for _, a := range approvals {
			var d any
			_ = json.Unmarshal(a.ActionDetail, &d)
			canonical, _ := json.Marshal(d)
			if a.Scope != scope || !bytes.Equal(canonical, raw) {
				continue
			}
			if a.State == "denied" {
				return deny("A person denied this exact tool action")
			}
			if a.State == "approved" && a.ExecutedAt == nil {
				approvedIDs = append(approvedIDs, a)
				approved = true
				break
			}
		}
		if approved {
			continue
		}
		if run.TicketID == nil {
			return deny("This action requires approval; ticket-less runs cannot request this scope")
		}
		summary := fmt.Sprintf("Allow %s for %s", in.Tool, scope)
		if command, ok := in.Input["command"].(string); ok {
			summary += ": " + command
		} else if file, ok := in.Input["file_path"].(string); ok {
			summary += ": " + file
		}
		if r := []rune(summary); len(r) > 300 {
			summary = string(r[:297]) + "..."
		}
		if _, err := s.RequestApproval(ctx, c, "", scope, summary, detail, "Harness tool requested this exact input"); err != nil {
			return ToolDecision{}, err
		}
		return deny("Approval requested in LazyAgents. End this turn; retry the exact action after approval")
	}
	if len(approvedIDs) > 0 {
		err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
			tq := store.New(tx)
			current, err := tq.LockRun(ctx, run.ID)
			if err != nil {
				return err
			}
			if current.State != "running" {
				return fmt.Errorf("run is no longer running")
			}
			for _, a := range approvedIDs {
				if _, err := tq.ClaimApprovalExecution(ctx, store.ClaimApprovalExecutionParams{ID: a.ID, RunID: run.ID}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return deny("Approval already used or run changed; request a new approval")
		}
	}
	return ToolDecision{Decision: "allow"}, nil
}
