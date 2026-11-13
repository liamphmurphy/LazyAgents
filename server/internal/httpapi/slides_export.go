package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"path"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// SaveDeckExportToLibrary keeps a finished PDF or PowerPoint export of one of
// the caller's decks. The browser builds the file before it uploads, so a
// failed export never reaches here, and a refused file stores nothing.
func (s *Server) SaveDeckExportToLibrary(ctx context.Context, req api.SaveDeckExportToLibraryRequestObject) (api.SaveDeckExportToLibraryResponseObject, error) {
	u, _, ws := me(ctx)
	cv, err := s.myDeck(ctx, req.CanvasId)
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
	var contentType, ext string
	switch {
	case bytes.HasPrefix(all, []byte("%PDF-")):
		contentType, ext = "application/pdf", ".pdf"
	case isPPTX(all):
		contentType, ext = "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"
	default:
		return nil, apperr.Invalid("file", "only a PDF or PowerPoint export can be saved")
	}
	name := runs.ChatAttachmentName(f.file.Name)
	title := strings.TrimSpace(strings.TrimSuffix(name, path.Ext(name)))
	if title == "" {
		title = cv.Title
	}
	detail := ""
	if cur, err := s.Q.GetUserCanvas(ctx, store.GetUserCanvasParams{ID: cv.ID, WorkspaceID: ws, UserID: u.ID}); err == nil {
		if d, err := deck.Parse(cur.Body); err == nil {
			detail = runs.CountOf(len(d.Slides), "slide")
		}
	}
	row, err := s.keepExport(ctx, u.ID, ws, cv, exportFile{data: all, contentType: contentType, filename: title + ext, title: title, detail: detail})
	if err != nil {
		return nil, err
	}
	return api.SaveDeckExportToLibrary201JSONResponse(toLibraryItem(row)), nil
}

// isPPTX reports whether data is a zip holding a PowerPoint presentation.
func isPPTX(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == "ppt/presentation.xml" {
			return true
		}
	}
	return false
}
