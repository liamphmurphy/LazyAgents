package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Every chat mutation takes the session lock before its run lock. The session
// serializes posts, cancellation, completion, message seq and state notifications.
func lockRunChat(ctx context.Context, q *store.Queries, run store.Run) (store.ChatSession, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return chat, err
	}
	return q.LockChatSession(ctx, store.LockChatSessionParams{ID: chat.ID, WorkspaceID: run.WorkspaceID})
}

func idleChat(ctx context.Context, q *store.Queries, chat store.ChatSession, run store.Run, state string) error {
	reason := "human_reply"
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "waiting", WaitReason: &reason}); err != nil {
		return err
	}
	if _, err := q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: state}); err != nil {
		return err
	}
	return runs.RecomputeAgentStatus(ctx, q, run.AgentID)
}

func (r *Runner) finishChatTurn(ctx context.Context, c runCtx, turn store.RunTurn, res harness.Result, code int, waitErr error, logKey *string) {
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		chat, err := lockRunChat(ctx, q, c.run)
		if err != nil {
			return err
		}
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		usage, err := json.Marshal(res.Usage)
		if err != nil {
			return err
		}
		exit := int32(code)
		if err = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, ExitCode: &exit, LogKey: logKey, Usage: usage}); err != nil {
			return err
		}
		var source *string
		if res.Usage.CostSource != "" {
			source = &res.Usage.CostSource
		}
		if _, err = q.AddRunUsage(ctx, store.AddRunUsageParams{ID: run.ID, ToolCalls: int32(res.Usage.ToolCalls), InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, CostUsdMicros: int64(res.Usage.CostUSD * 1e6), CostSource: source}); err != nil {
			return err
		}
		if run.State != "running" && run.State != "paused" {
			return nil
		}
		state := "idle"
		if run.State == "running" {
			role, body := "assistant", res.Final
			if waitErr != nil || code != 0 || res.IsError || res.UsageLimited {
				state, role = "failed", "system"
				body = res.Error
				if body == "" {
					body = fmt.Sprintf("The chat turn stopped (exit %d). Send another message to retry.", code)
				}
			}
			citations := []byte("[]")
			if role == "assistant" {
				if citations, err = runs.ChatCitations(ctx, q, run.ID, turn.StartedAt); err != nil {
					return err
				}
			}
			if _, err = q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: role, BodyMd: body, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: []byte("[]"), Citations: citations, Usage: usage}); err != nil {
				return err
			}
		}
		if err = idleChat(ctx, q, chat, run, state); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"state": "turn_ended", "turn": turn.Seq, "exit_code": code})
		return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: payload})
	})
	if err != nil {
		r.Log.Error("finish chat turn", "run", c.run.ID, "err", err)
	}
}

// failChat handles setup/invocation failures before a harness result exists.
// The segment stays usable; pending/running records cannot block its next turn.
func (r *Runner) failChat(ctx context.Context, c runCtx, outcome string) {
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		chat, err := lockRunChat(ctx, q, c.run)
		if err != nil {
			return err
		}
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		if err = q.FinishUnfinishedChatTurns(ctx, run.ID); err != nil {
			return err
		}
		if _, err = q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "system", BodyMd: outcome, RunID: &run.ID, ContextRefs: []byte("[]"), Citations: []byte("[]")}); err != nil {
			return err
		}
		return idleChat(ctx, q, chat, run, "failed")
	})
	if err != nil {
		r.Log.Error("fail chat turn", "run", c.run.ID, "err", err)
	}
}
