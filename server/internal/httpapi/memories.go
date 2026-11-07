package httpapi

import (
	"context"
	"time"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/memory"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Memory manager endpoints. Memory is strictly per person: every call is
// scoped to the signed-in user, so another person's entries are a 404.

const memoryPageSize = 50

func memoryOwner(ctx context.Context) memory.Owner {
	u, _, ws := me(ctx)
	return memory.Owner{WorkspaceID: ws, UserID: u.ID}
}

func toMemory(m store.Memory) api.Memory {
	return api.Memory{Id: m.ID, Title: m.Title, Summary: m.Summary, Body: m.Body, Tags: m.Tags, Type: m.Type, Source: m.Source,
		SourceChatId: m.SourceChatID, SourceRunId: m.SourceRunID, SupersededBy: m.SupersededBy, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func (s *Server) ListMemories(ctx context.Context, req api.ListMemoriesRequestObject) (api.ListMemoriesResponseObject, error) {
	o := memoryOwner(ctx)
	q := memory.Query{Limit: memoryPageSize + 1}
	if req.Params.Q != nil {
		q.Text = *req.Params.Q
	}
	if req.Params.Type != nil {
		q.Type = *req.Params.Type
	}
	if req.Params.IncludeRetired != nil {
		q.IncludeRetired = *req.Params.IncludeRetired
	}
	if req.Params.Offset != nil {
		q.Offset = *req.Params.Offset
	}
	svc := memory.New(s.Q)
	hits, err := svc.Find(ctx, o, q)
	if err != nil {
		return nil, err
	}
	total, err := svc.Count(ctx, o, q)
	if err != nil {
		return nil, err
	}
	out := api.ListMemories200JSONResponse{Memories: make([]api.Memory, 0, len(hits)), Total: total, HasMore: len(hits) > memoryPageSize}
	for i, h := range hits {
		if i == memoryPageSize {
			break
		}
		out.Memories = append(out.Memories, toMemory(h.Memory))
	}
	return out, nil
}

func (s *Server) GetMemory(ctx context.Context, req api.GetMemoryRequestObject) (api.GetMemoryResponseObject, error) {
	m, err := memory.New(s.Q).Get(ctx, memoryOwner(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetMemory200JSONResponse(toMemory(m)), nil
}

func (s *Server) UpdateMemory(ctx context.Context, req api.UpdateMemoryRequestObject) (api.UpdateMemoryResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	b := req.Body
	restore := b.Restore != nil && *b.Restore
	if restore && b.SupersededBy != nil {
		return nil, apperr.Invalid("restore", "give superseded_by or restore, not both")
	}
	o, svc := memoryOwner(ctx), memory.New(s.Q)
	m, err := svc.Get(ctx, o, req.Id)
	if err != nil {
		return nil, err
	}
	if b.Title != nil || b.Summary != nil || b.Body != nil || b.Tags != nil || b.Type != nil {
		// A person's edit keeps the entry's original source.
		if m, err = svc.Update(ctx, o, req.Id, memory.Patch{Title: b.Title, Summary: b.Summary, Body: b.Body, Tags: b.Tags, Type: b.Type}, memory.Source{}); err != nil {
			return nil, err
		}
	}
	switch {
	case b.SupersededBy != nil:
		m, err = svc.Supersede(ctx, o, req.Id, b.SupersededBy)
	case restore:
		m, err = svc.Supersede(ctx, o, req.Id, nil)
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateMemory200JSONResponse(toMemory(m)), nil
}

func (s *Server) DeleteMemory(ctx context.Context, req api.DeleteMemoryRequestObject) (api.DeleteMemoryResponseObject, error) {
	if err := memory.New(s.Q).Forget(ctx, memoryOwner(ctx), req.Id); err != nil {
		return nil, err
	}
	return api.DeleteMemory204Response{}, nil
}

func (s *Server) DeleteAllMemories(ctx context.Context, req api.DeleteAllMemoriesRequestObject) (api.DeleteAllMemoriesResponseObject, error) {
	if !req.Params.Confirm {
		return nil, apperr.Invalid("confirm", "must be true to delete every memory")
	}
	n, err := memory.New(s.Q).DeleteAll(ctx, memoryOwner(ctx))
	if err != nil {
		return nil, err
	}
	return api.DeleteAllMemories200JSONResponse{Deleted: n}, nil
}

func (s *Server) ExportMemories(ctx context.Context, _ api.ExportMemoriesRequestObject) (api.ExportMemoriesResponseObject, error) {
	rows, err := memory.New(s.Q).Export(ctx, memoryOwner(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ExportMemories200JSONResponse{ExportedAt: time.Now().UTC(), Memories: make([]api.Memory, len(rows))}
	for i, m := range rows {
		out.Memories[i] = toMemory(m)
	}
	return out, nil
}
