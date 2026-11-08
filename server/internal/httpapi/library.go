package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The Library is private to its owner like chat attachments: every lookup
// filters on the caller, and someone else's item is a 404.

const libraryPageSize = 60

const libraryDiagramType = "image/svg+xml"

func toLibraryItem(l store.ListLibraryItemsRow) api.LibraryItem {
	out := api.LibraryItem{
		Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType, SizeBytes: l.SizeBytes,
		ChatId: l.ChatID, ChatTitle: l.ChatTitle, ProjectKey: l.ProjectKey, PersonaId: l.PersonaID, PersonaName: l.PersonaName, CreatedAt: l.CreatedAt,
	}
	if l.Kind == "link" {
		out.Url, out.Note, out.Site, out.VideoId, out.Summary = &l.Url, &l.Note, &l.Site, &l.VideoID, &l.Summary
		status := api.LibraryLinkStatus(l.LinkStatus)
		out.LinkStatus = &status
	}
	source := api.LibraryItemSource(l.Source)
	out.Source = &source
	// A persona reference whose persona was deleted stands for nobody.
	if l.Reference != nil && (*l.Reference == "self" || l.ReferencePersonaID != nil) {
		r := api.LibraryReferenceRole(*l.Reference)
		out.Reference, out.ReferencePersonaId, out.ReferencePersonaName = &r, l.ReferencePersonaID, l.ReferencePersonaName
	}
	return out
}

func (s *Server) ListLibraryItems(ctx context.Context, req api.ListLibraryItemsRequestObject) (api.ListLibraryItemsResponseObject, error) {
	u, _, ws := me(ctx)
	p := store.ListLibraryItemsParams{WorkspaceID: ws, UserID: u.ID, RowLimit: libraryPageSize + 1}
	if req.Params.Kind != nil {
		k := string(*req.Params.Kind)
		p.Kind = &k
	}
	if req.Params.Offset != nil {
		p.RowOffset = int32(*req.Params.Offset)
	}
	rows, err := s.Q.ListLibraryItems(ctx, p)
	if err != nil {
		return nil, err
	}
	out := api.ListLibraryItems200JSONResponse{Items: make([]api.LibraryItem, 0, len(rows)), HasMore: len(rows) > libraryPageSize}
	for i, r := range rows {
		if i == libraryPageSize {
			break
		}
		out.Items = append(out.Items, toLibraryItem(r))
	}
	return out, nil
}

func (s *Server) GetLibraryItemContent(ctx context.Context, req api.GetLibraryItemContentRequestObject) (api.GetLibraryItemContentResponseObject, error) {
	u, _, ws := me(ctx)
	l, err := s.Q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: req.ItemId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && l.BlobKey == "") {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, l.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	download := req.Params.Download != nil && *req.Params.Download
	return contentResponse{body: o.Body, closer: o.Body, contentType: l.ContentType, length: l.SizeBytes, filename: l.Filename, inline: !download}, nil
}

// DeleteLibraryItem removes the row, then the object unless a chat still
// shows it. Two deletes racing (this and the attachment sweep) can each see
// the other's row and leave the object behind; that only leaks storage.
func (s *Server) DeleteLibraryItem(ctx context.Context, req api.DeleteLibraryItemRequestObject) (api.DeleteLibraryItemResponseObject, error) {
	u, _, ws := me(ctx)
	d, err := s.Q.DeleteLibraryItem(ctx, store.DeleteLibraryItemParams{ID: req.ItemId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	// A link saved without an image has no object.
	if d.Orphaned && d.BlobKey != "" {
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, d.BlobKey)
	}
	return api.DeleteLibraryItem204Response{}, nil
}

// UploadLibraryItem stores an image of the person's own, e.g. their photo for
// "this is me". Uploads share the generated-image cap: both are what the
// Library keeps for good.
func (s *Server) UploadLibraryItem(ctx context.Context, req api.UploadLibraryItemRequestObject) (api.UploadLibraryItemResponseObject, error) {
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
	if _, ok := runs.GeneratedImageType(head[:n]); !ok {
		return nil, apperr.Invalid("file", "%s isn't supported: the Library takes PNG, JPEG, GIF or WebP images", name)
	}
	// A soft cap: two concurrent uploads can each pass the check.
	stored, err := s.Q.ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if stored+f.file.Size > runs.MaxGeneratedImageStorageBytes {
		return nil, apperr.Invalid("file", "your Library is full (1 GB): remove images you no longer need")
	}
	title := strings.TrimSpace(strings.TrimSuffix(name, path.Ext(name)))
	if title == "" {
		title = "Uploaded image"
	}
	if r := []rune(title); len(r) > runs.MaxLibraryTitleRunes {
		title = string(r[:runs.MaxLibraryTitleRunes])
	}
	key := runs.ChatAttachmentKey(ws, u.ID)
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, f.tmp, f.file.Size, ct); err != nil {
		return nil, err
	}
	l, err := s.Q.CreateUploadedLibraryItem(ctx, store.CreateUploadedLibraryItemParams{WorkspaceID: ws, UserID: u.ID, Title: title, Filename: name, ContentType: ct, SizeBytes: f.file.Size, BlobKey: key})
	if err != nil {
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, key)
		return nil, err
	}
	source := api.LibraryItemSource(l.Source)
	return api.UploadLibraryItem201JSONResponse{Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType, SizeBytes: l.SizeBytes,
		Source: &source, CreatedAt: l.CreatedAt}, nil
}

// SaveLibraryDiagram keeps an SVG from one of the caller's assistant replies.
// The object key comes from the content, so a repeat save (or two racing)
// lands on the same object and row.
func (s *Server) SaveLibraryDiagram(ctx context.Context, req api.SaveLibraryDiagramRequestObject) (api.SaveLibraryDiagramResponseObject, error) {
	u, _, ws := me(ctx)
	m, err := s.Q.GetChatMessage(ctx, req.Body.MessageId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	chat, err := s.ownChat(ctx, m.SessionID)
	if err != nil {
		return nil, err
	}
	if m.Role != "assistant" {
		return nil, apperr.Invalid("message_id", "not an assistant reply")
	}
	svg, err := runs.CheckLibraryDiagram(req.Body.Svg)
	if err != nil {
		return nil, err
	}
	key := runs.LibraryDiagramKey(ws, u.ID, svg)
	l, err := s.Q.GetLibraryDiagram(ctx, store.GetLibraryDiagramParams{WorkspaceID: ws, UserID: u.ID, BlobKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		// A soft cap, as for uploads.
		stored, err := s.Q.ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			return nil, err
		}
		if stored+int64(len(svg)) > runs.MaxGeneratedImageStorageBytes {
			return nil, apperr.Invalid("svg", "your Library is full (1 GB): remove items you no longer need")
		}
		if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, bytes.NewReader(svg), int64(len(svg)), libraryDiagramType); err != nil {
			return nil, err
		}
		l, err = s.Q.CreateLibraryDiagram(ctx, store.CreateLibraryDiagramParams{WorkspaceID: ws, UserID: u.ID, Title: runs.LibraryDiagramTitle(chat.Title), Filename: "diagram.svg",
			ContentType: libraryDiagramType, SizeBytes: int64(len(svg)), BlobKey: key, ChatID: &chat.ID, MessageID: &m.ID, PersonaID: m.PersonaID})
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	source := api.LibraryItemSource(l.Source)
	return api.SaveLibraryDiagram201JSONResponse{Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType, SizeBytes: l.SizeBytes,
		ChatId: l.ChatID, PersonaId: l.PersonaID, Source: &source, CreatedAt: l.CreatedAt}, nil
}

// SetLibraryReference makes one of the caller's images the reference for a
// role, replacing the role's previous image and whatever this image stood for
// before. The persona must be the caller's own.
func (s *Server) SetLibraryReference(ctx context.Context, req api.SetLibraryReferenceRequestObject) (api.SetLibraryReferenceResponseObject, error) {
	u, _, ws := me(ctx)
	role := string(req.Body.Role)
	var persona *uuid.UUID
	switch req.Body.Role {
	case api.LibraryReferenceRoleSelf:
		if req.Body.PersonaId != nil {
			return nil, apperr.Invalid("persona_id", "only for role persona")
		}
	case api.LibraryReferenceRolePersona:
		if req.Body.PersonaId == nil {
			return nil, apperr.Invalid("persona_id", "required for role persona")
		}
		if _, err := s.Q.GetPersona(ctx, store.GetPersonaParams{ID: *req.Body.PersonaId, WorkspaceID: ws, UserID: u.ID}); errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Invalid("persona_id", "not one of your personas")
		} else if err != nil {
			return nil, err
		}
		persona = req.Body.PersonaId
	default:
		return nil, apperr.Invalid("role", "persona or self")
	}
	if err := s.setLibraryReference(ctx, ws, u.ID, req.ItemId, &role, persona); err != nil {
		return nil, err
	}
	return api.SetLibraryReference204Response{}, nil
}

func (s *Server) ClearLibraryReference(ctx context.Context, req api.ClearLibraryReferenceRequestObject) (api.ClearLibraryReferenceResponseObject, error) {
	u, _, ws := me(ctx)
	if err := s.setLibraryReference(ctx, ws, u.ID, req.ItemId, nil, nil); err != nil {
		return nil, err
	}
	return api.ClearLibraryReference204Response{}, nil
}

// setLibraryReference marks item for role (nil clears it) under the person's
// reference lock, unmarking the role's current image first.
func (s *Server) setLibraryReference(ctx context.Context, ws, user, item uuid.UUID, role *string, persona *uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		if err := q.LockLibraryReferences(ctx, store.LockLibraryReferencesParams{WorkspaceID: ws, UserID: user}); err != nil {
			return err
		}
		l, err := q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: item, WorkspaceID: ws, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if role != nil {
			if l.Kind != "image" {
				return apperr.Invalid("role", "only an image can be a reference")
			}
			if err := q.ClearLibraryReferenceRole(ctx, store.ClearLibraryReferenceRoleParams{WorkspaceID: ws, UserID: user, Role: *role, PersonaID: persona}); err != nil {
				return err
			}
		}
		_, err = q.SetLibraryReference(ctx, store.SetLibraryReferenceParams{ID: item, WorkspaceID: ws, UserID: user, Role: role, PersonaID: persona})
		return err
	})
}
