package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/transfer"
)

// noBlobs is an object store with nothing in it.
type noBlobs struct{}

func (noBlobs) ListBuckets(context.Context) ([]string, error)             { return nil, nil }
func (noBlobs) List(context.Context, string, func(blob.Info) error) error { return nil }
func (noBlobs) Get(context.Context, string, string) (blob.Object, error) {
	return blob.Object{}, blob.ErrNotFound
}
func (noBlobs) Put(context.Context, string, string, io.Reader, int64, string) error { return nil }
func (noBlobs) EnsureBucket(context.Context, string) error                          { return nil }

func TestExportInstanceDownloadsAnArchive(t *testing.T) {
	e := newChatEnvServer(t, nil, func(s *Server) {
		s.Exporter = &transfer.Exporter{Pool: s.Pool, Blobs: noBlobs{}, AppVersion: "test"}
	})
	res, err := http.Get(e.srv.URL + "/api/v1/instance/export")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", res.StatusCode, body)
	}
	if got := res.Header.Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", got)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="vesper-export-`) {
		t.Errorf("Content-Disposition = %q, want an attachment named vesper-export-DATE.zip", cd)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q: an archive of everything mustn't be cached", got)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("the download isn't a zip: %v", err)
	}
	have := map[string]bool{}
	for _, f := range zr.File {
		have[f.Name] = true
	}
	for _, want := range []string{"manifest.json", "db/tickets.csv", "db/users.csv"} {
		if !have[want] {
			t.Errorf("archive has no %s", want)
		}
	}
	// The seeded demo has a user and tickets, so the tables aren't empty files.
	for _, f := range zr.File {
		if f.Name == "db/tickets.csv" && f.UncompressedSize64 == 0 {
			t.Error("db/tickets.csv is empty")
		}
	}
}

func TestExportInstanceNeedsSetup(t *testing.T) {
	e := newChatEnvServer(t, nil, nil) // no Exporter
	e.want("GET", "/instance/export", nil, http.StatusConflict)
}

// The archive holds every secret and table, so a signed-out request gets nothing.
func TestExportInstanceRequiresSignIn(t *testing.T) {
	_, h := newSignedInServer(t)
	if rec := getWithCookie(h, "/api/v1/instance/export", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed out: GET /instance/export = %d, want 401", rec.Code)
	}
}
