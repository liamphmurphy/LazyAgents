package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Chat audio rides the video sync's ticker (videos.go): a coding chat's
// audio renderer reports one progress file per job, and a finished file is
// stored once the agent calls finish_audio.

// AudioMaxOutputBytes caps a finished audio file; it matches the upload limit.
const AudioMaxOutputBytes = runs.MaxChatAttachmentBytes

// audioProgress is what a renderer writes to its job's progress.json.
type audioProgress struct {
	Percent int    `json:"percent"`
	Note    string `json:"note"`
}

func cleanAudioProgress(p audioProgress) audioProgress {
	p.Percent = min(max(p.Percent, 0), 100)
	p.Note = strings.TrimSpace(strings.ReplaceAll(p.Note, "\n", " "))
	if r := []rune(p.Note); len(r) > 120 {
		p.Note = string(r[:120])
	}
	if !utf8.ValidString(p.Note) {
		p.Note = ""
	}
	return p
}

// tickAudio copies the progress of each running audio job and, for one the
// agent finished, stores its file. final is the pass after the turn ended.
func (s *videoSync) tickAudio(ctx context.Context, final bool) {
	jobs, err := s.r.q().RunningChatAudioJobs(ctx, &s.c.run.ID)
	if err != nil {
		s.r.Log.Warn("chat audio", "run", s.c.run.ID, "err", err)
		return
	}
	for _, job := range jobs {
		s.syncAudioJob(ctx, job)
		if job.Stage == runs.AudioStageSave {
			s.r.saveChatAudio(ctx, s.c, job, final)
		}
	}
}

func (s *videoSync) syncAudioJob(ctx context.Context, job store.ChatAudioJob) {
	dir := runs.AudioDir(job.ID)
	if _, ok := s.stamps[job.ID]; !ok {
		s.stamps[job.ID] = map[string]string{}
	}
	known := s.stamps[job.ID]
	files, err := s.r.Backend.ReadVideoFiles(ctx, wsID(s.c.run), VideoFilesRequest{Dir: dir, Known: known})
	if err != nil {
		s.r.Log.Warn("read chat audio files", "run", s.c.run.ID, "audio", job.ID, "err", err)
		return
	}
	for _, f := range files {
		if f.Path != dir+"/"+runs.AudioProgressFile {
			continue
		}
		var p audioProgress
		if json.Unmarshal(f.Data, &p) != nil {
			continue // half-written: the next tick reads it again
		}
		p = cleanAudioProgress(p)
		known[f.Path] = f.Stamp
		if int32(p.Percent) == job.Percent && p.Note == job.StageNote {
			continue
		}
		if err := s.r.q().SetChatAudioProgress(ctx, store.SetChatAudioProgressParams{ID: job.ID, Percent: int32(p.Percent), StageNote: p.Note}); err != nil {
			s.r.Log.Warn("chat audio progress", "audio", job.ID, "err", err)
		}
	}
}

// failLeftoverAudio fails the audio jobs still running when the turn ended.
func (r *Runner) failLeftoverAudio(ctx context.Context, c runCtx) {
	jobs, err := r.q().RunningChatAudioJobs(ctx, &c.run.ID)
	if err != nil {
		r.Log.Warn("chat audio", "run", c.run.ID, "err", err)
		return
	}
	if len(jobs) == 0 {
		return
	}
	kind, reason := runs.VideoFailureError, "The reply ended before the audio was finished."
	if run, err := r.q().GetRun(ctx, store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID}); err == nil && run.State != "running" {
		kind, reason = runs.VideoFailureStopped, "Stopped before the audio was finished."
	}
	for _, j := range jobs {
		r.failChatAudio(ctx, j, kind, reason)
	}
}

func (r *Runner) failChatAudio(ctx context.Context, job store.ChatAudioJob, kind, reason string) {
	if _, err := r.q().FailChatAudioJob(ctx, store.FailChatAudioJobParams{ID: job.ID, Error: &reason, FailureKind: &kind}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		r.Log.Warn("fail chat audio", "audio", job.ID, "err", err)
	}
}

// saveChatAudio stores a finished job's file and keeps it in the Library. A
// file that isn't there yet is waited for until the turn ends (final).
func (r *Runner) saveChatAudio(ctx context.Context, c runCtx, job store.ChatAudioJob, final bool) {
	if r.Blobs == nil || r.Cfg.AttachmentBucket == "" {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "Vesper has no storage configured for audio, so it couldn't be saved.")
		return
	}
	var (
		f    io.ReadCloser
		size int64
		out  string
	)
	for _, p := range runs.AudioOutputPaths(job.ID) {
		file, n, err := r.Backend.OpenWorkspaceFile(ctx, wsID(c.run), p, AudioMaxOutputBytes)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio couldn't be read: "+err.Error())
			return
		}
		f, size, out = file, n, p
		break
	}
	if f == nil {
		if final {
			r.failChatAudio(ctx, job, runs.VideoFailureError, "There was no audio file in "+runs.AudioDir(job.ID)+" (audio.wav, audio.mp3, audio.ogg or audio.m4a).")
		}
		return
	}
	defer f.Close()
	if size == 0 {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio file is empty.")
		return
	}
	stored, err := r.q().ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: job.WorkspaceID, UserID: job.UserID})
	if err != nil {
		r.Log.Warn("library usage", "audio", job.ID, "err", err)
		return
	}
	if stored+size > runs.MaxGeneratedImageStorageBytes {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The person's Library is full (1 GB), so the audio couldn't be saved.")
		return
	}
	// Copied to a local file first: the decoder needs to seek.
	tmp, err := os.CreateTemp("", "vesper-audio-*")
	if err != nil {
		r.Log.Warn("chat audio temp file", "audio", job.ID, "err", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if n, err := io.Copy(tmp, io.LimitReader(f, size)); err != nil || n != size {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio couldn't be copied out of the workspace.")
		return
	}
	info, err := runs.ReadAudioInfo(out, tmp, size)
	if errors.Is(err, runs.ErrNotAudio) {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The file at "+out+" isn't the audio its extension says.")
		return
	}
	if err != nil {
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio couldn't be read: "+err.Error())
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return
	}
	filename := audioFilename(job.Title, out)
	key := fmt.Sprintf("chat-audio/%s/%s/audio-%s%s", job.WorkspaceID, job.ID, uuid.NewString()[:8], out[strings.LastIndex(out, "."):])
	if err := r.Blobs.Put(ctx, r.Cfg.AttachmentBucket, key, tmp, size, info.ContentType); err != nil {
		r.Log.Warn("store chat audio", "audio", job.ID, "err", err)
		r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio couldn't be stored.")
		return
	}
	duration := info.DurationMS
	if duration == 0 {
		duration = int(job.DurationMs) // a format the server can't decode: the planned length
	}
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var lib *uuid.UUID
		// A chat deleted meanwhile keeps nothing in the Library; the sweep
		// removes the job and its file.
		if job.ChatID != nil {
			item, err := q.CreateLibraryAudio(ctx, store.CreateLibraryAudioParams{WorkspaceID: job.WorkspaceID, UserID: job.UserID, Title: job.Title, Filename: filename,
				ContentType: info.ContentType, SizeBytes: size, BlobKey: key, Source: "generated", ChatID: job.ChatID, MessageID: job.MessageID, PersonaID: job.PersonaID,
				DurationMs: int32p(duration), Peaks: info.Peaks, Method: job.Method})
			if err != nil {
				return err
			}
			lib = &item.ID
		}
		_, err := q.FinishChatAudioJob(ctx, store.FinishChatAudioJobParams{ID: job.ID, OutputBlobKey: &key, OutputFilename: &filename, OutputContentType: strp(info.ContentType),
			OutputSizeBytes: &size, OutputDurationMs: int32p(duration), LibraryItemID: lib})
		return err
	})
	if err != nil {
		r.Log.Warn("record chat audio", "audio", job.ID, "err", err)
		_ = r.Blobs.Delete(context.WithoutCancel(ctx), r.Cfg.AttachmentBucket, key)
		if !errors.Is(err, pgx.ErrNoRows) {
			r.failChatAudio(ctx, job, runs.VideoFailureError, "The audio couldn't be saved.")
		}
	}
}

// audioFilename is a download name from the title and the file's extension,
// e.g. metal-riff-demo.wav.
func audioFilename(title, out string) string {
	s := strings.Trim(unsafeFilename.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], "-")
	}
	if s == "" {
		s = "audio"
	}
	return s + out[strings.LastIndex(out, "."):]
}
