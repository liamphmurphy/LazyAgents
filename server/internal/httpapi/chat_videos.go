package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func toChatVideo(j store.ChatVideoJob, frames []store.ChatVideoFrame) api.ChatVideo {
	out := api.ChatVideo{
		Id: j.ID, Title: j.Title, Width: int(j.Width), Height: int(j.Height), Fps: int(j.Fps), DurationMs: int(j.DurationMs),
		Stage: api.ChatVideoStage(j.Stage), State: api.ChatVideoState(j.State), Error: j.Error,
		ScriptPath: j.ScriptPath, ScriptLines: int(j.ScriptLines), ScriptTail: j.ScriptTail, Checks: []api.ChatVideoCheck{},
		FramesDone: int(j.FramesDone), FramesTotal: int(j.FramesTotal), StageNote: j.StageNote, RenderStartedAt: j.RenderStartedAt,
		RunId: j.RunID, MessageId: j.MessageID, LibraryItemId: j.LibraryItemID, Frames: make([]api.ChatVideoFrame, 0, len(frames)),
		CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, FinishedAt: j.FinishedAt,
	}
	if j.FailureKind != nil {
		k := api.ChatVideoFailureKind(*j.FailureKind)
		out.FailureKind = &k
	}
	if j.ErrorLog != "" {
		out.ErrorLog = &j.ErrorLog
	}
	var checks []runs.VideoCheck
	if json.Unmarshal(j.Checks, &checks) == nil {
		for _, c := range checks {
			out.Checks = append(out.Checks, api.ChatVideoCheck{Label: c.Label, Passed: c.Passed})
		}
	}
	if j.State == "done" && j.OutputBlobKey != nil {
		o := api.ChatVideoOutput{Filename: deref(j.OutputFilename), ContentType: deref(j.OutputContentType), Codec: j.OutputCodec}
		if j.OutputSizeBytes != nil {
			o.SizeBytes = *j.OutputSizeBytes
		}
		if j.OutputDurationMs != nil {
			d := int(*j.OutputDurationMs)
			o.DurationMs = &d
		}
		out.Output = &o
	}
	for _, f := range frames {
		out.Frames = append(out.Frames, api.ChatVideoFrame{Kind: api.ChatVideoFrameKind(f.Kind), Idx: int(f.Idx), Version: f.UpdatedAt.UnixMilli()})
	}
	return out
}

func (s *Server) ListChatVideos(ctx context.Context, req api.ListChatVideosRequestObject) (api.ListChatVideosResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	jobs, err := s.Q.ListChatVideoJobs(ctx, &c.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	frames, err := s.Q.ChatVideoFrames(ctx, ids)
	if err != nil {
		return nil, err
	}
	byJob := map[uuid.UUID][]store.ChatVideoFrame{}
	for _, f := range frames {
		byJob[f.JobID] = append(byJob[f.JobID], f)
	}
	out := api.ChatVideos{Videos: make([]api.ChatVideo, 0, len(jobs))}
	for _, j := range jobs {
		out.Videos = append(out.Videos, toChatVideo(j, byJob[j.ID]))
	}
	return api.ListChatVideos200JSONResponse(out), nil
}

// ownChatVideo is one of the caller's chat's videos, else 404.
func (s *Server) ownChatVideo(ctx context.Context, chatID, id uuid.UUID) (store.ChatVideoJob, error) {
	c, err := s.ownChat(ctx, chatID)
	if err != nil {
		return store.ChatVideoJob{}, err
	}
	j, err := s.Q.GetChatVideoJob(ctx, store.GetChatVideoJobParams{ID: id, WorkspaceID: c.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (j.ChatID == nil || *j.ChatID != c.ID)) {
		return j, apperr.ErrNotFound
	}
	return j, err
}

func (s *Server) GetChatVideoFrame(ctx context.Context, req api.GetChatVideoFrameRequestObject) (api.GetChatVideoFrameResponseObject, error) {
	j, err := s.ownChatVideo(ctx, req.Id, req.VideoId)
	if err != nil {
		return nil, err
	}
	f, err := s.Q.GetChatVideoFrame(ctx, store.GetChatVideoFrameParams{JobID: j.ID, Kind: string(req.Kind), Idx: int32(req.Idx), WorkspaceID: j.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, f.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	// The URL names the version, so a matching one never changes.
	cache := "private, no-cache"
	if req.Params.V != nil && *req.Params.V == f.UpdatedAt.UnixMilli() {
		cache = "private, max-age=31536000, immutable"
	}
	name := fmt.Sprintf("%s-%d", f.Kind, f.Idx)
	return contentResponse{body: o.Body, closer: o.Body, contentType: f.ContentType, length: f.SizeBytes, filename: name, inline: true, cache: cache}, nil
}

func (s *Server) GetChatVideoContent(ctx context.Context, req api.GetChatVideoContentRequestObject) (api.GetChatVideoContentResponseObject, error) {
	j, err := s.ownChatVideo(ctx, req.Id, req.VideoId)
	if err != nil {
		return nil, err
	}
	if j.State != "done" || j.OutputBlobKey == nil || j.OutputSizeBytes == nil {
		return nil, apperr.ErrNotFound
	}
	download := req.Params.Download != nil && *req.Params.Download
	resp, err := s.serveRanged(ctx, *j.OutputBlobKey, deref(j.OutputContentType), *j.OutputSizeBytes, deref(j.OutputFilename), !download, req.Params.Range)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// serveRanged serves a stored file whole, or the one byte range asked for.
func (s *Server) serveRanged(ctx context.Context, key, contentType string, size int64, filename string, inline bool, rangeHeader *string) (rangedResponse, error) {
	resp := contentResponse{contentType: contentType, filename: filename, inline: inline, ranged: true, total: size}
	start, end, partial, ok := parseByteRange(deref(rangeHeader), size)
	if !ok {
		return rangedResponse{unsatisfiable: true, total: size}, nil
	}
	var o blob.Object
	var err error
	if partial {
		o, err = s.Blobs.GetRange(ctx, s.ArtifactBucket, key, start, end)
		resp.partial, resp.start, resp.length = true, start, end-start+1
	} else {
		o, err = s.Blobs.Get(ctx, s.ArtifactBucket, key)
		resp.length = size
	}
	if errors.Is(err, blob.ErrNotFound) {
		return rangedResponse{}, apperr.ErrNotFound
	} else if err != nil {
		return rangedResponse{}, err
	}
	resp.body, resp.closer = o.Body, o.Body
	return rangedResponse{contentResponse: resp}, nil
}

// parseByteRange reads a Range header of one byte range against a file of
// size bytes. Without a usable single range (none, several, another unit)
// the whole file is served (partial false); ok false means the range can't
// be satisfied (416).
func parseByteRange(h string, size int64) (start, end int64, partial, ok bool) {
	spec, found := strings.CutPrefix(strings.TrimSpace(h), "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, size - 1, false, true
	}
	first, last, found := strings.Cut(strings.TrimSpace(spec), "-")
	if !found {
		return 0, size - 1, false, true
	}
	if first == "" { // the last n bytes
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, false
		}
		n = min(n, size)
		return size - n, size - 1, true, size > 0
	}
	a, err := strconv.ParseInt(first, 10, 64)
	if err != nil || a < 0 || a >= size {
		return 0, 0, false, false
	}
	b := size - 1
	if last != "" {
		if b, err = strconv.ParseInt(last, 10, 64); err != nil || b < a {
			return 0, 0, false, false
		}
		b = min(b, size-1)
	}
	return a, b, true, true
}

// rangedResponse is a file (whole or one range), or a 416.
type rangedResponse struct {
	contentResponse
	unsatisfiable bool
	total         int64
}

func (r rangedResponse) write(w http.ResponseWriter) error {
	if r.unsatisfiable {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", r.total))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		_, err := w.Write([]byte(`{"error":"range_not_satisfiable","message":"The requested range is outside the file."}`))
		return err
	}
	return r.contentResponse.write(w)
}

func (r rangedResponse) VisitGetChatVideoContentResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (r rangedResponse) VisitGetLibraryItemContentResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (r contentResponse) VisitGetChatVideoFrameResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (r contentResponse) VisitGetLibraryItemPosterResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (s *Server) GetLibraryItemPoster(ctx context.Context, req api.GetLibraryItemPosterRequestObject) (api.GetLibraryItemPosterResponseObject, error) {
	u, _, ws := me(ctx)
	l, err := s.Q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: req.ItemId, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && l.PosterKey == nil) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	o, err := s.Blobs.Get(ctx, s.ArtifactBucket, *l.PosterKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	ct := o.ContentType
	if !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg" // posters are copied from frames, which are checked images
	}
	return contentResponse{body: o.Body, closer: o.Body, contentType: ct, length: o.ContentLength, filename: "poster", inline: true, cache: "private, max-age=86400"}, nil
}
