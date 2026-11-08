package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The Library is private to its owner like chat attachments: every lookup
// filters on the caller, and someone else's item is a 404.

const libraryPageSize = 60

func toLibraryItem(l store.ListLibraryItemsRow) api.LibraryItem {
	return api.LibraryItem{
		Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType, SizeBytes: l.SizeBytes,
		ChatId: l.ChatID, ChatTitle: l.ChatTitle, ProjectKey: l.ProjectKey, PersonaId: l.PersonaID, PersonaName: l.PersonaName, CreatedAt: l.CreatedAt,
	}
}

func (s *Server) ListLibraryItems(ctx context.Context, req api.ListLibraryItemsRequestObject) (api.ListLibraryItemsResponseObject, error) {
	u, _, ws := me(ctx)
	p := store.ListLibraryItemsParams{WorkspaceID: ws, UserID: u.ID, RowLimit: libraryPageSize + 1}
	if req.Params.Kind != nil {
		k := string(*req.Params.Kind)
		p.Kind = &k
	}
	if req.Params.Offset != nil {
		p.RowOffset = int32(*req.Params.Offset)
	}
	rows, err := s.Q.ListLibraryItems(ctx, p)
	if err != nil {
		return nil, err
	}
	out := api.ListLibraryItems200JSONResponse{Items: make([]api.LibraryItem, 0, len(rows)), HasMore: len(rows) > libraryPageSize}
	for i, r := range rows {
		if i == libraryPageSize {
			break
		}
		out.Items = append(out.Items, toLibraryItem(r))
	}
	return out, nil
}

func (s *Server) GetLibraryItemContent(ctx context.Context, req api.GetLibraryItemContentRequestObject) (api.GetLibraryItemContentResponseObject, error) {
	u, _, ws := me(ctx)
	l, err := s.Q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: req.ItemId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, l.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	download := req.Params.Download != nil && *req.Params.Download
	return contentResponse{body: o.Body, closer: o.Body, contentType: l.ContentType, length: l.SizeBytes, filename: l.Filename, inline: !download}, nil
}

// DeleteLibraryItem removes the row, then the object unless a chat still
// shows it. Two deletes racing (this and the attachment sweep) can each see
// the other's row and leave the object behind; that only leaks storage.
func (s *Server) DeleteLibraryItem(ctx context.Context, req api.DeleteLibraryItemRequestObject) (api.DeleteLibraryItemResponseObject, error) {
	u, _, ws := me(ctx)
	d, err := s.Q.DeleteLibraryItem(ctx, store.DeleteLibraryItemParams{ID: req.ItemId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if d.Orphaned {
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, d.BlobKey)
	}
	return api.DeleteLibraryItem204Response{}, nil
}
