package httpapi

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/blob"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Chat attachments (LAZYAGENTS-72) are private to the person who uploaded
// them: every lookup filters on the caller, and someone else's is a 404.

// chatAttachmentKey is where an upload's bytes live in the artifact bucket.
func chatAttachmentKey(ws, user uuid.UUID) string {
	return runs.ChatAttachmentKey(ws, user)
}

func toChatAttachment(a store.ChatAttachment) api.ChatAttachment {
	source := api.ChatAttachmentSource(a.Source)
	return api.ChatAttachment{Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes, Source: &source, CreatedAt: a.CreatedAt}
}

func (s *Server) UploadChatAttachment(ctx context.Context, req api.UploadChatAttachmentRequestObject) (api.UploadChatAttachmentResponseObject, error) {
	u, _, ws := me(ctx)
	f, err := readForm(req.Body)
	if err != nil {
		return nil, err
	}
	defer f.close()
	if f.file == nil {
		return nil, apperr.Invalid("file", "required")
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(f.tmp, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if _, err := f.tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	name := runs.ChatAttachmentName(f.file.Name)
	ct, err := runs.ChatAttachmentType(name, f.file.Size, head[:n])
	if err != nil {
		return nil, err
	}
	// A soft cap: two concurrent uploads can each pass the check.
	stored, err := s.Q.ChatAttachmentBytes(ctx, store.ChatAttachmentBytesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if err := runs.CheckChatAttachmentQuota(stored, f.file.Size); err != nil {
		return nil, err
	}
	key := chatAttachmentKey(ws, u.ID)
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, f.tmp, f.file.Size, ct); err != nil {
		return nil, err
	}
	a, err := s.Q.CreateChatAttachment(ctx, store.CreateChatAttachmentParams{WorkspaceID: ws, UserID: u.ID, Filename: name, ContentType: ct, SizeBytes: f.file.Size, BlobKey: key})
	if err != nil {
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, key)
		return nil, err
	}
	return api.UploadChatAttachment201JSONResponse(toChatAttachment(a)), nil
}

func (s *Server) GetChatAttachmentContent(ctx context.Context, req api.GetChatAttachmentContentRequestObject) (api.GetChatAttachmentContentResponseObject, error) {
	u, _, ws := me(ctx)
	a, err := s.Q.GetChatAttachment(ctx, store.GetChatAttachmentParams{ID: req.AttachmentId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, a.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return contentResponse{body: o.Body, closer: o.Body, contentType: a.ContentType, length: a.SizeBytes, filename: a.Filename, inline: true}, nil
}

// messageAttachments lists the files attached to each of msgs, by message id.
func (s *Server) messageAttachments(ctx context.Context, msgs []store.ChatMessage) (map[uuid.UUID][]api.ChatAttachment, error) {
	out := map[uuid.UUID][]api.ChatAttachment{}
	var ids []uuid.UUID
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" { // assistant messages carry generated images
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.Q.ListChatAttachmentsForMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, a := range rows {
		out[*a.MessageID] = append(out[*a.MessageID], toChatAttachment(a))
	}
	return out, nil
}
