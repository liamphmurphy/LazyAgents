package httpapi

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const dockLastLineMax = 120

// dockRank orders states for a persona with several chats; the highest wins.
var dockRank = map[api.PersonaDockState]int{api.Working: 0, api.Replied: 1, api.NeedsYou: 2}

// dockLastLine is the last non-empty line of a reply, shortened for a list row.
func dockLastLine(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	line := []rune(strings.TrimSpace(lines[len(lines)-1]))
	if len(line) > dockLastLineMax {
		return string(line[:dockLastLineMax]) + "…"
	}
	return string(line)
}

// dockPersona is who a chat shows under: whoever is answering now (so a
// hand-off moves it), otherwise whoever wrote the latest reply. Nil is the
// built-in Assistant, which has no place on the dock.
func dockPersona(c store.PersonaDockChatsRow) *uuid.UUID {
	if c.State == "running" {
		if c.SpeakerPersonaID != nil {
			return c.SpeakerPersonaID
		}
		return c.PersonaID
	}
	if c.LastPersonaID != nil {
		return c.LastPersonaID
	}
	return c.PersonaID
}

func dockState(c store.PersonaDockChatsRow) api.PersonaDockState {
	line := dockLastLine(c.LastBody)
	switch {
	case c.WaitingApproval:
		return api.NeedsYou
	case c.State == "running":
		return api.Working
	case c.Unread > 0 && strings.HasSuffix(line, "?"):
		return api.NeedsYou
	}
	return api.Replied
}

// buildPersonaDock groups chats by persona, so a persona appears once however
// many chats it is active in. rows and the result are both newest first.
func buildPersonaDock(rows []store.PersonaDockChatsRow, personas map[uuid.UUID]store.Persona) []api.PersonaDockEntry {
	out := []api.PersonaDockEntry{}
	at := map[uuid.UUID]int{}
	for _, c := range rows {
		id := dockPersona(c)
		if id == nil {
			continue
		}
		p, ok := personas[*id]
		if !ok {
			continue
		}
		i, seen := at[*id]
		if !seen {
			i = len(out)
			at[*id] = i
			out = append(out, api.PersonaDockEntry{Persona: *toPersonaSummary(p), State: api.Working, LastActivityAt: c.LastActivityAt, Chats: []api.PersonaDockChat{}})
		}
		state := dockState(c)
		e := &out[i]
		e.Unread += int(c.Unread)
		if dockRank[state] > dockRank[e.State] {
			e.State = state
		}
		e.Chats = append(e.Chats, api.PersonaDockChat{Id: openapi_types.UUID(c.ID), Title: c.Title, State: state, Unread: int(c.Unread),
			LastLine: dockLastLine(c.LastBody), LastSeq: c.LastSeq, WaitingApproval: c.WaitingApproval, LastActivityAt: c.LastActivityAt})
	}
	return out
}

func (s *Server) GetPersonaDock(ctx context.Context, _ api.GetPersonaDockRequestObject) (api.GetPersonaDockResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.PersonaDockChats(ctx, store.PersonaDockChatsParams{WorkspaceID: ws, CreatedBy: u.ID})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, c := range rows {
		if id := dockPersona(c); id != nil && !slices.Contains(ids, *id) {
			ids = append(ids, *id)
		}
	}
	personas := map[uuid.UUID]store.Persona{}
	if len(ids) > 0 {
		found, err := s.Q.PersonasByIDs(ctx, store.PersonasByIDsParams{Ids: ids, WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			personas[p.ID] = p
		}
	}
	harnesses, err := s.harnessActivity(ctx, ws)
	if err != nil {
		return nil, err
	}
	return api.GetPersonaDock200JSONResponse{Personas: buildPersonaDock(rows, personas), Harnesses: harnesses}, nil
}

func (s *Server) MarkChatRead(ctx context.Context, req api.MarkChatReadRequestObject) (api.MarkChatReadResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if err := s.Q.MarkChatRead(ctx, store.MarkChatReadParams{ID: c.ID, Seq: req.Body.Seq}); err != nil {
		return nil, err
	}
	return api.MarkChatRead204Response{}, nil
}
