package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// designExportKinds is how each export format is kept in the Library: the
// sniffed content decides, and the stored name gets the matching extension.
var designExportKinds = []struct {
	kind, contentType, ext string
	is                     func(head, all []byte) bool
}{
	{"image", "image/png", ".png", func(h, _ []byte) bool { return bytes.HasPrefix(h, []byte("\x89PNG\r\n\x1a\n")) }},
	{"document", "application/pdf", ".pdf", func(h, _ []byte) bool { return bytes.HasPrefix(h, []byte("%PDF-")) }},
	{"diagram", libraryDiagramType, ".svg", func(_, all []byte) bool { return runs.IsSVGDocument(all) }},
}

// SaveDesignExportToLibrary keeps a finished PNG, SVG or PDF export of one of
// the caller's designs. The browser builds the whole file before it uploads,
// so a failed export never reaches here, and a refused file stores nothing.
func (s *Server) SaveDesignExportToLibrary(ctx context.Context, req api.SaveDesignExportToLibraryRequestObject) (api.SaveDesignExportToLibraryResponseObject, error) {
	u, _, ws := me(ctx)
	cv, err := s.myDesign(ctx, req.CanvasId)
	if err != nil {
		return nil, err
	}
	f, err := readForm(req.Body)
	if err != nil {
		return nil, err
	}
	defer f.close()
	if f.file == nil {
		return nil, apperr.Invalid("file", "required")
	}
	if f.file.Size == 0 {
		return nil, apperr.Invalid("file", "the file is empty")
	}
	// readForm has capped the file at 25 MB, so the check can read all of it.
	all, err := io.ReadAll(f.tmp)
	if err != nil {
		return nil, err
	}
	head := all[:min(len(all), 512)]
	idx := -1
	for i, k := range designExportKinds {
		if k.is(head, all) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, apperr.Invalid("file", "only a PNG, SVG or PDF export can be saved")
	}
	k := designExportKinds[idx]

	name := runs.ChatAttachmentName(f.file.Name)
	title := strings.TrimSpace(strings.TrimSuffix(name, path.Ext(name)))
	if title == "" {
		title = cv.Title
	}
	if r := []rune(title); len(r) > runs.MaxLibraryTitleRunes {
		title = string(r[:runs.MaxLibraryTitleRunes])
	}
	filename := title + k.ext

	// A soft cap, as for uploads: two concurrent saves can each pass it.
	stored, err := s.Q.ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	size := int64(len(all))
	if stored+size > runs.MaxGeneratedImageStorageBytes {
		return nil, apperr.Invalid("file", "your Library is full (1 GB): remove items you no longer need")
	}
	key := fmt.Sprintf("library-designs/%s/%s/%s", ws, u.ID, uuid.New())
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, bytes.NewReader(all), size, k.contentType); err != nil {
		return nil, err
	}
	l, err := s.Q.CreateLibraryItem(ctx, store.CreateLibraryItemParams{WorkspaceID: ws, UserID: u.ID, Kind: k.kind, Title: title, Filename: filename,
		ContentType: k.contentType, SizeBytes: size, BlobKey: key, ChatID: &cv.ChatID})
	if err != nil {
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, key)
		return nil, err
	}
	source := api.LibraryItemSource(l.Source)
	return api.SaveDesignExportToLibrary201JSONResponse{Id: l.ID, Kind: api.LibraryItemKind(l.Kind), Title: l.Title, Filename: l.Filename, ContentType: l.ContentType,
		SizeBytes: l.SizeBytes, ChatId: l.ChatID, PersonaId: l.PersonaID, Source: &source, CreatedAt: l.CreatedAt}, nil
}
