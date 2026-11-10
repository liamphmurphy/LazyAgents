package agentapi

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Chat history tools: a chat's persona looking back on the person's earlier
// conversations, for what was said but never saved to memory. They reach only
// the person's own chats, never hidden or temporary ones, and by default only
// those this persona took part in, so one persona's conversations don't
// surface in another's unless the person asks for that.

const (
	chatHistoryDefaultLimit = 8
	chatHistoryMaxLimit     = 20
	readChatDefaultLimit    = 20
	readChatMaxLimit        = 40
	// readChatMaxChars caps each message read back, so one long reply can't
	// crowd out the rest of the page.
	readChatMaxChars = 4000
)

type SearchChatsIn struct {
	Query      string `json:"query" jsonschema:"Words to look for, web-search style: quoted phrases, OR, and -word to exclude"`
	AnyPersona bool   `json:"any_persona,omitempty" jsonschema:"Also search chats with the person's other personas; only when the person asks for that"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Up to 20 chats, default 8"`
}

type ReadChatIn struct {
	ChatID     string `json:"chat_id" jsonschema:"Chat id from search_chats"`
	AroundSeq  int64  `json:"around_seq,omitempty" jsonschema:"Read the messages around this one (a seq from search_chats); omit to read from the start"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Up to 40 messages, default 20"`
	AnyPersona bool   `json:"any_persona,omitempty" jsonschema:"Set when the chat came from a search with any_persona"`
}

type chatHistoryHit struct {
	ChatID   uuid.UUID `json:"chat_id"`
	Title    string    `json:"title"`
	Persona  string    `json:"persona,omitempty"`
	Archived bool      `json:"archived,omitempty"`
	Seq      int64     `json:"seq"`
	Role     string    `json:"role"`
	At       string    `json:"at"`
	Snippet  string    `json:"snippet"`
}

type chatHistoryMessage struct {
	Seq     int64  `json:"seq"`
	Role    string `json:"role"`
	Persona string `json:"persona,omitempty"`
	At      string `json:"at"`
	Body    string `json:"body"`
	// Clipped marks a body cut at readChatMaxChars.
	Clipped bool `json:"clipped,omitempty"`
}

// chatHistoryScope is the chat a history tool runs in, refused outside chats
// and in temporary ones.
func chatHistoryScope(ctx context.Context, d Deps, c agents.Caller) (*chatScope, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("chat history tools only work in a chat with a person")
	}
	if s.Temporary {
		return nil, apperr.Conflict(temporaryChatRefusal)
	}
	return s, nil
}

// snippetMarks turns the private-use match markers SearchChatHistory puts
// around each match into Markdown bold.
var snippetMarks = strings.NewReplacer("\ue000", "**", "\ue001", "**")

func searchChats(ctx context.Context, d Deps, c agents.Caller, in SearchChatsIn) (any, error) {
	s, err := chatHistoryScope(ctx, d, c)
	if err != nil {
		return nil, err
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return nil, apperr.Invalid("query", "say what to look for")
	}
	if in.Limit <= 0 || in.Limit > chatHistoryMaxLimit {
		in.Limit = chatHistoryDefaultLimit
	}
	rows, err := d.Q.SearchChatHistory(ctx, store.SearchChatHistoryParams{Query: in.Query, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID,
		CurrentChatID: s.ChatID, AnyPersona: in.AnyPersona, PersonaID: s.PersonaID, RowLimit: int32(in.Limit)})
	if err != nil {
		return nil, err
	}
	out := make([]chatHistoryHit, len(rows))
	for i, r := range rows {
		out[i] = chatHistoryHit{ChatID: r.SessionID, Title: r.ChatTitle, Persona: r.PersonaName, Archived: r.Archived, Seq: r.Seq, Role: r.Role,
			At: r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), Snippet: snippetMarks.Replace(r.Snippet)}
	}
	hint := "Open one with read_chat(chat_id, around_seq) before relying on it: a snippet is only part of what was said."
	if len(out) == 0 {
		hint = "No past chats match. Say so rather than guessing; try other words or, if the person wants, any_persona."
		if !in.AnyPersona {
			hint = "No past chats with you match. Say so rather than guessing; try other words, or any_persona if the person wants their other personas' chats searched."
		}
	}
	return map[string]any{"chats": out, "hint": hint}, nil
}

func readChat(ctx context.Context, d Deps, c agents.Caller, in ReadChatIn) (any, error) {
	s, err := chatHistoryScope(ctx, d, c)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("chat_id", in.ChatID)
	if err != nil {
		return nil, err
	}
	chat, err := d.Q.ChatHistorySession(ctx, store.ChatHistorySessionParams{ID: id, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID,
		AnyPersona: in.AnyPersona, PersonaID: s.PersonaID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.Limit <= 0 || in.Limit > readChatMaxLimit {
		in.Limit = readChatDefaultLimit
	}
	// Start a few messages before the one asked about, so it reads in context.
	after := int64(0)
	if in.AroundSeq > 0 {
		after = max(0, in.AroundSeq-int64(in.Limit)/4-1)
	}
	rows, err := d.Q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Seq: after, Limit: int32(in.Limit)})
	if err != nil {
		return nil, err
	}
	names, err := personaNames(ctx, d, c.Agent.WorkspaceID, s.UserID, chat.PersonaID, rows)
	if err != nil {
		return nil, err
	}
	msgs := make([]chatHistoryMessage, 0, len(rows))
	for _, m := range rows {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		body, clipped := m.BodyMd, false
		if r := []rune(body); len(r) > readChatMaxChars {
			body, clipped = string(r[:readChatMaxChars]), true
		}
		msg := chatHistoryMessage{Seq: m.Seq, Role: m.Role, At: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), Body: body, Clipped: clipped}
		if m.Role == "assistant" {
			pid := m.PersonaID
			if pid == nil {
				pid = chat.PersonaID
			}
			if pid != nil {
				msg.Persona = names[*pid]
			}
		}
		msgs = append(msgs, msg)
	}
	out := map[string]any{"chat_id": chat.ID, "title": chat.Title, "archived": chat.Archived,
		"created_at": chat.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), "messages": msgs}
	if len(rows) == in.Limit {
		out["hint"] = "There may be more: call read_chat again with around_seq set to the last seq here to read on (it repeats a few messages for context)."
	}
	return out, nil
}

// personaNames names the personas that wrote rows, by id: each message's own
// persona, or the chat's host for one without.
func personaNames(ctx context.Context, d Deps, ws, user uuid.UUID, host *uuid.UUID, rows []store.ChatMessage) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	ids := []*uuid.UUID{host}
	for _, m := range rows {
		ids = append(ids, m.PersonaID)
	}
	for _, id := range ids {
		if id == nil {
			continue
		}
		if _, ok := out[*id]; ok {
			continue
		}
		p, err := d.Q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: ws, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) {
			out[*id] = "" // deleted since
			continue
		}
		if err != nil {
			return nil, err
		}
		out[*id] = p.Name
	}
	return out, nil
}
