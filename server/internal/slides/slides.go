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
	"time"
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
Slides are 1920x1080 units, origin top-left. Text is paragraphs of runs; a run's size is in units (48 units = 24 pt). Paragraphs may set align (left, center, right, justify) and lineSpacing (0.8 to 3); runs may set bold, italic, underline, strike, color and highlight. Use theme colour names (text, heading, accent, accent2, muted, surface, background) rather than hex, keep text short enough to fit its box (about 6 bullets of under 12 words on a body slide), and leave ids off new slides and elements: the server assigns them. Shape elements (type shape, with fill, stroke, strokeWidth, rotation and text) take a PowerPoint preset name: rect, roundRect, ellipse, triangle, rtTriangle, diamond, pentagon, hexagon, octagon, rightArrow, leftArrow, upArrow, downArrow, chevron, homePlate, wedgeRectCallout, wedgeRoundRectCallout, star5, plus, donut or heart. To style a shape or line, set_element with {"style": name}: filled-accent, filled-accent2, outline-accent, subtle, emphasis or ghost (they use theme colours, so they follow theme changes). To switch the deck's theme, set_theme with {"preset": name}: a built-in theme (light, dark, paper, contrast, dusk, ocean, forest, sunset, mono, blueprint) or one the person saved, by its name. Layouts: title, title-body, two-column, three-column, comparison (label and label2 head body and body2), image-right, image-left, image-full (title over a full-bleed image), quote (body is the quote, subtitle the attribution), big-number (title is the stat, subtitle the caption), agenda (numbered body), section, closing, blank; insert_slide with only a layout gets that layout's empty placeholders, which you then fill with set_element. An image placeholder is an empty text element the person fills from the editor; leave it alone. When the person gives an outline (pasted in chat, or "Draft slides from this outline" from the New deck chooser), draft it: one slide per top-level line with its indented lines as that slide's points, inserted after the existing slides in one canvas_patch, choosing the layout that suits each (agenda for a list of topics, big-number for a stat, quote, comparison, three-column, section for a heading alone) and filling every placeholder you keep; don't change or remove existing slides unless asked. The person reviews your changes slide by slide before they're kept. Keep replies short, saying what you changed.`

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

// CreateFromStarter starts a deck from a starter deck (deck.Starters) in a
// built-in theme, with fresh ids. An empty title takes the starter's name and
// keeps its sample title slide; a title replaces the first slide's title.
func (s *Service) CreateFromStarter(ctx context.Context, ws, user uuid.UUID, starter, title, theme string) (store.ChatCanvas, store.ChatSession, error) {
	st, ok := deck.StarterNamed(starter)
	if !ok {
		return store.ChatCanvas{}, store.ChatSession{}, apperr.Invalid("starter", "is not a starter deck")
	}
	title, err := CleanTitle(title)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	slideTitle := title
	if title == UntitledTitle {
		title, slideTitle = st.Name, ""
	}
	d, err := deck.FromStarter(st.Key, deck.ThemeNamed(theme), slideTitle)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	body, err := d.Encode()
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	return s.start(ctx, ws, user, title, body, "Started from the "+st.Name+" starter")
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

// AddAsset lists an image under a deck, ready for its blob to be written.
// It holds the blob's lock while it inserts the row, so a sweep either sees
// this reference and keeps the blob, or has deleted the blob before the
// caller writes it again.
func (s *Service) AddAsset(ctx context.Context, p store.CreateDeckAssetParams) (store.DeckAsset, error) {
	var row store.DeckAsset
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockDeckBlob(ctx, p.BlobKey); err != nil {
			return err
		}
		var err error
		row, err = q.CreateDeckAsset(ctx, p)
		return err
	})
	return row, err
}

// BlobSweepDelay is how long a released image blob waits before the sweep
// may delete it.
const BlobSweepDelay = time.Hour

// blobSweepKeyTimeout bounds checking and deleting one blob.
const blobSweepKeyTimeout = 30 * time.Second

// BlobDeleter deletes blobs (blob.Store).
type BlobDeleter interface {
	Delete(ctx context.Context, bucket, key string) error
}

// SweepBlobs deletes up to limit image blobs released before cutoff that no
// deck uses any more, and returns how many it deleted. Deleting a deck, its
// chat (or a failed upload) queues its blobs (a trigger on deck_assets); a
// blob a copy or another deck still uses is dequeued and kept. A blob whose
// delete fails stays queued, at the back, for a later sweep.
func SweepBlobs(ctx context.Context, pool *pgxpool.Pool, blobs BlobDeleter, bucket string, cutoff time.Time, limit int32) (int, error) {
	keys, err := store.New(pool).DueDeckBlobSweeps(ctx, store.DueDeckBlobSweepsParams{Before: cutoff, RowLimit: limit})
	if err != nil {
		return 0, err
	}
	deleted := 0
	var errs []error
	for _, key := range keys {
		if ctx.Err() != nil {
			break
		}
		gone := false
		// Bounded, so a hung store can't hold the key's lock (and uploads of
		// that image) or the runner's sweep indefinitely.
		kctx, cancel := context.WithTimeout(ctx, blobSweepKeyTimeout)
		err := pgx.BeginFunc(kctx, pool, func(tx pgx.Tx) error {
			ctx := kctx
			q := store.New(tx)
			if err := q.LockDeckBlob(ctx, key); err != nil {
				return err
			}
			unused, err := q.TakeDeckBlobSweep(ctx, store.TakeDeckBlobSweepParams{BlobKey: key, Before: cutoff})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !unused) {
				return err // another sweep took it, it was released again, or it's in use
			} else if err != nil {
				return err
			}
			// Still holding the lock, so no upload can take the key until
			// the blob is gone; a failure rolls the dequeue back.
			if err := blobs.Delete(ctx, bucket, key); err != nil {
				return err
			}
			gone = true
			return nil
		})
		cancel()
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
			if rerr := store.New(pool).RequeueDeckBlobSweep(ctx, key); rerr != nil {
				errs = append(errs, rerr)
			}
			continue
		}
		if gone {
			deleted++
		}
	}
	return deleted, errors.Join(errs...)
}
