package agentapi

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agents"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/memory"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Memory tools: the personal assistant's per-person memory. They only work in
// a chat, and only on the memory of the person the chat belongs to.

type SearchMemoryIn struct {
	Query          string `json:"query,omitempty" jsonschema:"Words to look for in titles, tags, summaries and bodies; empty lists the newest entries"`
	Type           string `json:"type,omitempty" jsonschema:"Only this type: preference, project, person, life, work, fact or other"`
	Limit          int    `json:"limit,omitempty" jsonschema:"Up to 50, default 10"`
	IncludeRetired bool   `json:"include_retired,omitempty" jsonschema:"Also return entries superseded by newer ones"`
}

type MemoryIDIn struct {
	ID string `json:"id" jsonschema:"Memory id from the index, search_memory or save_memory"`
}

type SaveMemoryIn struct {
	Title   string   `json:"title" jsonschema:"Short name for the memory, e.g. Prefers metric units"`
	Summary string   `json:"summary,omitempty" jsonschema:"One line saying what to remember"`
	Body    string   `json:"body,omitempty" jsonschema:"Details, if the summary isn't enough"`
	Tags    []string `json:"tags,omitempty" jsonschema:"A few lowercase keywords"`
	Type    string   `json:"type,omitempty" jsonschema:"One of preference, project, person, life, work, fact, other (default other)"`
}

type UpdateMemoryIn struct {
	ID           string    `json:"id" jsonschema:"Memory id"`
	Title        *string   `json:"title,omitempty"`
	Summary      *string   `json:"summary,omitempty"`
	Body         *string   `json:"body,omitempty"`
	Tags         *[]string `json:"tags,omitempty" jsonschema:"Replaces all tags"`
	Type         *string   `json:"type,omitempty" jsonschema:"One of preference, project, person, life, work, fact, other"`
	SupersededBy string    `json:"superseded_by,omitempty" jsonschema:"Retire this entry in favour of the newer entry with this id"`
}

// memoryEntry is a memory as the assistant reads it.
type memoryEntry struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	Body         string     `json:"body,omitempty"`
	Tags         []string   `json:"tags"`
	Type         string     `json:"type"`
	Source       string     `json:"source,omitempty"`
	SupersededBy *uuid.UUID `json:"superseded_by,omitempty"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

func toMemoryEntry(m store.Memory, withBody bool) memoryEntry {
	e := memoryEntry{ID: m.ID, Title: m.Title, Summary: m.Summary, Tags: m.Tags, Type: m.Type, Source: m.Source,
		SupersededBy: m.SupersededBy, CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), UpdatedAt: m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")}
	if withBody {
		e.Body = m.Body
	}
	return e
}

// memoryOwner is the person whose memory a chat run may use, and the source
// to stamp on what it writes.
func memoryOwner(ctx context.Context, d Deps, c agents.Caller) (memory.Owner, memory.Source, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return memory.Owner{}, memory.Source{}, err
	}
	if s == nil {
		return memory.Owner{}, memory.Source{}, apperr.Conflict("memory tools only work in a chat with a person")
	}
	return memory.Owner{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID},
		memory.Source{Label: "chat " + s.ChatID.String(), ChatID: &s.ChatID, RunID: c.RunID}, nil
}

func searchMemory(ctx context.Context, d Deps, c agents.Caller, in SearchMemoryIn) (any, error) {
	o, _, err := memoryOwner(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 10
	}
	hits, err := memory.New(d.Q).Find(ctx, o, memory.Query{Text: in.Query, Type: in.Type, Limit: in.Limit, IncludeRetired: in.IncludeRetired})
	if err != nil {
		return nil, err
	}
	out := make([]memoryEntry, len(hits))
	for i, h := range hits {
		out[i] = toMemoryEntry(h.Memory, false)
	}
	return map[string]any{"memories": out, "hint": "Open one with read_memory(id)."}, nil
}

func readMemory(ctx context.Context, d Deps, c agents.Caller, in MemoryIDIn) (any, error) {
	o, _, err := memoryOwner(ctx, d, c)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("id", in.ID)
	if err != nil {
		return nil, err
	}
	m, err := memory.New(d.Q).Get(ctx, o, id)
	if err != nil {
		return nil, err
	}
	return toMemoryEntry(m, true), nil
}

func saveMemory(ctx context.Context, d Deps, c agents.Caller, in SaveMemoryIn) (any, error) {
	o, src, err := memoryOwner(ctx, d, c)
	if err != nil {
		return nil, err
	}
	r, err := memory.New(d.Q).Save(ctx, o, memory.Input{Title: in.Title, Summary: in.Summary, Body: in.Body, Tags: in.Tags, Type: in.Type}, src)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": r.Memory.ID, "action": "created", "title": r.Memory.Title,
		"tell_the_person": "Remembered: " + r.Memory.Title + ". Say so in your reply, and that they can ask you to forget it."}
	if r.Updated {
		p := r.Previous
		out["action"] = "updated"
		out["previous"] = map[string]any{"id": p.ID, "title": p.Title, "summary": p.Summary, "type": p.Type}
		out["tell_the_person"] = "Updated what I remember: '" + p.Title + "' is now '" + r.Memory.Title + "'. Say so in your reply; if this wasn't the same thing, save it again with a more distinct title and restore the old entry with update_memory."
	}
	return out, nil
}

func updateMemory(ctx context.Context, d Deps, c agents.Caller, in UpdateMemoryIn) (any, error) {
	o, src, err := memoryOwner(ctx, d, c)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("id", in.ID)
	if err != nil {
		return nil, err
	}
	svc := memory.New(d.Q)
	var m store.Memory
	if in.Title != nil || in.Summary != nil || in.Body != nil || in.Tags != nil || in.Type != nil {
		if m, err = svc.Update(ctx, o, id, memory.Patch{Title: in.Title, Summary: in.Summary, Body: in.Body, Tags: in.Tags, Type: in.Type}, src); err != nil {
			return nil, err
		}
	}
	if s := strings.TrimSpace(in.SupersededBy); s != "" {
		by, err := parseUUID("superseded_by", s)
		if err != nil {
			return nil, err
		}
		if m, err = svc.Supersede(ctx, o, id, &by); err != nil {
			return nil, err
		}
	}
	if m.ID == uuid.Nil {
		return nil, apperr.Invalid("id", "give at least one field to change")
	}
	return map[string]any{"memory": toMemoryEntry(m, false), "tell_the_person": "Updated what I remember: " + m.Title + "."}, nil
}

func forgetMemory(ctx context.Context, d Deps, c agents.Caller, in MemoryIDIn) (any, error) {
	o, _, err := memoryOwner(ctx, d, c)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("id", in.ID)
	if err != nil {
		return nil, err
	}
	svc := memory.New(d.Q)
	m, err := svc.Get(ctx, o, id)
	if err != nil {
		return nil, err
	}
	if err := svc.Forget(ctx, o, id); err != nil {
		return nil, err
	}
	return map[string]any{"forgotten": m.ID, "title": m.Title, "tell_the_person": "Forgot: " + m.Title + "."}, nil
}
