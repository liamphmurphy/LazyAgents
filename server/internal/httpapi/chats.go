package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Chat endpoints (wiki: Architecture/ADR - Project chat sessions). Every
// lookup is scoped to the workspace and the creator: someone else's chat is a 404.

const chatPageSize = 50

// ownChat loads a chat the caller created, or ErrNotFound.
func (s *Server) ownChat(ctx context.Context, id uuid.UUID) (store.ChatSession, error) {
	u, _, ws := me(ctx)
	c, err := s.Q.GetChatSession(ctx, store.GetChatSessionParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.CreatedBy != u.ID) {
		return c, apperr.ErrNotFound
	}
	return c, err
}

// toChats renders chats with batched lookups (projects, agents, counts, errors).
func (s *Server) toChats(ctx context.Context, chats []store.ChatSession) ([]api.ChatSession, error) {
	out := make([]api.ChatSession, 0, len(chats))
	if len(chats) == 0 {
		return out, nil
	}
	u, _, ws := me(ctx)
	projects, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	agentRows, err := s.Q.ListAllAgents(ctx, ws)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(chats))
	var failed, personaIDs []uuid.UUID
	for i, c := range chats {
		ids[i] = c.ID
		if c.State == "failed" {
			failed = append(failed, c.ID)
		}
		if c.PersonaID != nil && !slices.Contains(personaIDs, *c.PersonaID) {
			personaIDs = append(personaIDs, *c.PersonaID)
		}
	}
	personaRows := map[uuid.UUID]store.Persona{}
	if len(personaIDs) > 0 {
		// Only the caller's own personas; chats are listed for their creator.
		rows, err := s.Q.PersonasByIDs(ctx, store.PersonasByIDsParams{Ids: personaIDs, WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			personaRows[p.ID] = p
		}
	}
	counts := map[uuid.UUID]int{}
	rows, err := s.Q.ChatMessageCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.SessionID] = int(r.N)
	}
	reasons := map[uuid.UUID]string{}
	if len(failed) > 0 {
		errs, err := s.Q.ChatLastErrors(ctx, failed)
		if err != nil {
			return nil, err
		}
		for _, e := range errs {
			reasons[e.SessionID] = e.BodyMd
		}
	}
	for _, c := range chats {
		item := api.ChatSession{
			Id: c.ID, Title: c.Title, Harness: api.Harness(c.Harness), Model: c.Model, State: api.ChatState(c.State),
			CurrentRunId: c.CurrentRunID, Archived: c.Archived, Revision: int(c.Revision), MessageCount: counts[c.ID],
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, LastMessageAt: c.LastMessageAt,
		}
		if i := slices.IndexFunc(projects, func(p store.Project) bool { return p.ID == c.ProjectID }); i >= 0 {
			item.Project = projects[i].Key
		}
		if i := slices.IndexFunc(agentRows, func(a store.Agent) bool { return a.ID == c.AgentID }); i >= 0 {
			item.Agent = *toAgentRef(agentRows[i])
		}
		if c.PersonaID != nil {
			if p, ok := personaRows[*c.PersonaID]; ok {
				item.Persona = toPersonaSummary(p)
			}
		}
		if r, ok := reasons[c.ID]; ok && c.State == "failed" {
			item.Error = &r
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) toChat(ctx context.Context, c store.ChatSession) (api.ChatSession, error) {
	out, err := s.toChats(ctx, []store.ChatSession{c})
	if err != nil {
		return api.ChatSession{}, err
	}
	return out[0], nil
}

// toMessages renders messages; turn numbers come from the few runs involved.
func (s *Server) toMessages(ctx context.Context, msgs []store.ChatMessage) ([]api.ChatMessage, error) {
	u, _, _ := me(ctx)
	turnSeq := map[uuid.UUID]int{}
	seenRuns := map[uuid.UUID]bool{}
	for _, m := range msgs {
		if m.RunID == nil || m.RunTurnID == nil || seenRuns[*m.RunID] {
			continue
		}
		seenRuns[*m.RunID] = true
		turns, err := s.Q.ListTurns(ctx, *m.RunID)
		if err != nil {
			return nil, err
		}
		for _, t := range turns {
			turnSeq[t.ID] = int(t.Seq)
		}
	}
	files, err := s.messageAttachments(ctx, msgs)
	if err != nil {
		return nil, err
	}
	out := make([]api.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		item := api.ChatMessage{Id: m.ID, Seq: int(m.Seq), Role: api.ChatMessageRole(m.Role), BodyMd: m.BodyMd, RunId: m.RunID, Model: m.Model, CreatedAt: m.CreatedAt}
		if m.Harness != nil {
			h := api.Harness(*m.Harness)
			item.Harness = &h
		}
		if m.RunTurnID != nil && m.Role == "assistant" {
			if n, ok := turnSeq[*m.RunTurnID]; ok {
				item.TurnSeq = &n
			}
		}
		refs, _ := recode[[]api.ChatContextRef](jsonOrEmpty(m.ContextRefs))
		cites, _ := recode[[]api.ChatCitation](jsonOrEmpty(m.Citations))
		item.ContextRefs, item.Citations = &refs, &cites
		if m.Role == "user" {
			item.Author = toUserRef(u)
			attached := files[m.ID]
			if attached == nil {
				attached = []api.ChatAttachment{}
			}
			item.Attachments = &attached
		}
		if len(m.Usage) > 0 {
			var usage harness.Usage
			if json.Unmarshal(m.Usage, &usage) == nil && m.Role == "assistant" {
				in, outTok, cost := usage.InputTokens, usage.OutputTokens, float32(usage.CostUSD)
				item.InputTokens, item.OutputTokens, item.CostUsd = &in, &outTok, &cost
				var saved map[string]json.RawMessage
				_ = json.Unmarshal(m.Usage, &saved)
				reported := usage.CacheReportedInputTokens
				legacySupported := m.Harness != nil && (*m.Harness == "codex" || *m.Harness == "claude-code")
				if _, modern := saved["cache_reported_input_tokens"]; !modern && legacySupported && saved["cached_tokens"] != nil && (usage.SessionInputTokens > 0 || usage.CostSource == "harness") {
					reported = usage.InputTokens
				}
				cached, uncached, unknown := cacheBreakdown(in, usage.CachedTokens, reported)
				item.CachedInputTokens, item.UncachedInputTokens, item.UnreportedInputTokens = &cached, &uncached, &unknown
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// jsonOrEmpty keeps recode working on a NULL-ish column.
func jsonOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func (s *Server) ListChatModels(ctx context.Context, _ api.ListChatModelsRequestObject) (api.ListChatModelsResponseObject, error) {
	_, _, ws := me(ctx)
	list, err := s.Runs.ChatCatalog().List(ctx, s.Q, ws)
	if err != nil {
		return nil, err
	}
	out := make(api.ListChatModels200JSONResponse, 0, len(list))
	for _, h := range list {
		item := api.ChatHarnessModels{Harness: api.Harness(h.Harness), Label: h.Label, Available: h.Available, Models: make([]api.ChatModel, len(h.Models))}
		if h.UnavailableReason != "" {
			reason := h.UnavailableReason
			item.UnavailableReason = &reason
		}
		for i, m := range h.Models {
			item.Models[i] = api.ChatModel{Id: m.ID, Label: m.Label, Default: m.Default}
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) ListChats(ctx context.Context, req api.ListChatsRequestObject) (api.ListChatsResponseObject, error) {
	u, _, ws := me(ctx)
	offset := deref(req.Params.Offset)
	if offset < 0 || offset > 100000 {
		return nil, apperr.Invalid("offset", "must be between 0 and 100000")
	}
	filter, personaID, err := s.chatPersonaFilter(ctx, req.Params.PersonaId)
	if err != nil {
		return nil, err
	}
	chats, err := s.Q.ListChatsForUser(ctx, store.ListChatsForUserParams{
		WorkspaceID: ws, CreatedBy: u.ID, Archived: deref(req.Params.Archived), ProjectKey: req.Params.Project,
		PersonaFilter: filter, PersonaID: personaID, RowOffset: int32(offset), RowLimit: chatPageSize + 1,
	})
	if err != nil {
		return nil, err
	}
	more := len(chats) > chatPageSize
	if more {
		chats = chats[:chatPageSize]
	}
	items, err := s.toChats(ctx, chats)
	if err != nil {
		return nil, err
	}
	return api.ListChats200JSONResponse{Chats: items, HasMore: more}, nil
}

// chatPersonaFilter resolves GET /chats?persona_id= to ListChatsForUser's
// filter: `all`, `none`, or one of the caller's personas. Omitted means the
// caller's default persona, or `none` when they have no default.
func (s *Server) chatPersonaFilter(ctx context.Context, raw *string) (string, *uuid.UUID, error) {
	o := personaOwner(ctx)
	if raw == nil {
		p, err := s.personas().Default(ctx, o)
		if err != nil || p == nil {
			return "none", nil, err
		}
		return "id", &p.ID, nil
	}
	switch *raw {
	case "all", "none":
		return *raw, nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return "", nil, apperr.Invalid("persona_id", "must be a persona id, all or none")
	}
	if _, err := s.personas().Get(ctx, o, id); errors.Is(err, apperr.ErrNotFound) {
		return "", nil, apperr.Invalid("persona_id", "unknown persona")
	} else if err != nil {
		return "", nil, err
	}
	return "id", &id, nil
}

// toMessageInput converts the contract's message body.
func toMessageInput(b api.NewChatMessage) (runs.ChatMessageInput, error) {
	in := runs.ChatMessageInput{Body: b.Body, Model: deref(b.Model)}
	if b.Harness != nil {
		in.Harness = string(*b.Harness)
	}
	if b.ContextRefs != nil {
		refs, err := recode[[]runs.ChatContextRef](*b.ContextRefs)
		if err != nil {
			return in, err
		}
		in.ContextRefs = refs
	}
	if b.AttachmentIds != nil {
		in.AttachmentIDs = *b.AttachmentIds
	}
	return in, nil
}

func (s *Server) CreateChat(ctx context.Context, req api.CreateChatRequestObject) (api.CreateChatResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	b := req.Body
	in := runs.ChatInput{Project: b.Project, Title: deref(b.Title), Model: deref(b.Model), PersonaID: b.PersonaId}
	if b.Harness != nil {
		in.Harness = string(*b.Harness)
	}
	if b.Message != nil {
		m, err := toMessageInput(*b.Message)
		if err != nil {
			return nil, err
		}
		in.Message = &m
	}
	chat, err := s.Runs.StartChat(ctx, ws, u.ID, in)
	if err != nil {
		return nil, err
	}
	out, err := s.toChat(ctx, chat)
	return api.CreateChat201JSONResponse(out), err
}

func (s *Server) GetChat(ctx context.Context, req api.GetChatRequestObject) (api.GetChatResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toChat(ctx, c)
	return api.GetChat200JSONResponse(out), err
}

func (s *Server) UpdateChat(ctx context.Context, req api.UpdateChatRequestObject) (api.UpdateChatResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	if req.Body.Revision < 1 || req.Body.Revision > 2147483647 {
		return nil, apperr.Invalid("revision", "invalid revision")
	}
	c, err := s.Runs.UpdateChat(ctx, ws, u.ID, req.Id, runs.ChatPatchInput{Revision: int32(req.Body.Revision), Title: req.Body.Title, Archived: req.Body.Archived})
	if err != nil {
		return nil, err
	}
	out, err := s.toChat(ctx, c)
	return api.UpdateChat200JSONResponse(out), err
}

func (s *Server) DeleteChat(ctx context.Context, req api.DeleteChatRequestObject) (api.DeleteChatResponseObject, error) {
	u, _, ws := me(ctx)
	if err := s.Runs.DeleteChat(ctx, ws, u.ID, req.Id); err != nil {
		return nil, err
	}
	return api.DeleteChat204Response{}, nil
}

func (s *Server) ListChatMessages(ctx context.Context, req api.ListChatMessagesRequestObject) (api.ListChatMessagesResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	limit := 100
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, apperr.Invalid("limit", "must be between 1 and 200")
	}
	if req.Params.After != nil && *req.Params.After < 0 {
		return nil, apperr.Invalid("after", "must be 0 or more")
	}
	if req.Params.Before != nil && *req.Params.Before < 1 {
		return nil, apperr.Invalid("before", "must be 1 or more")
	}
	p := store.PageChatMessagesParams{SessionID: c.ID, RowLimit: int32(limit)}
	if req.Params.After != nil {
		v := int64(*req.Params.After)
		p.AfterSeq = &v
	}
	if req.Params.Before != nil {
		v := int64(*req.Params.Before)
		p.BeforeSeq = &v
	}
	// Without `after`, return the newest page (before the cursor); with it,
	// everything newer, oldest first.
	p.NewestFirst = p.AfterSeq == nil
	rows, err := s.Q.PageChatMessages(ctx, p)
	if err != nil {
		return nil, err
	}
	if p.NewestFirst {
		slices.Reverse(rows)
	}
	msgs, err := s.toMessages(ctx, rows)
	if err != nil {
		return nil, err
	}
	return api.ListChatMessages200JSONResponse{Messages: msgs, HasMore: len(rows) > 0 && rows[0].Seq > 1}, nil
}

func (s *Server) PostChatMessage(ctx context.Context, req api.PostChatMessageRequestObject) (api.PostChatMessageResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	in, err := toMessageInput(*req.Body)
	if err != nil {
		return nil, err
	}
	msg, err := s.Runs.PostChatMessage(ctx, ws, u.ID, req.Id, in)
	if err != nil {
		return nil, err
	}
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	chat, err := s.toChat(ctx, c)
	if err != nil {
		return nil, err
	}
	msgs, err := s.toMessages(ctx, []store.ChatMessage{msg})
	if err != nil {
		return nil, err
	}
	return api.PostChatMessage202JSONResponse{Chat: chat, Message: msgs[0]}, nil
}

func (s *Server) CancelChatTurn(ctx context.Context, req api.CancelChatTurnRequestObject) (api.CancelChatTurnResponseObject, error) {
	u, _, ws := me(ctx)
	c, err := s.Runs.CancelChat(ctx, ws, u.ID, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toChat(ctx, c)
	return api.CancelChatTurn200JSONResponse(out), err
}
