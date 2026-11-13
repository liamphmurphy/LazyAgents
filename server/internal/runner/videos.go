package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// videoSyncInterval is how often the runner copies a chat video's files while
// the turn runs.
const videoSyncInterval = 2 * time.Second

// videoScriptTailLines is how much of the renderer the card shows while it's written.
const videoScriptTailLines = 6

var (
	stillName = regexp.MustCompile(`^stills/0*(\d{1,7})\.(png|jpe?g|webp)$`)
	stripName = regexp.MustCompile(`^strip/0*(\d{1,2})\.(png|jpe?g|webp)$`)
)

// videoProgress is what a renderer writes to progress.json.
type videoProgress struct {
	FramesDone  int    `json:"frames_done"`
	FramesTotal int    `json:"frames_total"`
	Stage       string `json:"stage"`
	Note        string `json:"note"`
}

// videoSync copies the files of a chat run's video jobs. One lives for one
// turn; stamps remembers what it already copied so a file is read once per
// change.
type videoSync struct {
	r      *Runner
	c      runCtx
	secret map[string]string // values to redact from the renderer's lines
	mu     sync.Mutex
	stamps map[uuid.UUID]map[string]string
	// progress is the last progress read per job; a renderer rewrites it
	// often, and the job is only updated when it changes.
	progress map[uuid.UUID]videoProgress
	script   map[uuid.UUID][2]string // lines, tail
}

// startVideoSync copies the run's video files every videoSyncInterval until
// the returned stop is called. stop waits for a copy in flight.
func (r *Runner) startVideoSync(ctx context.Context, c runCtx, secret map[string]string) (*videoSync, func()) {
	s := &videoSync{r: r, c: c, secret: secret, stamps: map[uuid.UUID]map[string]string{}, progress: map[uuid.UUID]videoProgress{}, script: map[uuid.UUID][2]string{}}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(videoSyncInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx, false)
			}
		}
	}()
	var once sync.Once
	return s, func() { once.Do(func() { close(done); wg.Wait() }) }
}

// tick copies what changed for each running job and, for one the agent
// finished, stores its video. final is the pass after the turn ended: a job
// still running then can't be finished any more.
func (s *videoSync) tick(ctx context.Context, final bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs, err := s.r.q().RunningChatVideoJobs(ctx, &s.c.run.ID)
	if err != nil {
		s.r.Log.Warn("chat videos", "run", s.c.run.ID, "err", err)
		return
	}
	for _, job := range jobs {
		s.syncJob(ctx, job)
		if job.Stage == runs.VideoStageSave {
			s.r.saveChatVideo(ctx, s.c, job, final)
		}
	}
	s.tickAudio(ctx, final)
}

// known is what this sync already copied for the job. A runner that took
// over mid-turn copies each file once more, replacing the same slots.
func (s *videoSync) known(job store.ChatVideoJob) map[string]string {
	k, ok := s.stamps[job.ID]
	if !ok {
		k = map[string]string{}
		s.stamps[job.ID] = k
	}
	return k
}

func (s *videoSync) syncJob(ctx context.Context, job store.ChatVideoJob) {
	if s.r.Blobs == nil || s.r.Cfg.AttachmentBucket == "" {
		return
	}
	dir := runs.VideoDir(job.ID)
	known := s.known(job)
	files, err := s.r.Backend.ReadVideoFiles(ctx, wsID(s.c.run), VideoFilesRequest{Dir: dir, Script: job.ScriptPath, Known: known})
	if err != nil {
		s.r.Log.Warn("read chat video files", "run", s.c.run.ID, "video", job.ID, "err", err)
		return
	}
	prog, havePrev := s.progress[job.ID]
	script := s.script[job.ID]
	for _, f := range files {
		rel := strings.TrimPrefix(f.Path, dir+"/")
		switch {
		case job.ScriptPath != "" && f.Path == job.ScriptPath:
			lines, tail := scriptTail(redact(string(f.Data), s.secret))
			script = [2]string{strconv.Itoa(lines), tail}
		case rel == runs.VideoProgressFile:
			var p videoProgress
			if json.Unmarshal(f.Data, &p) != nil {
				break // half-written: the next tick reads it again
			}
			prog = cleanProgress(p)
			havePrev = true
		default:
			kind, idx, ok := frameSlot(rel)
			if !ok || !s.storeFrame(ctx, job, kind, idx, f) {
				continue
			}
		}
		known[f.Path] = f.Stamp
	}
	s.progress[job.ID], s.script[job.ID] = prog, script
	lines, _ := strconv.Atoi(script[0])
	if !havePrev && script == ([2]string{}) {
		return
	}
	if !havePrev {
		prog = videoProgress{FramesDone: int(job.FramesDone), FramesTotal: int(job.FramesTotal), Note: job.StageNote}
	}
	if script == ([2]string{}) {
		lines, script[1] = int(job.ScriptLines), job.ScriptTail
	}
	encoding := prog.Stage == runs.VideoStageEncode
	if int32(prog.FramesDone) == job.FramesDone && int32(prog.FramesTotal) == job.FramesTotal && prog.Note == job.StageNote &&
		int32(lines) == job.ScriptLines && script[1] == job.ScriptTail && !(encoding && job.Stage == runs.VideoStageRender) {
		return
	}
	if err := s.r.q().SetChatVideoProgress(ctx, store.SetChatVideoProgressParams{ID: job.ID, FramesDone: int32(prog.FramesDone), FramesTotal: int32(prog.FramesTotal),
		StageNote: prog.Note, ScriptLines: int32(lines), ScriptTail: script[1], Encoding: encoding}); err != nil {
		s.r.Log.Warn("chat video progress", "video", job.ID, "err", err)
	}
}

// frameSlot names the frame a file in the job's directory fills.
func frameSlot(rel string) (kind string, idx int, ok bool) {
	switch rel {
	case runs.VideoPreviewFile:
		return "preview", 0, true
	case runs.VideoPosterFile:
		return "poster", 0, true
	}
	if m := stillName.FindStringSubmatch(rel); m != nil {
		n, _ := strconv.Atoi(m[1])
		return "still", n, true
	}
	if m := stripName.FindStringSubmatch(rel); m != nil {
		n, _ := strconv.Atoi(m[1])
		return "strip", n, n < runs.VideoStripCells
	}
	return "", 0, false
}

// storeFrame copies one picture, replacing the slot's earlier one. It
// reports whether the file was handled (stored, or refused for good).
func (s *videoSync) storeFrame(ctx context.Context, job store.ChatVideoJob, kind string, idx int, f VideoFile) bool {
	ct, ok := runs.GeneratedImageType(f.Data)
	if !ok {
		return true // not a picture; skipped until it changes
	}
	if kind == "still" {
		n := 0
		for p := range s.stamps[job.ID] {
			if strings.Contains(p, "/stills/") && p != f.Path {
				n++
			}
		}
		if n >= runs.VideoMaxStills {
			return true
		}
	}
	key := fmt.Sprintf("chat-videos/%s/%s/%s-%d-%s", job.WorkspaceID, job.ID, kind, idx, uuid.NewString()[:8])
	if err := s.r.Blobs.Put(ctx, s.r.Cfg.AttachmentBucket, key, bytes.NewReader(f.Data), int64(len(f.Data)), ct); err != nil {
		s.r.Log.Warn("store chat video frame", "video", job.ID, "err", err)
		return false
	}
	replaced, err := s.r.q().UpsertChatVideoFrame(ctx, store.UpsertChatVideoFrameParams{JobID: job.ID, Kind: kind, Idx: int32(idx), BlobKey: key, ContentType: ct, SizeBytes: int64(len(f.Data)), SourceStamp: f.Stamp})
	if err != nil {
		s.r.Log.Warn("record chat video frame", "video", job.ID, "err", err)
		_ = s.r.Blobs.Delete(context.WithoutCancel(ctx), s.r.Cfg.AttachmentBucket, key)
		return false
	}
	if replaced != "" {
		_ = s.r.Blobs.Delete(context.WithoutCancel(ctx), s.r.Cfg.AttachmentBucket, replaced)
	}
	return true
}

func cleanProgress(p videoProgress) videoProgress {
	p.FramesTotal = min(max(p.FramesTotal, 0), 10_000_000)
	p.FramesDone = min(max(p.FramesDone, 0), max(p.FramesTotal, 0))
	if p.FramesTotal == 0 {
		p.FramesDone = 0
	}
	p.Note = strings.TrimSpace(strings.ReplaceAll(p.Note, "\n", " "))
	if r := []rune(p.Note); len(r) > 120 {
		p.Note = string(r[:120])
	}
	if !utf8.ValidString(p.Note) {
		p.Note = ""
	}
	return p
}

// scriptTail counts the renderer's lines and keeps its last few non-empty ones.
func scriptTail(src string) (int, string) {
	if !utf8.ValidString(src) {
		return 0, ""
	}
	src = strings.TrimRight(src, "\n")
	if src == "" {
		return 0, ""
	}
	all := strings.Split(src, "\n")
	var tail []string
	for i := len(all) - 1; i >= 0 && len(tail) < videoScriptTailLines; i-- {
		l := strings.TrimRight(all[i], " \t\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		if r := []rune(l); len(r) > 160 {
			l = string(r[:160])
		}
		tail = append([]string{l}, tail...)
	}
	return len(all), strings.Join(tail, "\n")
}

// finishVideoSync is the pass after a chat turn ends: it copies the last
// files, stores a video the agent finished, and fails any job left running
// (stopped, if the person stopped the reply).
func (r *Runner) finishVideoSync(ctx context.Context, s *videoSync, stop func()) {
	stop()
	s.tick(ctx, true)
	r.failLeftoverAudio(ctx, s.c)
	jobs, err := r.q().RunningChatVideoJobs(ctx, &s.c.run.ID)
	if err != nil {
		r.Log.Warn("chat videos", "run", s.c.run.ID, "err", err)
		return
	}
	if len(jobs) == 0 {
		return
	}
	kind, reason := runs.VideoFailureError, "The reply ended before the video was finished."
	if run, err := r.q().GetRun(ctx, store.GetRunParams{ID: s.c.run.ID, WorkspaceID: s.c.run.WorkspaceID}); err == nil && run.State != "running" {
		kind, reason = runs.VideoFailureStopped, "Stopped before the video was finished."
	}
	for _, j := range jobs {
		r.failChatVideo(ctx, j, kind, reason)
	}
}

func (r *Runner) failChatVideo(ctx context.Context, job store.ChatVideoJob, kind, reason string) {
	if _, err := r.q().FailChatVideoJob(ctx, store.FailChatVideoJobParams{ID: job.ID, Error: &reason, FailureKind: &kind}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		r.Log.Warn("fail chat video", "video", job.ID, "err", err)
	}
}

// saveChatVideo stores a finished job's video and keeps it in the Library. A
// file that isn't there yet is waited for until the turn ends (final).
func (r *Runner) saveChatVideo(ctx context.Context, c runCtx, job store.ChatVideoJob, final bool) {
	if r.Blobs == nil || r.Cfg.AttachmentBucket == "" {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "Vesper has no storage configured for videos, so it couldn't be saved.")
		return
	}
	out := path.Join(runs.VideoDir(job.ID), runs.VideoOutputFile)
	f, size, err := r.Backend.OpenWorkspaceFile(ctx, wsID(c.run), out, VideoMaxOutputBytes)
	if errors.Is(err, fs.ErrNotExist) {
		if final {
			r.failChatVideo(ctx, job, runs.VideoFailureError, "There was no video file at "+out+".")
		}
		return
	}
	if err != nil {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The video couldn't be read: "+err.Error())
		return
	}
	defer f.Close()
	if size == 0 {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The video file is empty.")
		return
	}
	stored, err := r.q().ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: job.WorkspaceID, UserID: job.UserID})
	if err != nil {
		r.Log.Warn("library usage", "video", job.ID, "err", err)
		return
	}
	if stored+size > runs.MaxGeneratedImageStorageBytes {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The person's Library is full (1 GB), so the video couldn't be saved.")
		return
	}
	// Copied to a local file first: the boxes that describe it may be at the end.
	tmp, err := os.CreateTemp("", "vesper-video-*.mp4")
	if err != nil {
		r.Log.Warn("chat video temp file", "video", job.ID, "err", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if n, err := io.Copy(tmp, io.LimitReader(f, size)); err != nil || n != size {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The video couldn't be copied out of the workspace.")
		return
	}
	info, err := runs.ReadVideoInfo(tmp, size)
	if errors.Is(err, runs.ErrNotMP4) {
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The file at "+out+" isn't an MP4 video.")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return
	}
	key := fmt.Sprintf("chat-videos/%s/%s/video-%s.mp4", job.WorkspaceID, job.ID, uuid.NewString()[:8])
	if err := r.Blobs.Put(ctx, r.Cfg.AttachmentBucket, key, tmp, size, "video/mp4"); err != nil {
		r.Log.Warn("store chat video", "video", job.ID, "err", err)
		r.failChatVideo(ctx, job, runs.VideoFailureError, "The video couldn't be stored.")
		return
	}
	poster := r.copyVideoPoster(ctx, job)
	width, height := info.Width, info.Height
	if width == 0 || height == 0 {
		width, height = int(job.Width), int(job.Height)
	}
	duration := info.DurationMS
	if duration == 0 {
		duration = int(job.DurationMs)
	}
	filename := videoFilename(job.Title)
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var lib *uuid.UUID
		// A chat deleted meanwhile keeps nothing in the Library; the sweep
		// removes the job and its video.
		if job.ChatID != nil {
			item, err := q.CreateLibraryVideo(ctx, store.CreateLibraryVideoParams{WorkspaceID: job.WorkspaceID, UserID: job.UserID, Title: job.Title,
				Filename: filename, ContentType: "video/mp4", SizeBytes: size, BlobKey: key, ChatID: job.ChatID, MessageID: job.MessageID, PersonaID: job.PersonaID,
				DurationMs: int32p(duration), Width: int32p(width), Height: int32p(height), PosterKey: poster})
			if err != nil {
				return err
			}
			lib = &item.ID
		}
		_, err := q.FinishChatVideoJob(ctx, store.FinishChatVideoJobParams{ID: job.ID, OutputBlobKey: &key, OutputFilename: &filename, OutputContentType: strp("video/mp4"),
			OutputSizeBytes: &size, OutputCodec: info.Codec, OutputDurationMs: int32p(duration), LibraryItemID: lib})
		return err
	})
	if err != nil {
		r.Log.Warn("record chat video", "video", job.ID, "err", err)
		_ = r.Blobs.Delete(context.WithoutCancel(ctx), r.Cfg.AttachmentBucket, key)
		if poster != nil {
			_ = r.Blobs.Delete(context.WithoutCancel(ctx), r.Cfg.AttachmentBucket, *poster)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			r.failChatVideo(ctx, job, runs.VideoFailureError, "The video couldn't be saved.")
		}
	}
}

// copyVideoPoster gives the Library item its own copy of the job's poster:
// the poster frame, else the middle still, else the last preview.
func (r *Runner) copyVideoPoster(ctx context.Context, job store.ChatVideoJob) *string {
	if r.Attachments == nil {
		return nil
	}
	frames, err := r.q().ChatVideoFrames(ctx, []uuid.UUID{job.ID})
	if err != nil {
		return nil
	}
	var stills []store.ChatVideoFrame
	var pick *store.ChatVideoFrame
	for i, f := range frames {
		switch f.Kind {
		case "poster":
			pick = &frames[i]
		case "still":
			stills = append(stills, f)
		}
	}
	if pick == nil && len(stills) > 0 {
		pick = &stills[len(stills)/2]
	}
	if pick == nil {
		for i, f := range frames {
			if f.Kind == "preview" {
				pick = &frames[i]
			}
		}
	}
	if pick == nil {
		return nil
	}
	o, err := r.Attachments.Get(ctx, r.Cfg.AttachmentBucket, pick.BlobKey)
	if err != nil {
		return nil
	}
	defer o.Body.Close()
	key := fmt.Sprintf("library-posters/%s/%s/%s", job.WorkspaceID, job.UserID, uuid.NewString())
	if err := r.Blobs.Put(ctx, r.Cfg.AttachmentBucket, key, io.LimitReader(o.Body, pick.SizeBytes), pick.SizeBytes, pick.ContentType); err != nil {
		return nil
	}
	return &key
}

var unsafeFilename = regexp.MustCompile(`[^a-z0-9]+`)

// videoFilename is a download name from the title, e.g. black-cat-rainy-street.mp4.
func videoFilename(title string) string {
	s := strings.Trim(unsafeFilename.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], "-")
	}
	if s == "" {
		s = "video"
	}
	return s + ".mp4"
}

func int32p(v int) *int32 { n := int32(v); return &n }

func strp(s string) *string { return &s }
