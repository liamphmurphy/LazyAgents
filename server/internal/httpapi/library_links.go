package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/linkpreview"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A saved link's fallbacks when its page couldn't be read.
const (
	libraryLinkType     = "text/uri-list"
	libraryLinkMaxTitle = 300
)

func libraryLinkKey(ws, user uuid.UUID, link string) string {
	return fmt.Sprintf("library-links/%s/%s/%x", ws, user, sha256.Sum256([]byte(link)))
}

// libraryLinkTitle names a link with no readable title after its address.
func libraryLinkTitle(title, link string) string {
	if title != "" {
		return title
	}
	t := link
	if u, err := url.Parse(link); err == nil {
		t = strings.TrimPrefix(u.Hostname(), "www.") + strings.TrimRight(u.Path, "/")
	}
	if r := []rune(t); len(r) > libraryLinkMaxTitle {
		t = string(r[:libraryLinkMaxTitle])
	}
	return t
}

// SaveLibraryLink keeps a link recommended in one of the caller's replies. The
// image gets its own copy, because removing an item deletes the object it holds.
func (s *Server) SaveLibraryLink(ctx context.Context, req api.SaveLibraryLinkRequestObject) (api.SaveLibraryLinkResponseObject, error) {
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
	var rec *linkpreview.Recommendation
	for _, r := range linkpreview.Recommended(m.BodyMd) {
		if r.URL == req.Body.Url {
			rec = &r
			break
		}
	}
	if rec == nil {
		return nil, apperr.Invalid("url", "that reply doesn't recommend this link")
	}
	l, err := s.Q.GetLibraryLink(ctx, store.GetLibraryLinkParams{WorkspaceID: ws, UserID: u.ID, Url: rec.URL})
	if errors.Is(err, pgx.ErrNoRows) {
		l, err = s.createLibraryLink(ctx, ws, u.ID, chat.ID, m, *rec)
	}
	if err != nil {
		return nil, err
	}
	source := api.LibraryItemSource(l.Source)
	return api.SaveLibraryLink201JSONResponse{Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType, SizeBytes: l.SizeBytes,
		ChatId: l.ChatID, PersonaId: l.PersonaID, Source: &source, Url: &l.Url, Note: &l.Note, Site: &l.Site, VideoId: &l.VideoID, CreatedAt: l.CreatedAt}, nil
}

func (s *Server) createLibraryLink(ctx context.Context, ws, user, chat uuid.UUID, m store.ChatMessage, rec linkpreview.Recommendation) (store.LibraryItem, error) {
	var none store.LibraryItem
	p, err := s.linkPreview(ctx, ws, rec.URL)
	if err != nil {
		return none, err
	}
	key, contentType, size := "", libraryLinkType, int64(0)
	if p.BlobKey != "" {
		o, err := s.Blobs.Get(ctx, s.ArtifactBucket, p.BlobKey)
		if err != nil {
			return none, err
		}
		defer o.Body.Close()
		img, err := io.ReadAll(o.Body)
		if err != nil {
			return none, err
		}
		// A soft cap, as for uploads.
		stored, err := s.Q.ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: ws, UserID: user})
		if err != nil {
			return none, err
		}
		if stored+int64(len(img)) > runs.MaxGeneratedImageStorageBytes {
			return none, apperr.Invalid("url", "your Library is full (1 GB): remove items you no longer need")
		}
		key, contentType, size = libraryLinkKey(ws, user, rec.URL), p.ImageType, int64(len(img))
		if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, bytes.NewReader(img), size, contentType); err != nil {
			return none, err
		}
	}
	return s.Q.CreateLibraryLink(ctx, store.CreateLibraryLinkParams{WorkspaceID: ws, UserID: user, Title: libraryLinkTitle(p.Title, rec.URL), ContentType: contentType, SizeBytes: size, BlobKey: key,
		ChatID: &chat, MessageID: &m.ID, PersonaID: m.PersonaID, Url: rec.URL, Note: rec.Note, Site: p.Site, VideoID: p.VideoID})
}
