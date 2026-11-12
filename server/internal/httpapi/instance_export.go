package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// ExportInstance streams the whole-instance archive (see internal/transfer).
// The archive is built as it is sent: tables are copied straight out of
// Postgres and objects straight out of S3, so memory stays flat however much
// is stored. Once the response has started, a failure can only end it early;
// the download then fails in the browser, and the cause is in the server log.
func (s *Server) ExportInstance(ctx context.Context, _ api.ExportInstanceRequestObject) (api.ExportInstanceResponseObject, error) {
	if s.Exporter == nil {
		return nil, apperr.Conflict("this server isn't set up to export")
	}
	pr, pw := io.Pipe()
	go func() {
		// ctx is the request's: a closed tab cancels the export.
		_, err := s.Exporter.Export(ctx, pw)
		if err != nil {
			s.Log.Error("instance export", "err", err)
		}
		pw.CloseWithError(err)
	}()
	return instanceArchive{api.ExportInstance200ApplicationzipResponse{Body: pr}}, nil
}

// instanceArchive adds a download filename to the generated response.
type instanceArchive struct {
	api.ExportInstance200ApplicationzipResponse
}

func (e instanceArchive) VisitExportInstanceResponse(w http.ResponseWriter) error {
	name := fmt.Sprintf("vesper-export-%s.zip", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	// An archive of everything must never be cached by a proxy or the browser.
	w.Header().Set("Cache-Control", "no-store")
	return e.ExportInstance200ApplicationzipResponse.VisitExportInstanceResponse(w)
}
