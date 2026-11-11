package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// An aside is a question about a reply while it runs, answered without
// interrupting it. Each aside is a hidden chat of its own (parent_chat_id),
// answered by the same persona in a fresh session whose prompt carries the
// parent's recent conversation and a snapshot of the running reply: what it
// has written and which tools it called, with their output. The aside reads
// but changes nothing (ChatCaps.Aside: no shell, images, connectors or
// secrets, and only the read tools over MCP), so it can't race the reply it
// asks about. Neither sees the other afterwards: the parent's history never
// includes its asides.

const (
	// MaxLiveAsides bounds the asides answering at once for one chat.
	MaxLiveAsides = 3
	// asideHistoryChars bounds the parent's conversation an aside gets.
	asideHistoryChars = 12000
	// asideEvents is how many of the running reply's latest events it gets.
	asideEvents = 60
	// asideInputRunes and asideOutputRunes clip one tool call's input and output.
	asideInputRunes  = 240
	asideOutputRunes = 600
	asideTextRunes   = 2000
	// asideSnapshotRunes bounds the whole snapshot of the running reply.
	asideSnapshotRunes = 16000
)

const asideFooter = `---
(Vesper: this is an aside. While you keep working on the reply above in the main conversation, the person asked the question below. You are a separate instance answering only this question: the main reply carries on without seeing it, and nothing you write here reaches it.
Answer briefly and concretely from the snapshot above and the conversation; use the read tools if you need more. Don't create, change or delete anything (tickets, epics, memory, canvases, chats, code). If the question asks to change what the main reply is doing, say so, and that the person can queue it as a follow-up or send it to interrupt.)

The person's question:
`

// StartChatAside asks a question about the chat's running reply. It returns
// the aside: a hidden chat whose first turn answers it.
func (s *Service) StartChatAside(ctx context.Context, ws, user, parentID uuid.UUID, question string) (store.ChatSession, error) {
	var out store.ChatSession
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > 20000 {
		return out, apperr.Invalid("body", "provide 1–20000 characters")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		parent, err := lockChat(ctx, q, ws, user, parentID)
		if err != nil {
			return err
		}
		switch {
		case parent.Archived:
			return apperr.Conflict("the chat is archived")
		case parent.ParentChatID != nil:
			return apperr.Conflict("an aside can't have asides of its own")
		case parent.State != "running" || parent.CurrentRunID == nil:
			return apperr.Conflict("nothing is running: send it as a message instead")
		}
		asides, err := q.ListChatAsides(ctx, &parent.ID)
		if err != nil {
			return err
		}
		if n := countRunning(asides); n >= MaxLiveAsides {
			return apperr.Conflict("%d asides are still answering; wait for one to finish", n)
		}
		preamble, err := asidePreamble(ctx, q, parent)
		if err != nil {
			return err
		}
		p, err := chatProject(ctx, q, parent)
		if err != nil {
			return err
		}
		// No repository and no connectors; the persona answering the parent
		// answers the aside. Temporary carries over, so a temporary chat's
		// asides don't reach memory or past chats either.
		aside, err := s.startChat(ctx, q, ws, user, ChatInput{
			Project:      p.Key,
			Title:        "Aside: " + firstLine(question, 70),
			PersonaID:    ChatSpeakerID(parent),
			Hidden:       true,
			Temporary:    parent.Temporary,
			NoRepo:       true,
			ConnectorIDs: []uuid.UUID{},
		})
		if err != nil {
			return err
		}
		// Asked of the same model, at the same effort, as the reply it's about.
		if aside, err = q.SetChatSelection(ctx, store.SetChatSelectionParams{ID: aside.ID, Harness: parent.Harness, Model: parent.Model, Effort: parent.Effort}); err != nil {
			return err
		}
		if aside, err = q.SetChatParent(ctx, store.SetChatParentParams{ID: aside.ID, ParentChatID: &parent.ID}); err != nil {
			return err
		}
		if aside, _, err = postChatMessage(ctx, q, s.ChatCatalog(), aside, ChatMessageInput{Body: question, preamble: preamble}, s.HandoffChars); err != nil {
			return err
		}
		// The parent's clients learn of the new aside from its chat.updated.
		if _, err = q.BumpChatRevision(ctx, parent.ID); err != nil {
			return err
		}
		out = aside
		return nil
	})
	return out, err
}

func countRunning(asides []store.ChatSession) int {
	n := 0
	for _, a := range asides {
		if a.State == "running" {
			n++
		}
	}
	return n
}

// ListChatAsides is the chat's asides, oldest first.
func (s *Service) ListChatAsides(ctx context.Context, ws, user, id uuid.UUID) ([]store.ChatSession, error) {
	var out []store.ChatSession
	q := store.New(s.Pool)
	chat, err := readOwnChat(ctx, q, ws, user, id)
	if err != nil {
		return out, err
	}
	return q.ListChatAsides(ctx, &chat.ID)
}

// EndAside runs in the runner's transaction once an aside's turn has ended
// (the chat lock is held): it has answered, or was stopped, so its segment
// completes and the runner sweep removes its workspace.
func EndAside(ctx context.Context, q *store.Queries, chatID uuid.UUID) error {
	chat, err := q.LockChatSessionByID(ctx, chatID)
	if err != nil || chat.State == "running" || chat.CurrentRunID == nil {
		return err
	}
	run, err := q.LockRun(ctx, *chat.CurrentRunID)
	if err != nil {
		return err
	}
	if slices.Contains(Terminal, run.State) {
		return nil
	}
	outcome := "The aside answered."
	if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
		return err
	}
	if err = q.DropPendingTurns(ctx, run.ID); err != nil {
		return err
	}
	if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
		return err
	}
	if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_segment_ended", "reason": "aside_answered"}); err != nil {
		return err
	}
	return RecomputeAgentStatus(ctx, q, run.AgentID)
}

// asidePreamble is what an aside's turn is told ahead of the question: the
// parent's recent conversation, then the snapshot of its running reply.
func asidePreamble(ctx context.Context, q *store.Queries, parent store.ChatSession) (string, error) {
	total, err := q.CountChatHistory(ctx, parent.ID)
	if err != nil {
		return "", err
	}
	rows, err := q.ListRecentChatMessages(ctx, store.ListRecentChatMessagesParams{SessionID: parent.ID, RowLimit: handoffRows, MaxChars: handoffFetchChars})
	if err != nil {
		return "", err
	}
	hist := make([]HandoffMessage, len(rows))
	for i, r := range rows {
		hist[i] = HandoffMessage{Seq: r.Seq, Role: r.Role, Harness: deref(r.Harness), Model: deref(r.Model), Persona: r.PersonaName, Body: r.BodyMd}
	}
	hc, err := loadHandoffContext(ctx, q, parent.ID)
	if err != nil {
		return "", err
	}
	snapshot, err := liveTurnSnapshot(ctx, q, *parent.CurrentRunID, time.Now())
	if err != nil {
		return "", err
	}
	// The handoff's lead would say other models answered; here it's the same persona, still working.
	rendered := renderHandoff(hist, int(total), asideHistoryChars, hc, snapshot+asideFooter, "")
	return strings.Replace(rendered, handoffLead, "You are answering an aside in the conversation below, the one you are working on right now.", 1), nil
}

// liveTurnSnapshot describes the running reply on run so far, oldest first.
func liveTurnSnapshot(ctx context.Context, q *store.Queries, runID uuid.UUID, now time.Time) (string, error) {
	since, err := q.OpenChatTurnStart(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "---\nYour reply in the main conversation is about to start; it hasn't written anything or used any tools yet.\n\n", nil
	}
	if err != nil {
		return "", err
	}
	rows, err := q.LiveTurnEvents(ctx, store.LiveTurnEventsParams{RunID: runID, At: since, RowLimit: asideEvents})
	if err != nil {
		return "", err
	}
	slices.Reverse(rows)
	return renderLiveTurn(rows, now.Sub(since)), nil
}

// renderLiveTurn is liveTurnSnapshot's text, from the reply's events oldest
// first. When it runs long, the oldest lines go first.
func renderLiveTurn(rows []store.LiveTurnEventsRow, elapsed time.Duration) string {
	var lines []string
	for _, r := range rows {
		var p struct {
			Text    string          `json:"text"`
			Tool    string          `json:"tool"`
			Input   json.RawMessage `json:"input"`
			Output  string          `json:"output"`
			IsError bool            `json:"is_error"`
		}
		if json.Unmarshal(r.Payload, &p) != nil {
			continue
		}
		switch r.Type {
		case "message":
			if t := strings.TrimSpace(p.Text); t != "" {
				lines = append(lines, "You wrote:\n"+quote(clip(t, asideTextRunes)))
			}
		case "tool_call":
			if p.Tool == "" {
				continue
			}
			line := "Called " + p.Tool
			if in := strings.TrimSpace(string(p.Input)); in != "" && in != "{}" && in != "null" {
				line += " " + head(in, asideInputRunes)
			}
			lines = append(lines, line)
		case "tool_result":
			out := strings.TrimSpace(p.Output)
			if out == "" {
				continue
			}
			label := "Result"
			if p.IsError {
				label = "Error"
			}
			lines = append(lines, label+":\n"+quote(clip(out, asideOutputRunes)))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nYour reply in the main conversation is still running (for %s so far). What it has done, oldest first:\n\n", elapsed.Round(time.Second))
	if len(lines) == 0 {
		b.WriteString("Nothing yet: it hasn't written anything or used any tools.\n\n")
		return b.String()
	}
	size := 0
	keep := len(lines)
	for keep > 0 && size+len([]rune(lines[keep-1])) <= asideSnapshotRunes {
		keep--
		size += len([]rune(lines[keep])) + 2
	}
	if keep > 0 {
		fmt.Fprintf(&b, "(%d earlier steps left out)\n\n", keep)
	}
	b.WriteString(strings.Join(lines[keep:], "\n\n"))
	b.WriteString("\n\n")
	return b.String()
}

// head keeps the start of s within max runes.
func head(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
