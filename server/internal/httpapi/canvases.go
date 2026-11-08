package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) ListChatCanvases(ctx context.Context, req api.ListChatCanvasesRequestObject) (api.ListChatCanvasesResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListChatCanvases(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := api.ChatCanvases{Canvases: make([]api.ChatCanvasSummary, 0, len(rows))}
	for _, r := range rows {
		out.Canvases = append(out.Canvases, api.ChatCanvasSummary{Id: r.ID, Kind: api.ChatCanvasSummaryKind(r.Kind), Title: r.Title, Seq: r.Seq, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return api.ListChatCanvases200JSONResponse(out), nil
}

func (s *Server) GetChatCanvas(ctx context.Context, req api.GetChatCanvasRequestObject) (api.GetChatCanvasResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	cv, err := s.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: req.CanvasId, SessionID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cur, err := s.Q.LatestChatCanvasVersion(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	v := cur
	if req.Params.Seq != nil {
		v, err = s.Q.GetChatCanvasVersion(ctx, store.GetChatCanvasVersionParams{CanvasID: cv.ID, Seq: *req.Params.Seq})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	rows, err := s.Q.ListChatCanvasVersions(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	versions := make([]api.ChatCanvasVersion, 0, len(rows))
	for _, r := range rows {
		versions = append(versions, api.ChatCanvasVersion{Seq: r.Seq, Note: r.Note, AuthorType: api.ChatCanvasVersionAuthorType(r.AuthorType), CreatedAt: r.CreatedAt})
	}
	return api.GetChatCanvas200JSONResponse{Id: cv.ID, Kind: api.ChatCanvasKind(cv.Kind), Title: cv.Title, Seq: cur.Seq, CreatedAt: cv.CreatedAt, UpdatedAt: cv.UpdatedAt, ViewingSeq: v.Seq, Body: v.Body, Versions: versions}, nil
}
