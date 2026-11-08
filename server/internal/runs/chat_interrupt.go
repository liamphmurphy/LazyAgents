package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A message the person sends while a reply is running interrupts it: the
// reply is stopped, and the next turn answers the new message together with
// the one before it and what the stopped reply had already said and done
// (its text and tool calls; thinking isn't recorded). When the stopped turn
// hadn't started, that turn is simply replaced. Otherwise the message waits on
// the chat (interrupt_message_id) until the runner has stopped the process,
// and ResumeInterruptedChat starts its turn.

// Bounds on what the interruption note quotes from the stopped turn.
const (
	interruptQuoteRunes = 4000
	interruptMaxTools   = 20
)

// stopChatTurn stops the chat's running turn. idle reports that it stopped at
// once; dropped is then the turn that hadn't been claimed yet, if any.
// Otherwise the run is paused and the runner returns the chat to idle when
// the process exits. The caller holds the chat lock.
func stopChatTurn(ctx context.Context, q *store.Queries, chat store.ChatSession) (_ store.ChatSession, idle bool, dropped *store.RunTurn, _ error) {
	run, err := q.LockRun(ctx, *chat.CurrentRunID)
	if err != nil {
		return chat, false, nil, err
	}
	if run.State == "paused" {
		return chat, false, nil, nil
	} // cancellation is already draining setup/the turn
	// Read before the drop; it is only used when nothing turns out active,
	// so a claimant that wins in between can't make it stale.
	pending, err := q.PendingChatTurn(ctx, run.ID)
	if err == nil {
		dropped = &pending
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return chat, false, nil, err
	}
	// Resolve the claim boundary first. At READ COMMITTED, this UPDATE
	// waits for a winning claimant and rechecks its pending predicate;
	// the following SELECT gets a fresh snapshot and sees that running turn.
	// If we win, the row stays locked/done so a claimant cannot take it.
	if err = q.DropPendingTurns(ctx, run.ID); err != nil {
		return chat, false, nil, err
	}
	active, err := q.HasRunningChatTurn(ctx, run.ID)
	if err != nil {
		return chat, false, nil, err
	}
	state, reason := "waiting", "human_reply"
	if active || run.State == "starting" {
		state, reason = "paused", "pause"
	}
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state, WaitReason: &reason}); err != nil {
		return chat, false, nil, err
	}
	idle = state == "waiting"
	if idle {
		if chat, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: "idle"}); err != nil {
			return chat, false, nil, err
		}
	} else {
		dropped = nil
	}
	return chat, idle, dropped, event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_turn_cancelled"})
}

// replacedTurnNote joins a turn that never started to the message replacing
// it: its whole input (the earlier message, plus any handoff or note it
// carried, since the session never saw it) comes first. It is "" when there
// was no such turn from the person, and interruptNote is used instead.
func replacedTurnNote(dropped *store.RunTurn) string {
	if dropped == nil || dropped.Trigger != "comment" {
		return ""
	}
	return dropped.InputMessage + "\n\n(Vesper: before you started on that, the person sent another message. Answer both together; if the new one changes or cancels the earlier request, follow the new one.)\n\n"
}

// ResumeInterruptedChat runs in the runner's transaction once a stopped turn's
// process has exited and the chat is idle (the chat lock is held). If a
// message interrupted that turn, it starts the turn that answers it. since is
// when the stopped turn began; nil when it never got to run.
func ResumeInterruptedChat(ctx context.Context, q *store.Queries, chatID, runID uuid.UUID, since *time.Time, handoffChars int) error {
	chat, err := q.LockChatSessionByID(ctx, chatID)
	if err != nil || chat.InterruptMessageID == nil {
		return err
	}
	msgID := *chat.InterruptMessageID
	if chat, err = q.SetChatInterrupt(ctx, store.SetChatInterruptParams{ID: chat.ID}); err != nil {
		return err
	}
	if chat.Archived || chat.State == "running" {
		return nil
	}
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: chat.ProjectID, WorkspaceID: chat.WorkspaceID})
	if err != nil || p.Archived {
		return err
	}
	msg, err := q.GetChatMessage(ctx, msgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	note, err := interruptNote(ctx, q, chat, msg.Seq, &runID, since)
	if err != nil {
		return err
	}
	speaker, queue, err := routeHumanMessage(ctx, q, chat, msg.BodyMd)
	if err != nil {
		return err
	}
	if chat, err = q.SetChatSpeaker(ctx, store.SetChatSpeakerParams{ID: chat.ID, SpeakerPersonaID: speaker, QueuedPersonaIds: queue, RelayTurns: 0}); err != nil {
		return err
	}
	_, run, turn, err := startChatTurn(ctx, q, chat, p.Key, note+msg.BodyMd, false, handoffChars)
	if err != nil {
		return err
	}
	return q.SetChatMessageTurn(ctx, store.SetChatMessageTurnParams{ID: msg.ID, RunID: &run.ID, RunTurnID: &turn.ID, Harness: &run.Harness, Model: &run.Model})
}

// interruptNote is the preamble of the turn that answers an interruption: the
// person's previous message and what the stopped reply had written and which
// tools it called. The stored message is only what the person wrote. before is
// the interrupting message's seq; runID and since locate the stopped turn's
// events (nil since: it never ran).
func interruptNote(ctx context.Context, q *store.Queries, chat store.ChatSession, before int64, runID *uuid.UUID, since *time.Time) (string, error) {
	prev, err := q.PreviousChatUserMessage(ctx, store.PreviousChatUserMessageParams{SessionID: chat.ID, Seq: before})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var said []string
	var tools []string
	if runID != nil && since != nil {
		rows, err := q.InterruptedTurnEvents(ctx, store.InterruptedTurnEventsParams{RunID: *runID, At: *since})
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			var p struct {
				Text string `json:"text"`
				Tool string `json:"tool"`
			}
			if json.Unmarshal(r.Payload, &p) != nil {
				continue
			}
			switch {
			case r.Type == "message" && strings.TrimSpace(p.Text) != "":
				said = append(said, strings.TrimSpace(p.Text))
			case r.Type == "tool_call" && p.Tool != "":
				tools = append(tools, p.Tool)
			}
		}
	}
	var b strings.Builder
	b.WriteString("(Vesper: the person interrupted your reply to their previous message to send the one below.\n\n")
	b.WriteString("Their previous message:\n" + quote(clip(prev.BodyMd, interruptQuoteRunes)) + "\n\n")
	if len(said) == 0 && len(tools) == 0 {
		b.WriteString("You hadn't written anything or used any tools yet.\n\n")
	}
	if len(tools) > 0 {
		b.WriteString("Tools you had already called, in order: " + summarizeTools(tools) + ".\n\n")
	}
	if len(said) > 0 {
		b.WriteString("What you had written so far:\n" + quote(clip(strings.Join(said, "\n\n"), interruptQuoteRunes)) + "\n\n")
	}
	b.WriteString("Answer the new message together with the previous one: carry on from where you were, using what you already found, without redoing finished work or repeating what you wrote. If the new message changes or cancels the earlier request, follow the new message.)\n\n")
	return b.String(), nil
}

// summarizeTools lists tool names in call order, collapsing repeats
// ("read_page ×3") and capping the list.
func summarizeTools(tools []string) string {
	var out []string
	for i := 0; i < len(tools); {
		j := i
		for j < len(tools) && tools[j] == tools[i] {
			j++
		}
		if n := j - i; n > 1 {
			out = append(out, fmt.Sprintf("%s ×%d", tools[i], n))
		} else {
			out = append(out, tools[i])
		}
		i = j
	}
	if len(out) > interruptMaxTools {
		out = append(out[:interruptMaxTools], fmt.Sprintf("and %d more", len(out)-interruptMaxTools))
	}
	return strings.Join(out, ", ")
}

// clip keeps the end of s, where an interrupted reply was heading, within max runes.
func clip(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return "…" + string(r[len(r)-max+1:])
}

func quote(s string) string {
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

// lastUserSeq stands for "after every message" when looking up the previous one.
const lastUserSeq = math.MaxInt64
