package httpapi

import (
	"bytes"
	"context"
	"io"
	"path"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
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
	row, err := s.keepExport(ctx, u.ID, ws, cv, exportFile{data: all, contentType: k.contentType, filename: title + k.ext, title: title})
	if err != nil {
		return nil, err
	}
	return api.SaveDesignExportToLibrary201JSONResponse(toLibraryItem(row)), nil
}
