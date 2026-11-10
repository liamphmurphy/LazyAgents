package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/docs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) CreateDoc(ctx context.Context, req api.CreateDocRequestObject) (api.CreateDocResponseObject, error) {
	u, _, ws := me(ctx)
	var title string
	if req.Body != nil && req.Body.Title != nil {
		title = *req.Body.Title
	}
	if len([]rune(title)) > 200 {
		return nil, apperr.Invalid("title", "must be at most 200 characters")
	}
	cv, chat, err := (&docs.Service{Pool: s.Pool, Runs: s.Runs}).Create(ctx, ws, u.ID, title)
	if err != nil {
		return nil, err
	}
	return api.CreateDoc201JSONResponse{Id: cv.ID, Kind: api.MyCanvasKind(cv.Kind), Title: cv.Title, ChatId: chat.ID, ChatTitle: chat.Title, UpdatedAt: cv.UpdatedAt}, nil
}

func (s *Server) GetMyCanvas(ctx context.Context, req api.GetMyCanvasRequestObject) (api.GetMyCanvasResponseObject, error) {
	u, _, ws := me(ctx)
	r, err := s.Q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: req.CanvasId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return api.GetMyCanvas200JSONResponse{Id: r.ID, Kind: api.MyCanvasKind(r.Kind), Title: r.Title, ChatId: r.ChatID, ChatTitle: r.ChatTitle, UpdatedAt: r.UpdatedAt}, nil
}
