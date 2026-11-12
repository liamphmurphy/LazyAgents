package designs

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// maxRenderAssetBytes caps the images posted with one render (the worker
// accepts 48 MB of request; base64 adds a third).
const maxRenderAssetBytes = 30 << 20

// assetURIs loads a design's images from the blob store as data URIs keyed by
// asset id. An image whose bytes are gone is left out; the worker draws a
// placeholder for it.
func (s *Service) assetURIs(ctx context.Context, canvasID uuid.UUID, body string) (map[string]string, error) {
	d, err := design.Parse(body)
	if err != nil {
		return nil, err
	}
	used, _ := d["assets"].(map[string]any)
	if len(used) == 0 || s.Blobs == nil {
		return nil, nil
	}
	q := store.New(s.Pool)
	out := map[string]string{}
	total := 0
	// Sorted, so the images kept under the size cap don't vary between renders.
	for _, id := range slices.Sorted(maps.Keys(used)) {
		a, err := q.GetDesignAsset(ctx, store.GetDesignAssetParams{CanvasID: canvasID, ID: id})
		if err != nil {
			continue
		}
		obj, err := s.Blobs.Get(ctx, s.Bucket, a.BlobKey)
		if errors.Is(err, blob.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(obj.Body, maxRenderAssetBytes+1))
		obj.Body.Close()
		if err != nil {
			return nil, err
		}
		if total += len(raw); total > maxRenderAssetBytes {
			break
		}
		out[id] = "data:" + a.Mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
	}
	return out, nil
}

// Render draws a design body (a page, or a node) through the worker, posting
// the design's images with it. ErrUnavailable means no renderer.
func (s *Service) Render(ctx context.Context, canvasID uuid.UUID, body, node, page string, scale float64) (Rendered, error) {
	if !s.Worker.Available() {
		return Rendered{}, ErrUnavailable
	}
	assets, err := s.assetURIs(ctx, canvasID, body)
	if err != nil {
		return Rendered{}, err
	}
	return s.Worker.Render(ctx, RenderRequest{Body: body, Node: node, Page: page, Scale: scale, Assets: assets})
}

// Boxes asks the worker for settled node boxes (page coordinates), by node id.
func (s *Service) Boxes(ctx context.Context, body, page string) (map[string]design.Box, error) {
	if !s.Worker.Available() {
		return nil, ErrUnavailable
	}
	pages, err := s.Worker.Layout(ctx, body, page)
	if err != nil {
		return nil, err
	}
	out := map[string]design.Box{}
	for _, p := range pages {
		for id, b := range p.Boxes {
			out[id] = b
		}
	}
	return out, nil
}

// Import asks the worker to turn Mermaid or SVG text into a node for a page.
func (s *Service) Import(ctx context.Context, in ImportRequest) (Imported, error) {
	if !s.Worker.Available() {
		return Imported{}, ErrUnavailable
	}
	return s.Worker.Import(ctx, in)
}
