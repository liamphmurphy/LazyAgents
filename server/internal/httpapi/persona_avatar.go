package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/blob"
	"github.com/liamphmurphy/lazyagents/server/internal/personas"
)

// Persona avatars (ADR Decision 4). They live in the blob bucket under
// personas/<workspace>/<user>/<persona>/<uuid>, outside chat_attachments, so
// the per-person upload cap and the unsent-attachment sweep never see them.
// A replaced or removed avatar's blob is deleted after the row commits.

var errNoBlobs = errors.New("blob storage is not configured")

// readAvatar reads the multipart body's single file part, at most
// MaxAvatarBytes. It stops reading as soon as the file is over the limit and
// refuses any other field, so it never reads an unbounded body.
func readAvatar(mr *multipart.Reader) ([]byte, error) {
	if mr == nil {
		return nil, apperr.Invalid("file", "required")
	}
	part, err := mr.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, apperr.Invalid("file", "required")
	}
	if err != nil {
		return nil, apperr.Invalid("body", "malformed multipart form")
	}
	defer part.Close()
	if name := part.FormName(); name != "file" {
		return nil, apperr.Invalid("file", "send only the file field (got %q)", name)
	}
	b, err := io.ReadAll(io.LimitReader(part, personas.MaxAvatarBytes+1))
	if err != nil {
		return nil, apperr.Invalid("body", "malformed multipart form")
	}
	if len(b) > personas.MaxAvatarBytes {
		return nil, personas.AvatarTooLarge()
	}
	return b, nil
}

// avatarRefused renders a 413 or 415 from CheckAvatar or readAvatar; other
// errors go through the shared error mapping.
func avatarRefused(err error) (api.UploadPersonaAvatarResponseObject, error) {
	var ae *personas.AvatarError
	if !errors.As(err, &ae) {
		return nil, err
	}
	field := "file"
	body := api.Error{Code: api.Invalid, Message: ae.Msg, Field: &field}
	if ae.Status == http.StatusRequestEntityTooLarge {
		return api.UploadPersonaAvatar413JSONResponse(body), nil
	}
	return api.UploadPersonaAvatar415JSONResponse(body), nil
}

func (s *Server) deleteAvatarBlob(ctx context.Context, key *string) {
	if key == nil || s.Blobs == nil {
		return
	}
	// The row no longer points here; a leftover blob is unreachable, so a
	// failure only costs storage.
	if err := s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, *key); err != nil {
		s.Log.Warn("delete persona avatar", "key", *key, "err", err)
	}
}

func (s *Server) UploadPersonaAvatar(ctx context.Context, req api.UploadPersonaAvatarRequestObject) (api.UploadPersonaAvatarResponseObject, error) {
	o := personaOwner(ctx)
	// 404 before reading the body for someone else's persona.
	if _, err := s.personas().Get(ctx, o, req.Id); err != nil {
		return nil, err
	}
	b, err := readAvatar(req.Body)
	if err != nil {
		return avatarRefused(err)
	}
	// The declared part Content-Type and filename are ignored: the type is sniffed.
	ct, err := personas.CheckAvatar(b)
	if err != nil {
		return avatarRefused(err)
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	key := personas.AvatarKey(o, req.Id)
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, bytes.NewReader(b), int64(len(b)), ct); err != nil {
		return nil, err
	}
	p, old, err := s.personas().SetAvatar(ctx, o, req.Id, &key, &ct)
	if err != nil {
		// Deleted meanwhile, or the write failed: nothing points at the new blob.
		s.deleteAvatarBlob(ctx, &key)
		return nil, err
	}
	s.deleteAvatarBlob(ctx, old)
	out, err := s.toPersona(ctx, p)
	return api.UploadPersonaAvatar200JSONResponse(out), err
}

func (s *Server) DeletePersonaAvatar(ctx context.Context, req api.DeletePersonaAvatarRequestObject) (api.DeletePersonaAvatarResponseObject, error) {
	p, old, err := s.personas().SetAvatar(ctx, personaOwner(ctx), req.Id, nil, nil)
	if err != nil {
		return nil, err
	}
	s.deleteAvatarBlob(ctx, old)
	out, err := s.toPersona(ctx, p)
	return api.DeletePersonaAvatar200JSONResponse(out), err
}

func (s *Server) GetPersonaAvatar(ctx context.Context, req api.GetPersonaAvatarRequestObject) (api.GetPersonaAvatarResponseObject, error) {
	p, err := s.personas().Get(ctx, personaOwner(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	// Only ever serve one of the sniffed image types, whatever the row says.
	if p.AvatarKey == nil || p.AvatarContentType == nil || !slices.Contains(personas.AvatarTypes, *p.AvatarContentType) {
		return nil, apperr.ErrNotFound
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	obj, err := s.Blobs.Get(ctx, s.ArtifactBucket, *p.AvatarKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound // replaced or deleted since the row was read
	} else if err != nil {
		return nil, err
	}
	return avatarResponse{body: obj.Body, contentType: *p.AvatarContentType, length: obj.ContentLength}, nil
}

// avatarResponse serves avatar bytes with the stored (sniffed) type and
// headers that stop the browser from treating them as anything but an
// image. The URL carries ?v=<revision>, so a long private cache is safe.
type avatarResponse struct {
	body        io.ReadCloser
	contentType string
	length      int64
}

func (r avatarResponse) VisitGetPersonaAvatarResponse(w http.ResponseWriter) error {
	defer r.body.Close()
	h := w.Header()
	h.Set("Content-Type", r.contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", "private, max-age=86400")
	if r.length > 0 {
		h.Set("Content-Length", fmt.Sprint(r.length))
	}
	w.WriteHeader(http.StatusOK)
	_, err := io.Copy(w, r.body)
	return err
}
