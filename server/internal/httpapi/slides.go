package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/slides"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) slides() *slides.Service { return &slides.Service{Pool: s.Pool, Runs: s.Runs} }

// myDeck is one of the caller's canvases, and only if it's a deck: 404 for
// anyone else's and for documents and mockups.
func (s *Server) myDeck(ctx context.Context, id uuid.UUID) (store.GetMyCanvasRow, error) {
	u, _, ws := me(ctx)
	cv, err := s.Q.GetMyCanvas(ctx, store.GetMyCanvasParams{ID: id, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && cv.Kind != canvas.KindDeck) {
		return cv, apperr.ErrNotFound
	}
	return cv, err
}

func myCanvasOut(cv store.ChatCanvas, chat store.ChatSession) api.MyCanvas {
	return api.MyCanvas{Id: cv.ID, Kind: api.MyCanvasKind(cv.Kind), Title: cv.Title, ChatId: chat.ID, ChatTitle: chat.Title, UpdatedAt: cv.UpdatedAt}
}

// deckCover is the deck cut down to its first shown slide and the images that
// slide uses, so the list can draw a thumbnail without loading every deck.
func deckCover(body string) (string, int) {
	d, err := deck.Parse(body)
	if err != nil || len(d.Slides) == 0 {
		return "", 0
	}
	first := d.Slides[0]
	if i := slices.IndexFunc(d.Slides, func(s deck.Slide) bool { return !s.Hidden }); i >= 0 {
		first = d.Slides[i]
	}
	assets := map[string]deck.Asset{}
	for _, e := range first.Elements {
		if a, ok := d.Assets[e.Asset]; ok {
			assets[e.Asset] = a
		}
	}
	n := len(d.Slides)
	d.Slides, d.Assets = []deck.Slide{first}, assets
	out, err := d.Encode()
	if err != nil {
		return "", n
	}
	return out, n
}

func (s *Server) ListDecks(ctx context.Context, _ api.ListDecksRequestObject) (api.ListDecksResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListUserDecks(ctx, store.ListUserDecksParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out := make([]api.DeckSummary, 0, len(rows))
	for _, r := range rows {
		cover, n := deckCover(r.Body)
		out = append(out, api.DeckSummary{Id: r.ID, Title: r.Title, ChatId: r.ChatID, UpdatedAt: r.UpdatedAt, SlideCount: n, Cover: cover})
	}
	return api.ListDecks200JSONResponse{Decks: out}, nil
}

func (s *Server) CreateDeck(ctx context.Context, req api.CreateDeckRequestObject) (api.CreateDeckResponseObject, error) {
	u, _, ws := me(ctx)
	var title, theme, starter string
	if req.Body != nil {
		if req.Body.Title != nil {
			title = *req.Body.Title
		}
		if req.Body.Theme != nil {
			theme = string(*req.Body.Theme)
		}
		if req.Body.Starter != nil {
			starter = string(*req.Body.Starter)
		}
	}
	var (
		cv   store.ChatCanvas
		chat store.ChatSession
		err  error
	)
	if starter != "" {
		cv, chat, err = s.slides().CreateFromStarter(ctx, ws, u.ID, starter, title, theme)
	} else {
		cv, chat, err = s.slides().Create(ctx, ws, u.ID, title, theme)
	}
	if err != nil {
		return nil, err
	}
	return api.CreateDeck201JSONResponse(myCanvasOut(cv, chat)), nil
}

// ListSlideStarters is the New deck chooser's catalogue. The decks are
// embedded fixtures, so this needs no database.
func (s *Server) ListSlideStarters(ctx context.Context, _ api.ListSlideStartersRequestObject) (api.ListSlideStartersResponseObject, error) {
	out := make([]api.SlideStarter, 0, len(deck.Starters))
	for _, st := range deck.Starters {
		d, err := deck.StarterDeck(st.Key)
		if err != nil {
			return nil, err
		}
		body, err := d.Encode()
		if err != nil {
			return nil, err
		}
		cover, n := deckCover(body)
		out = append(out, api.SlideStarter{Key: st.Key, Name: st.Name, Description: st.Description, SlideCount: n, Cover: cover})
	}
	return api.ListSlideStarters200JSONResponse{Starters: out}, nil
}

func (s *Server) RenameDeck(ctx context.Context, req api.RenameDeckRequestObject) (api.RenameDeckResponseObject, error) {
	cv, err := s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	if _, err := s.slides().Rename(ctx, cv, req.Body.Title); err != nil {
		return nil, err
	}
	cv, err = s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	return api.RenameDeck200JSONResponse{Id: cv.ID, Kind: api.MyCanvasKind(cv.Kind), Title: cv.Title, ChatId: cv.ChatID, ChatTitle: cv.ChatTitle, UpdatedAt: cv.UpdatedAt, ReviewedSeq: &cv.ReviewedSeq}, nil
}

func (s *Server) DeleteDeck(ctx context.Context, req api.DeleteDeckRequestObject) (api.DeleteDeckResponseObject, error) {
	cv, err := s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	u, _, ws := me(ctx)
	// The deck's chat exists only for it, so deleting the chat deletes the
	// canvas, its versions and its asset rows with it. Removing the rows
	// queues their blobs, and the runner's sweep deletes those no other deck
	// (such as a copy) still uses: slides.SweepBlobs.
	if err := s.Runs.DeleteChat(ctx, ws, u.ID, cv.ChatID); err != nil {
		return nil, err
	}
	return api.DeleteDeck204Response{}, nil
}

func (s *Server) DuplicateDeck(ctx context.Context, req api.DuplicateDeckRequestObject) (api.DuplicateDeckResponseObject, error) {
	src, err := s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	u, _, ws := me(ctx)
	cv, chat, err := s.slides().Duplicate(ctx, ws, u.ID, src)
	if err != nil {
		return nil, err
	}
	return api.DuplicateDeck201JSONResponse(myCanvasOut(cv, chat)), nil
}

func (s *Server) UploadDeckAsset(ctx context.Context, req api.UploadDeckAssetRequestObject) (api.UploadDeckAssetResponseObject, error) {
	cv, err := s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	if req.Body == nil {
		return nil, apperr.Invalid("file", "required")
	}
	part, err := req.Body.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, apperr.Invalid("file", "required")
	} else if err != nil {
		return nil, apperr.Invalid("body", "malformed multipart form")
	}
	defer part.Close()
	if part.FormName() != "file" {
		return nil, apperr.Invalid("file", "send only the file field (got %q)", part.FormName())
	}
	u, _, ws := me(ctx)
	a, err := slides.ReadAsset(part, ws, u.ID)
	if err != nil {
		return nil, err
	}
	// The same image already stored for this person costs nothing more.
	have, err := s.Q.UserHasDeckBlob(ctx, store.UserHasDeckBlobParams{WorkspaceID: ws, UserID: u.ID, BlobKey: a.Key})
	if err != nil {
		return nil, err
	}
	if !have {
		generated, err := s.Q.ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			return nil, err
		}
		decks, err := s.Q.DeckAssetBytes(ctx, store.DeckAssetBytesParams{WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			return nil, err
		}
		// Best effort: parallel uploads can each pass this check (as with Library saves).
		if generated+decks+int64(len(a.Bytes)) > runs.MaxGeneratedImageStorageBytes {
			return nil, apperr.Invalid("file", "your Library is full (1 GB): remove decks or items you no longer need")
		}
	}
	// The row goes in before the blob is written, so the sweep sees this
	// reference and leaves the shared blob alone.
	row, err := s.slides().AddAsset(ctx, store.CreateDeckAssetParams{CanvasID: cv.ID, ID: deck.NewID("a"), WorkspaceID: ws, UserID: u.ID,
		BlobKey: a.Key, Mime: a.Mime, Width: int32(a.Width), Height: int32(a.Height), SizeBytes: int64(len(a.Bytes))})
	if err != nil {
		return nil, err
	}
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, a.Key, bytes.NewReader(a.Bytes), int64(len(a.Bytes)), a.Mime); err != nil {
		// Removing the row queues the key, so the sweep deletes anything
		// partly written once no deck uses it. A cancelled request must
		// still remove it.
		_ = s.Q.DeleteDeckAsset(context.WithoutCancel(ctx), store.DeleteDeckAssetParams{CanvasID: cv.ID, ID: row.ID})
		return nil, err
	}
	return api.UploadDeckAsset201JSONResponse{Id: row.ID, Mime: api.DeckAssetMime(row.Mime), W: int(row.Width), H: int(row.Height)}, nil
}

func (s *Server) GetDeckAsset(ctx context.Context, req api.GetDeckAssetRequestObject) (api.GetDeckAssetResponseObject, error) {
	cv, err := s.myDeck(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	a, err := s.Q.GetDeckAsset(ctx, store.GetDeckAssetParams{CanvasID: cv.ID, ID: req.AssetId})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	obj, err := s.Blobs.Get(ctx, s.ArtifactBucket, a.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	// Content-addressed and owner-only, so a long private cache is safe.
	return avatarResponse{body: obj.Body, contentType: a.Mime, length: obj.ContentLength, cache: "private, max-age=31536000, immutable"}, nil
}
