package designs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/design"
)

// DefaultWorkerURL is where the design worker lives in the compose stack.
const DefaultWorkerURL = "http://web:3000/internal/designs"

// WorkerTimeout bounds one call to the worker.
const WorkerTimeout = 15 * time.Second

// ErrUnavailable means the worker can't be reached or isn't configured. Callers
// degrade ("render unavailable") rather than fail the request.
var ErrUnavailable = errors.New("the design renderer is unavailable")

// Worker is the client of the Node design worker (web/lib/designs/worker.ts):
// it renders a design body to a PNG and computes settled node boxes.
type Worker struct {
	URL    string // DESIGN_WORKER_URL
	Token  string // DESIGN_WORKER_TOKEN; unset means unavailable
	Client *http.Client
	// Backoff is the pause before the one retry of a busy (503) worker.
	Backoff time.Duration
}

// Box is a node's box in page coordinates.
type Box = design.Box

// LaidPage is one page's node boxes.
type LaidPage struct {
	ID    string         `json:"id"`
	Boxes map[string]Box `json:"boxes"`
}

// RenderRequest is what to draw: the whole page or one node, at a scale, with
// the design's images as data URIs keyed by asset id.
type RenderRequest struct {
	Body   string            `json:"body"`
	Node   string            `json:"node,omitempty"`
	Page   string            `json:"page,omitempty"`
	Scale  float64           `json:"scale,omitempty"`
	Assets map[string]string `json:"assets,omitempty"`
}

// Rendered is a PNG and its size in design pixels.
type Rendered struct {
	PNG  []byte
	W, H int
}

// RejectedError is the worker refusing a request (400) or timing out (504):
// a message worth showing, unlike an outage.
type RejectedError struct {
	Status  int
	Message string
}

func (e *RejectedError) Error() string { return e.Message }

// Available reports whether calls can be attempted at all.
func (w *Worker) Available() bool { return w != nil && w.Token != "" }

func (w *Worker) base() string {
	if w.URL == "" {
		return DefaultWorkerURL
	}
	return strings.TrimRight(w.URL, "/")
}

func (w *Worker) post(ctx context.Context, path string, in any) (*http.Response, []byte, error) {
	if !w.Available() {
		return nil, nil, ErrUnavailable
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, nil, err
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: WorkerTimeout}
	}
	backoff := w.Backoff
	if backoff == 0 {
		backoff = 400 * time.Millisecond
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.base()+path, bytes.NewReader(raw))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+w.Token)
		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			return nil, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		res.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if res.StatusCode == http.StatusServiceUnavailable && attempt == 0 {
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
		return res, body, nil
	}
}

// failure turns a non-200 reply into an error.
func failure(res *http.Response, body []byte) error {
	var m struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &m)
	switch res.StatusCode {
	case http.StatusBadRequest, http.StatusGatewayTimeout:
		if m.Message == "" {
			m.Message = "the renderer couldn't draw this design"
		}
		return &RejectedError{Status: res.StatusCode, Message: m.Message}
	}
	// 401 (wrong token), 503 (still busy after the retry), 5xx: not the design's fault.
	return fmt.Errorf("%w: worker answered %d %s", ErrUnavailable, res.StatusCode, m.Message)
}

// Render draws a design, a page or one node as a PNG.
func (w *Worker) Render(ctx context.Context, in RenderRequest) (Rendered, error) {
	res, body, err := w.post(ctx, "/render", in)
	if err != nil {
		return Rendered{}, err
	}
	if res.StatusCode != http.StatusOK {
		return Rendered{}, failure(res, body)
	}
	out := Rendered{PNG: body}
	out.W, _ = strconv.Atoi(res.Header.Get("X-Design-Width"))
	out.H, _ = strconv.Atoi(res.Header.Get("X-Design-Height"))
	return out, nil
}

// Layout computes settled node boxes in page coordinates, for every page or
// for one.
func (w *Worker) Layout(ctx context.Context, body, page string) ([]LaidPage, error) {
	in := map[string]string{"body": body}
	if page != "" {
		in["page"] = page
	}
	res, raw, err := w.post(ctx, "/layout", in)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, failure(res, raw)
	}
	var out struct {
		Pages []LaidPage `json:"pages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: unreadable layout reply", ErrUnavailable)
	}
	return out.Pages, nil
}
