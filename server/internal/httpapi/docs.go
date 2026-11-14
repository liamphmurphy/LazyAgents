package httpapi

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/docs"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
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
	return api.GetMyCanvas200JSONResponse{Id: r.ID, Kind: api.MyCanvasKind(r.Kind), Title: r.Title, ChatId: r.ChatID, ChatTitle: r.ChatTitle, UpdatedAt: r.UpdatedAt, ReviewedSeq: &r.ReviewedSeq}, nil
}

func (s *Server) MarkDocReviewed(ctx context.Context, req api.MarkDocReviewedRequestObject) (api.MarkDocReviewedResponseObject, error) {
	u, _, ws := me(ctx)
	if _, err := s.Q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: req.CanvasId, WorkspaceID: ws, UserID: u.ID}); errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	seq, err := s.Q.MarkMyCanvasReviewed(ctx, store.MarkMyCanvasReviewedParams{Seq: req.Body.Seq, ID: req.CanvasId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("seq", "this document has no version %d", req.Body.Seq)
	} else if err != nil {
		return nil, err
	}
	return api.MarkDocReviewed200JSONResponse{ReviewedSeq: seq}, nil
}

// SaveDocToLibrary keeps a copy of a text canvas as it is now. The blob is
// keyed by its content, so saving unchanged text returns the item it made.
func (s *Server) SaveDocToLibrary(ctx context.Context, req api.SaveDocToLibraryRequestObject) (api.SaveDocToLibraryResponseObject, error) {
	u, _, ws := me(ctx)
	cv, err := s.Q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: req.CanvasId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if cv.Kind != "text" {
		return nil, apperr.Invalid("canvasId", "only a text document can be saved as Markdown")
	}
	cur, err := s.Q.GetUserCanvas(ctx, store.GetUserCanvasParams{ID: cv.ID, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	content := []byte(cur.Body)
	row, err := s.keepExport(ctx, u.ID, ws, cv, exportFile{data: content, contentType: docs.LibraryContentType, filename: docs.LibraryFilename(cv.Title),
		title: docs.LibraryTitle(cv.Title), detail: runs.CountWords(cur.Body)})
	if err != nil {
		return nil, err
	}
	return api.SaveDocToLibrary201JSONResponse(toLibraryItem(row)), nil
}

// PromoteDocToWiki publishes a text canvas's current version as a wiki page.
// It is the person's own action, so the page goes live without an approval,
// and it is a copy: later edits to the document don't move the page.
func (s *Server) PromoteDocToWiki(ctx context.Context, req api.PromoteDocToWikiRequestObject) (api.PromoteDocToWikiResponseObject, error) {
	u, _, ws := me(ctx)
	cv, err := s.Q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: req.CanvasId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if cv.Kind != "text" {
		return nil, apperr.Invalid("canvasId", "only a text document can be published to the wiki")
	}
	cur, err := s.Q.GetUserCanvas(ctx, store.GetUserCanvasParams{ID: cv.ID, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	var requested string
	var replace bool
	if req.Body != nil {
		if req.Body.Path != nil {
			requested = *req.Body.Path
		}
		replace = req.Body.Replace != nil && *req.Body.Replace
	}
	path, err := artifacts.ValidNotePath(docs.WikiPath(requested, cv.Title))
	if err != nil {
		return nil, err
	}
	_, err = s.Q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path, Viewer: pageview.From(ctx).UserID})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if exists && !replace {
		return nil, apperr.Conflict("a wiki page called %q already exists: choose another path, or replace it", path)
	}
	mode := wiki.ModeCreate
	if exists {
		mode = wiki.ModeReplace
	}
	body := docs.WikiBody(cur.Body, cv.Title, time.Now().UTC().Format("2006-01-02"))
	res, err := s.Wiki.SaveAsUser(ctx, ws, u.ID, wiki.Save{Path: path, Body: body, Mode: mode, Audience: "shared", Note: "Promoted from the Doc “" + cv.Title + "”"})
	if err != nil {
		return nil, err
	}
	return api.PromoteDocToWiki201JSONResponse{Path: *res.Note.Path, Created: res.Created}, nil
}
