// Package slides is the Slides app (ADR: Slides deck model and export): decks
// the person builds and presents with a persona. A deck is a canvas of kind
// "deck" in its own chat in the person's Slides space, like a Docs document;
// its images are deck_assets rows over content-addressed blobs.
package slides

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// AppName marks the Slides app's space in chat_spaces.app.
const AppName = "slides"

// SpaceInstructions reach every chat the Slides app starts.
const SpaceInstructions = `This chat belongs to one slide deck in the person's Slides app: the deck canvas in this chat. They see the deck in an editor and talk to you from a panel beside it. Work on that deck: canvas_read it (the body is the deck JSON), then change it with canvas_patch deck ops (insert_slide, set_slide, move_slide, remove_slide, insert_element, set_element, remove_element, set_theme), never by pasting JSON into your reply.
Slides are 1920x1080 units, origin top-left. Text is paragraphs of runs; a run's size is in units (48 units = 24 pt). Use theme colour names (text, heading, accent, accent2, muted, surface, background) rather than hex, keep text short enough to fit its box (about 6 bullets of under 12 words on a body slide), and leave ids off new slides and elements: the server assigns them. Layouts: title, title-body, two-column, image-right, section, blank; insert_slide with only a layout gets that layout's empty placeholders, which you then fill with set_element. The person reviews your changes slide by slide before they're kept. Keep replies short, saying what you changed.`

// UntitledTitle names a deck started without a title.
const UntitledTitle = "Untitled deck"

// MaxAssetBytes caps one uploaded image.
const MaxAssetBytes = 10 << 20

// MaxAssetEdge caps an image's longest side in pixels; the editor downscales
// larger images before uploading.
const MaxAssetEdge = 8192

type Service struct {
	Pool *pgxpool.Pool
	Runs *runs.Service
}

// EnsureSpace is the person's Slides space, made the first time it's needed.
func (s *Service) EnsureSpace(ctx context.Context, ws, user uuid.UUID) (store.ChatSpace, error) {
	q := store.New(s.Pool)
	sp, err := q.GetAppSpace(ctx, store.GetAppSpaceParams{WorkspaceID: ws, UserID: user, AppName: AppName})
	if !errors.Is(err, pgx.ErrNoRows) {
		return sp, err
	}
	return q.CreateAppSpace(ctx, store.CreateAppSpaceParams{WorkspaceID: ws, UserID: user, Name: "Slides", Color: "violet",
		Description: "Slide decks you build with the assistant.", Instructions: SpaceInstructions, AppName: AppName})
}

// CleanTitle collapses whitespace and checks the length.
func CleanTitle(title string) (string, error) {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = UntitledTitle
	}
	if utf8.RuneCountInString(title) > 200 {
		return "", apperr.Invalid("title", "must be at most 200 characters")
	}
	return title, nil
}

// start makes a chat in the Slides space holding a deck canvas with body.
func (s *Service) start(ctx context.Context, ws, user uuid.UUID, title, body, note string) (store.ChatCanvas, store.ChatSession, error) {
	sp, err := s.EnsureSpace(ctx, ws, user)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	chat, err := s.Runs.StartChat(ctx, ws, user, runs.ChatInput{Title: title, SpaceID: &sp.ID})
	if err != nil {
		return store.ChatCanvas{}, chat, err
	}
	cv, _, err := (&canvas.Service{Pool: s.Pool}).Create(ctx, chat.ID, canvas.New{Kind: canvas.KindDeck, Title: title, Body: body, Note: note}, canvas.Actor{Type: "user", ID: user})
	return cv, chat, err
}

// Create starts a deck with one title slide in a built-in theme.
func (s *Service) Create(ctx context.Context, ws, user uuid.UUID, title, theme string) (store.ChatCanvas, store.ChatSession, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	slideTitle := title
	if title == UntitledTitle {
		slideTitle = ""
	}
	body, err := deck.New(slideTitle, theme).Encode()
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	return s.start(ctx, ws, user, title, body, "")
}

// Duplicate copies a deck's current version and its images into a new deck
// and chat. Slide and element ids are kept: they only need to be unique
// within a deck.
func (s *Service) Duplicate(ctx context.Context, ws, user uuid.UUID, src store.GetMyCanvasRow) (store.ChatCanvas, store.ChatSession, error) {
	q := store.New(s.Pool)
	cur, err := q.LatestChatCanvasVersion(ctx, src.ID)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	title := "Copy of " + src.Title
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200])
	}
	cv, chat, err := s.start(ctx, ws, user, title, cur.Body, fmt.Sprintf("Copied from %s", src.Title))
	if err != nil {
		return cv, chat, err
	}
	return cv, chat, q.CopyDeckAssets(ctx, store.CopyDeckAssetsParams{ToCanvas: cv.ID, FromCanvas: src.ID})
}

// Rename sets a deck's title. The deck's chat keeps its first title: the
// app never shows it, and renaming a chat waits for any running turn.
func (s *Service) Rename(ctx context.Context, cv store.GetMyCanvasRow, title string) (string, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return "", err
	}
	if err := store.New(s.Pool).RenameCanvas(ctx, store.RenameCanvasParams{ID: cv.ID, Title: title}); err != nil {
		return "", err
	}
	return title, nil
}

// Asset is an image checked and ready to store.
type Asset struct {
	Mime          string
	Width, Height int
	Bytes         []byte
	Key           string
}

// ReadAsset reads at most MaxAssetBytes and checks the bytes are a PNG or JPEG
// of a sane size. The type comes from the bytes, never from the client.
func ReadAsset(r io.Reader, ws, user uuid.UUID) (Asset, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxAssetBytes+1))
	if err != nil {
		return Asset{}, apperr.Invalid("file", "couldn't read the upload")
	}
	if len(b) == 0 {
		return Asset{}, apperr.Invalid("file", "the file is empty")
	}
	if len(b) > MaxAssetBytes {
		return Asset{}, apperr.Invalid("file", "images are at most 10 MB")
	}
	mime := http.DetectContentType(b)
	if mime != "image/png" && mime != "image/jpeg" {
		return Asset{}, apperr.Invalid("file", "use a PNG or JPEG image")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return Asset{}, apperr.Invalid("file", "this image couldn't be read")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxAssetEdge || cfg.Height > MaxAssetEdge {
		return Asset{}, apperr.Invalid("file", "images are at most %d pixels on a side", MaxAssetEdge)
	}
	key := fmt.Sprintf("slides/%s/%s/%x", ws, user, sha256.Sum256(b))
	return Asset{Mime: mime, Width: cfg.Width, Height: cfg.Height, Bytes: b, Key: key}, nil
}
