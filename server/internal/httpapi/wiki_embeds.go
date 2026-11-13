package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func toPageAttachment(a store.PageAttachment) api.PageAttachment {
	return api.PageAttachment{Id: a.ID, PageId: a.ArtifactID, Name: a.Name, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
		Sha256: a.Sha256, Href: links.AttachmentHref(a.ID), CreatedAt: a.CreatedAt}
}

func (s *Server) ResolveEmbeds(ctx context.Context, req api.ResolveEmbedsRequestObject) (api.ResolveEmbedsResponseObject, error) {
	_, _, ws := me(ctx)
	es, err := wiki.ResolveEmbeds(ctx, s.Q, ws, req.Params.From, req.Params.Ref)
	if err != nil {
		return nil, err
	}
	return api.ResolveEmbeds200JSONResponse{Embeds: same[[]api.Embed](es)}, nil
}

func (s *Server) ListPageAttachments(ctx context.Context, req api.ListPageAttachmentsRequestObject) (api.ListPageAttachmentsResponseObject, error) {
	_, _, ws := me(ctx)
	viewer := pageview.From(ctx).UserID
	if _, err := s.Q.GetArtifact(ctx, store.GetArtifactParams{ID: req.Id, WorkspaceID: ws, Viewer: viewer}); errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListVisiblePageAttachments(ctx, store.ListVisiblePageAttachmentsParams{ArtifactID: req.Id, WorkspaceID: ws, Viewer: viewer})
	if err != nil {
		return nil, err
	}
	out := api.ListPageAttachments200JSONResponse{Attachments: make([]api.PageAttachment, len(rows))}
	for i, r := range rows {
		out.Attachments[i] = toPageAttachment(r)
	}
	return out, nil
}

func (s *Server) UploadPageAttachment(ctx context.Context, req api.UploadPageAttachmentRequestObject) (api.UploadPageAttachmentResponseObject, error) {
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
	res, err := wiki.AddAttachment(ctx, s.Pool, s.Blobs, s.ArtifactBucket, ws, wiki.Upload{
		PageID: req.Id, Name: part.FileName(), ContentType: part.Header.Get("Content-Type"), Body: part, ByType: "user", ByID: u.ID,
	})
	if err != nil {
		return nil, err
	}
	return api.UploadPageAttachment201JSONResponse(toPageAttachment(res.Attachment)), nil
}

func (s *Server) GetPageAttachmentContent(ctx context.Context, req api.GetPageAttachmentContentRequestObject) (api.GetPageAttachmentContentResponseObject, error) {
	_, _, ws := me(ctx)
	a, err := s.Q.GetVisiblePageAttachment(ctx, store.GetVisiblePageAttachmentParams{ID: req.AttachmentId, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
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
	// The bytes behind a key never change, but access follows the page's audience.
	return avatarResponse{body: obj.Body, contentType: a.ContentType, length: obj.ContentLength, cache: "private, max-age=3600"}, nil
}

// VisitGetPageAttachmentContentResponse serves an attachment with the avatar headers (nosniff, sandboxed).
func (r avatarResponse) VisitGetPageAttachmentContentResponse(w http.ResponseWriter) error {
	return r.write(w)
}
