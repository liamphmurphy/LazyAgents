package runs

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A chat video is rendered by the coding chat itself, with code, during its
// turn. The agent moves the job through its stages with the video tools; the
// renderer it wrote reports progress and preview frames as files in the job's
// directory, which the runner copies while the turn runs (runner/videos.go).

// Video stages, in order. A job may skip one (e.g. no stills) but never goes back.
const (
	VideoStageWrite  = "write"
	VideoStageStills = "stills"
	VideoStageRender = "render"
	VideoStageEncode = "encode"
	VideoStageSave   = "save"
)

var videoStageOrder = map[string]int{VideoStageWrite: 0, VideoStageStills: 1, VideoStageRender: 2, VideoStageEncode: 3, VideoStageSave: 4}

// VideoStageIndex is a stage's place in the pipeline, -1 if unknown.
func VideoStageIndex(stage string) int {
	if i, ok := videoStageOrder[stage]; ok {
		return i
	}
	return -1
}

const (
	// MaxChatVideoJobsPerTurn caps start_video calls in one reply.
	MaxChatVideoJobsPerTurn = 2
	// MaxVideoChecks bounds the checks the agent reports on its stills.
	MaxVideoChecks = 8
	// MaxVideoTitleRunes matches chat_video_jobs.title.
	MaxVideoTitleRunes = 200
	// MaxVideoErrorRunes and MaxVideoLogBytes bound what fail_video records.
	MaxVideoErrorRunes = 1000
	MaxVideoLogBytes   = 16 << 10
)

// The files a renderer writes, relative to the job's directory.
const (
	VideoProgressFile = "progress.json"
	VideoPreviewFile  = "preview.jpg"
	VideoPosterFile   = "poster.jpg"
	VideoStillsDir    = "stills"
	VideoStripDir     = "strip"
	VideoOutputFile   = "video.mp4"
	// VideoStripCells is the filmstrip's length: cell i is filled once frame
	// i*frames_total/VideoStripCells has rendered.
	VideoStripCells = 30
	// VideoMaxStills bounds the stills copied for one job.
	VideoMaxStills = 8
)

// Why a job failed: something went wrong, or the person stopped the reply.
const (
	VideoFailureError   = "error"
	VideoFailureStopped = "stopped"
)

var failureError = VideoFailureError

// VideoDir is a job's directory, relative to the workspace.
func VideoDir(id uuid.UUID) string { return path.Join(".lazyagents/videos", id.String()) }

// VideoCheck is one thing the agent checked on its stills; Passed nil means
// it hasn't been checked yet.
type VideoCheck struct {
	Label  string `json:"label"`
	Passed *bool  `json:"passed"`
}

// StartChatVideoInput is start_video's request.
type StartChatVideoInput struct {
	Title       string
	Width       int
	Height      int
	FPS         int
	DurationSec float64
	Script      string
}

// StartChatVideo opens a video job for the reply the chat is writing. Only a
// chat that may run code in its workspace can render one.
func (s *Service) StartChatVideo(ctx context.Context, ws, runID uuid.UUID, in StartChatVideoInput) (store.ChatVideoJob, error) {
	var job store.ChatVideoJob
	title := strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(title); n < 1 || n > MaxVideoTitleRunes {
		return job, apperr.Invalid("title", "provide 1–%d characters", MaxVideoTitleRunes)
	}
	if in.Width < 16 || in.Width > 7680 || in.Height < 16 || in.Height > 4320 {
		return job, apperr.Invalid("width", "width must be 16–7680 and height 16–4320 pixels")
	}
	if in.FPS < 1 || in.FPS > 240 {
		return job, apperr.Invalid("fps", "must be 1–240")
	}
	durMS := int(in.DurationSec * 1000)
	if durMS < 100 || durMS > 3600000 {
		return job, apperr.Invalid("duration_seconds", "must be 0.1–3600 seconds")
	}
	script, err := cleanVideoScript(in.Script)
	if err != nil {
		return job, err
	}
	q := store.New(s.Pool)
	run, chat, turn, err := s.videoTurn(ctx, q, ws, runID)
	if err != nil {
		return job, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tq := store.New(tx)
		if _, err := tq.LockChatSession(ctx, store.LockChatSessionParams{ID: chat.ID, WorkspaceID: ws}); err != nil {
			return err
		}
		n, err := tq.CountChatVideoJobsForTurn(ctx, store.CountChatVideoJobsForTurnParams{RunID: &run.ID, TurnID: &turn.ID})
		if err != nil {
			return err
		}
		if n >= MaxChatVideoJobsPerTurn {
			return apperr.Conflict("You can render at most %d videos in one reply; tell the person to ask again for more.", MaxChatVideoJobsPerTurn)
		}
		job, err = tq.CreateChatVideoJob(ctx, store.CreateChatVideoJobParams{WorkspaceID: ws, UserID: chat.CreatedBy, ChatID: &chat.ID, RunID: &run.ID, TurnID: &turn.ID,
			PersonaID: ChatSpeakerID(chat), Title: title, Width: int32(in.Width), Height: int32(in.Height), Fps: int32(in.FPS), DurationMs: int32(durMS), ScriptPath: script})
		return err
	})
	return job, err
}

// UpdateChatVideo moves a job on to stage (stills, render or encode),
// recording the checks the agent made on its stills and the renderer's path
// when given.
func (s *Service) UpdateChatVideo(ctx context.Context, ws, runID, id uuid.UUID, stage string, checks []VideoCheck, script *string) (store.ChatVideoJob, error) {
	if stage != VideoStageStills && stage != VideoStageRender && stage != VideoStageEncode {
		return store.ChatVideoJob{}, apperr.Invalid("stage", "use stills, render or encode; finish_video saves the video and fail_video ends it")
	}
	var raw json.RawMessage
	if checks != nil {
		if len(checks) > MaxVideoChecks {
			return store.ChatVideoJob{}, apperr.Invalid("checks", "at most %d", MaxVideoChecks)
		}
		clean := make([]VideoCheck, 0, len(checks))
		for _, c := range checks {
			label := strings.TrimSpace(c.Label)
			if n := utf8.RuneCountInString(label); n < 1 || n > 120 {
				return store.ChatVideoJob{}, apperr.Invalid("checks", "each label must be 1–120 characters")
			}
			clean = append(clean, VideoCheck{Label: label, Passed: c.Passed})
		}
		raw, _ = json.Marshal(clean)
	}
	if script != nil {
		p, err := cleanVideoScript(*script)
		if err != nil {
			return store.ChatVideoJob{}, err
		}
		script = &p
	}
	return s.moveChatVideo(ctx, ws, runID, id, func(q *store.Queries, job store.ChatVideoJob) (store.ChatVideoJob, error) {
		if VideoStageIndex(stage) < VideoStageIndex(job.Stage) {
			return job, apperr.Conflict("This video is already at %s; stages only move forward.", job.Stage)
		}
		return q.SetChatVideoStage(ctx, store.SetChatVideoStageParams{ID: job.ID, Stage: stage, Checks: raw, ScriptPath: script})
	})
}

// FinishChatVideo asks the runner to save the job's video file to the chat and
// the Library; the card shows it once it's stored.
func (s *Service) FinishChatVideo(ctx context.Context, ws, runID, id uuid.UUID) (store.ChatVideoJob, error) {
	return s.moveChatVideo(ctx, ws, runID, id, func(q *store.Queries, job store.ChatVideoJob) (store.ChatVideoJob, error) {
		return q.SetChatVideoStage(ctx, store.SetChatVideoStageParams{ID: job.ID, Stage: VideoStageSave})
	})
}

// FailChatVideo ends a job at the stage it reached, with why and the log tail.
func (s *Service) FailChatVideo(ctx context.Context, ws, runID, id uuid.UUID, reason, log string) (store.ChatVideoJob, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return store.ChatVideoJob{}, apperr.Invalid("error", "say what went wrong")
	}
	if r := []rune(reason); len(r) > MaxVideoErrorRunes {
		reason = string(r[:MaxVideoErrorRunes])
	}
	log = TailBytes(strings.TrimSpace(log), MaxVideoLogBytes)
	return s.moveChatVideo(ctx, ws, runID, id, func(q *store.Queries, job store.ChatVideoJob) (store.ChatVideoJob, error) {
		return q.FailChatVideoJob(ctx, store.FailChatVideoJobParams{ID: job.ID, Error: &reason, FailureKind: &failureError, ErrorLog: log})
	})
}

// moveChatVideo applies change to one of the run's running jobs under its row lock.
func (s *Service) moveChatVideo(ctx context.Context, ws, runID, id uuid.UUID, change func(*store.Queries, store.ChatVideoJob) (store.ChatVideoJob, error)) (store.ChatVideoJob, error) {
	var out store.ChatVideoJob
	q := store.New(s.Pool)
	if _, _, _, err := s.videoTurn(ctx, q, ws, runID); err != nil {
		return out, err
	}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tq := store.New(tx)
		job, err := tq.LockChatVideoJob(ctx, store.LockChatVideoJobParams{ID: id, RunID: &runID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("video_id", "not a video from this chat")
		}
		if err != nil {
			return err
		}
		if job.State != "running" {
			return apperr.Conflict("This video already %s; start a new one with start_video if the person wants another.", map[string]string{"done": "finished", "failed": "failed"}[job.State])
		}
		if job.Stage == VideoStageSave {
			return apperr.Conflict("This video is being saved; it shows in the chat once it's stored.")
		}
		out, err = change(tq, job)
		return err
	})
	return out, err
}

// videoTurn checks the run is a coding chat answering a message, returning the
// run, its chat and the turn being answered.
func (s *Service) videoTurn(ctx context.Context, q *store.Queries, ws, runID uuid.UUID) (store.Run, store.ChatSession, store.RunTurn, error) {
	run, err := q.GetRun(ctx, store.GetRunParams{ID: runID, WorkspaceID: ws})
	if err != nil {
		return run, store.ChatSession{}, store.RunTurn{}, err
	}
	if run.Purpose != "chat" {
		return run, store.ChatSession{}, store.RunTurn{}, apperr.Conflict("the video tools only work in a chat")
	}
	if run.State != "running" {
		return run, store.ChatSession{}, store.RunTurn{}, apperr.Conflict("the video tools only work while you are answering a message")
	}
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, chat, store.RunTurn{}, apperr.Conflict("this chat run is no longer attached to a chat")
	}
	if err != nil {
		return run, chat, store.RunTurn{}, err
	}
	caps, err := chatCaps(ctx, q, chat)
	if err != nil {
		return run, chat, store.RunTurn{}, err
	}
	if !caps.Code {
		return run, chat, store.RunTurn{}, apperr.Conflict("Rendering a video needs a chat that can run code in a repository; tell the person this chat can't.")
	}
	turn, err := q.RunningChatTurn(ctx, run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, chat, turn, apperr.Conflict("the video tools only work while you are answering a message")
	}
	return run, chat, turn, err
}

// cleanVideoScript checks the renderer's path: relative, inside the workspace.
func cleanVideoScript(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	clean := path.Clean(p)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || len(clean) > 300 {
		return "", apperr.Invalid("script", "give the renderer's path relative to the workspace")
	}
	return clean, nil
}

// TailBytes keeps the last max bytes of s, starting at a rune boundary.
func TailBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
