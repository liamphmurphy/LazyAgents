package agentapi

import (
	"context"
	"path"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type StartVideoIn struct {
	Title           string  `json:"title" jsonschema:"What the video shows, 1–200 characters; the chat's card and the Library use it"`
	Width           int     `json:"width" jsonschema:"Frame width in pixels, e.g. 1920"`
	Height          int     `json:"height" jsonschema:"Frame height in pixels, e.g. 1080"`
	FPS             int     `json:"fps" jsonschema:"Frames per second, e.g. 30"`
	DurationSeconds float64 `json:"duration_seconds" jsonschema:"Planned length in seconds"`
	Script          string  `json:"script,omitempty" jsonschema:"The renderer's path relative to the workspace, once you know it; the chat shows its latest lines while you write it"`
}

type VideoCheckIn struct {
	Label  string `json:"label" jsonschema:"What you checked on the stills, e.g. Paws stay planted while the street scrolls"`
	Passed *bool  `json:"passed,omitempty" jsonschema:"true if it holds, false if not; omit while still checking"`
}

type UpdateVideoIn struct {
	VideoID string         `json:"video_id"`
	Stage   string         `json:"stage" jsonschema:"stills (the stills are in place), render (the full render is starting) or encode (frames are done, encoding the file)"`
	Checks  []VideoCheckIn `json:"checks,omitempty" jsonschema:"With stage stills: what you checked on them, up to 8; send again to tick them off"`
	Script  *string        `json:"script,omitempty" jsonschema:"The renderer's path, if it changed"`
}

type VideoIDIn struct {
	VideoID string `json:"video_id"`
}

type FailVideoIn struct {
	VideoID string `json:"video_id"`
	Error   string `json:"error" jsonschema:"One or two sentences for the person on what went wrong, e.g. ffmpeg isn't installed in this workspace"`
	Log     string `json:"log,omitempty" jsonschema:"The tail of the failing command's output (at most 16 KB is kept)"`
}

// videoChat is the chat run the caller belongs to, or an error for any other caller.
func videoChat(ctx context.Context, d Deps, c agents.Caller) error {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return err
	}
	if s == nil {
		return apperr.Conflict("the video tools only work in a chat with a person")
	}
	return nil
}

// videoFiles tells the agent where its renderer writes, relative to the workspace.
func videoFiles(job store.ChatVideoJob) map[string]any {
	dir := runs.VideoDir(job.ID)
	return map[string]any{
		"dir":      dir,
		"stills":   path.Join(dir, runs.VideoStillsDir) + "/<frame>.png",
		"progress": path.Join(dir, runs.VideoProgressFile),
		"preview":  path.Join(dir, runs.VideoPreviewFile),
		"strip":    path.Join(dir, runs.VideoStripDir) + "/<cell>.jpg",
		"poster":   path.Join(dir, runs.VideoPosterFile),
		"output":   path.Join(dir, runs.VideoOutputFile),
	}
}

func startVideo(ctx context.Context, d Deps, c agents.Caller, in StartVideoIn) (any, error) {
	if err := videoChat(ctx, d, c); err != nil {
		return nil, err
	}
	job, err := d.Runs.StartChatVideo(ctx, c.Agent.WorkspaceID, *c.RunID, runs.StartChatVideoInput{Title: in.Title, Width: in.Width, Height: in.Height, FPS: in.FPS, DurationSec: in.DurationSeconds, Script: in.Script})
	if err != nil {
		return nil, err
	}
	return map[string]any{"video_id": job.ID, "stage": job.Stage, "files": videoFiles(job),
		"tell_the_person": "The chat now shows a card for this video that follows its stages. Write the files where files says; Vesper copies them while you work."}, nil
}

func updateVideo(ctx context.Context, d Deps, c agents.Caller, in UpdateVideoIn) (any, error) {
	if err := videoChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.VideoID)
	if err != nil {
		return nil, apperr.Invalid("video_id", "not a video id")
	}
	var checks []runs.VideoCheck
	if in.Checks != nil {
		checks = make([]runs.VideoCheck, len(in.Checks))
		for i, ch := range in.Checks {
			checks[i] = runs.VideoCheck{Label: ch.Label, Passed: ch.Passed}
		}
	}
	job, err := d.Runs.UpdateChatVideo(ctx, c.Agent.WorkspaceID, *c.RunID, id, in.Stage, checks, in.Script)
	if err != nil {
		return nil, err
	}
	return map[string]any{"video_id": job.ID, "stage": job.Stage, "files": videoFiles(job)}, nil
}

func finishVideo(ctx context.Context, d Deps, c agents.Caller, in VideoIDIn) (any, error) {
	if err := videoChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.VideoID)
	if err != nil {
		return nil, apperr.Invalid("video_id", "not a video id")
	}
	job, err := d.Runs.FinishChatVideo(ctx, c.Agent.WorkspaceID, *c.RunID, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"video_id": job.ID, "stage": job.Stage,
		"tell_the_person": "Vesper is saving " + videoFiles(job)["output"].(string) + " to this chat and the person's Library; the card shows the player once it's stored, or why it couldn't be. Don't link or embed the file in your reply."}, nil
}

func failVideo(ctx context.Context, d Deps, c agents.Caller, in FailVideoIn) (any, error) {
	if err := videoChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.VideoID)
	if err != nil {
		return nil, apperr.Invalid("video_id", "not a video id")
	}
	job, err := d.Runs.FailChatVideo(ctx, c.Agent.WorkspaceID, *c.RunID, id, in.Error, in.Log)
	if err != nil {
		return nil, err
	}
	return map[string]any{"video_id": job.ID, "state": job.State, "stage": job.Stage}, nil
}
