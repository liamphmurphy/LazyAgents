package wiki

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Page attachments (images, PDF, audio, video) are embedded as ![[name]].
// Their blobs are content-addressed, so the same file on many pages is one
// object; the runner's sweep deletes an attachment nothing embeds once
// AttachmentSweepDelay has passed, and then its blob once nothing uses it.

// MaxAttachmentBytes bounds one attachment.
const MaxAttachmentBytes = 50 << 20

// AttachmentSweepDelay is how long an attachment no page embeds is kept, so
// a file uploaded before the page that embeds it is saved survives.
const AttachmentSweepDelay = time.Hour

// AttachmentBlobs stores attachment bytes.
type AttachmentBlobs interface {
	Put(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) error
}

// Upload is a file to attach to a page.
type Upload struct {
	PageID      uuid.UUID
	Name        string
	ContentType string // as declared by the client; sniffed when missing or generic
	Body        io.Reader
	ByType      string // user or agent
	ByID        uuid.UUID
}

// AttachmentResult is the stored attachment and whether an identical file
// was already stored (no new object written).
type AttachmentResult struct {
	Attachment store.PageAttachment
	Reused     bool
}

// AttachmentBlobKey is where a workspace's attachment with this hash lives.
func AttachmentBlobKey(ws uuid.UUID, sum string) string { return fmt.Sprintf("pages/%s/%s", ws, sum) }

func allowedAttachmentType(ct string) bool {
	return ct == "application/pdf" || strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") ||
		(strings.HasPrefix(ct, "image/") && ct != "image/svg+xml") // SVG can carry script
}

func attachmentType(name, declared string, head []byte) string {
	norm := func(ct string) string {
		ct, _, _ = mime.ParseMediaType(ct)
		return strings.ToLower(ct)
	}
	if ct := norm(http.DetectContentType(head)); allowedAttachmentType(ct) {
		return ct
	}
	for _, ct := range []string{declared, mime.TypeByExtension(strings.ToLower(path.Ext(name)))} {
		// Audio and video containers don't always sniff; trust the name or
		// the client only for those, never to turn text into an image.
		if ct = norm(ct); strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") {
			return ct
		}
	}
	return ""
}

// AttachmentName is a file name as embeds use it: its last path segment,
// with the characters links can't contain replaced.
func AttachmentName(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune("[]#|^/", r) || r < 0x20 {
			return '-'
		}
		return r
	}, name)
	if name == "." || name == "" {
		return ""
	}
	if r := []rune(name); len(r) > 255 {
		ext := path.Ext(name)
		name = string(r[:255-len([]rune(ext))]) + ext
	}
	return name
}

// AddAttachment stores a file on a page the viewer in ctx may see. An
// identical file with the same name on that page is returned as is; another
// file with that name replaces it. Ghost ![[name]] links resolve afterwards.
func AddAttachment(ctx context.Context, pool *pgxpool.Pool, blobs AttachmentBlobs, bucket string, ws uuid.UUID, up Upload) (AttachmentResult, error) {
	name := AttachmentName(up.Name)
	if name == "" || !links.IsFileName(name) {
		return AttachmentResult{}, apperr.Invalid("file", "needs a file name with an extension, like diagram.png")
	}
	data, err := io.ReadAll(io.LimitReader(up.Body, MaxAttachmentBytes+1))
	if err != nil {
		return AttachmentResult{}, err
	}
	if len(data) == 0 {
		return AttachmentResult{}, apperr.Invalid("file", "is empty")
	}
	if len(data) > MaxAttachmentBytes {
		return AttachmentResult{}, apperr.Invalid("file", "attachments are limited to 50 MB")
	}
	ct := attachmentType(name, up.ContentType, data[:min(len(data), 512)])
	if ct == "" {
		return AttachmentResult{}, apperr.Invalid("file", "attach images, PDF, audio or video")
	}
	h := sha256.Sum256(data)
	sum := hex.EncodeToString(h[:])
	key := AttachmentBlobKey(ws, sum)

	var res AttachmentResult
	stored := false
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		page, err := q.GetArtifact(ctx, store.GetArtifactParams{ID: up.PageID, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && page.Type != "note") {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		// Taken before the row goes in, so the sweep can't delete the blob
		// between our check and our reference.
		if err := q.LockDeckBlob(ctx, key); err != nil {
			return err
		}
		if stored, err = q.PageBlobInUse(ctx, key); err != nil {
			return err
		}
		old, err := q.GetPageAttachmentByName(ctx, store.GetPageAttachmentByNameParams{ArtifactID: page.ID, Name: name})
		switch {
		case err == nil && old.Sha256 == sum:
			res = AttachmentResult{Attachment: old, Reused: true}
			return nil
		case err == nil:
			// Same name, new content: links to the name now show this file.
			if err := q.DeletePageAttachment(ctx, old.ID); err != nil {
				return err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		row, err := q.InsertPageAttachment(ctx, store.InsertPageAttachmentParams{
			WorkspaceID: ws, ArtifactID: page.ID, Name: name, ContentType: ct, SizeBytes: int64(len(data)),
			Sha256: sum, BlobKey: key, CreatedByType: up.ByType, CreatedByID: up.ByID,
		})
		res = AttachmentResult{Attachment: row, Reused: stored}
		return err
	})
	if err != nil {
		return AttachmentResult{}, err
	}
	if !stored {
		if err := blobs.Put(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), ct); err != nil {
			// Removing the row queues the key for the blob sweep.
			if !res.Reused {
				_ = store.New(pool).DeletePageAttachment(context.WithoutCancel(ctx), res.Attachment.ID)
			}
			return AttachmentResult{}, err
		}
	}
	if err := links.ResolvePending(ctx, store.New(pool), ws); err != nil {
		return AttachmentResult{}, err
	}
	return res, nil
}

// SweepAttachments deletes up to limit attachments created before cutoff
// that no link embeds; their blobs go to the blob sweep (slides.SweepBlobs),
// which deletes each once no deck, design or page attachment uses it.
func SweepAttachments(ctx context.Context, q *store.Queries, cutoff time.Time, limit int32) (int, error) {
	ids, err := q.SweepUnusedPageAttachments(ctx, store.SweepUnusedPageAttachmentsParams{Before: cutoff, RowLimit: limit})
	return len(ids), err
}
