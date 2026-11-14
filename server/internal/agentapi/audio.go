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

type StartAudioIn struct {
	Title           string  `json:"title" jsonschema:"What the audio is, 1–200 characters; the chat's card and the Library use it"`
	DurationSeconds float64 `json:"duration_seconds" jsonschema:"Planned length in seconds"`
	Method          string  `json:"method,omitempty" jsonschema:"One line on how it's made, e.g. MIDI → FluidSynth (GeneralUser GS); the card and the Library show it"`
	Script          string  `json:"script,omitempty" jsonschema:"The renderer's path relative to the workspace, once you know it"`
}

type AudioCheckIn struct {
	Label  string `json:"label" jsonschema:"What you measured on the render, e.g. Peak level stays under -1 dBFS. You can't hear audio, so check numbers: length, peak level, silence"`
	Passed *bool  `json:"passed,omitempty" jsonschema:"true if it holds, false if not; omit while still checking"`
}

type UpdateAudioIn struct {
	AudioID string         `json:"audio_id"`
	Stage   string         `json:"stage" jsonschema:"render (the renderer is running) or check (the file is written and you are measuring it)"`
	Checks  []AudioCheckIn `json:"checks,omitempty" jsonschema:"With stage check: what you measured, up to 8; send again to tick them off"`
	Script  *string        `json:"script,omitempty" jsonschema:"The renderer's path, if it changed"`
	Method  *string        `json:"method,omitempty" jsonschema:"How it's made, if that changed"`
}

type AudioIDIn struct {
	AudioID string `json:"audio_id"`
}

type FailAudioIn struct {
	AudioID string `json:"audio_id"`
	Error   string `json:"error" jsonschema:"One or two sentences for the person on what went wrong, e.g. fluidsynth isn't installed in this workspace"`
	Log     string `json:"log,omitempty" jsonschema:"The tail of the failing command's output (at most 16 KB is kept)"`
}

// audioChat is the chat run the caller belongs to, or an error for any other caller.
func audioChat(ctx context.Context, d Deps, c agents.Caller) error {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return err
	}
	if s == nil {
		return apperr.Conflict("the audio tools only work in a chat with a person")
	}
	return nil
}

// audioFiles tells the agent where its renderer writes, relative to the workspace.
func audioFiles(job store.ChatAudioJob) map[string]any {
	dir := runs.AudioDir(job.ID)
	return map[string]any{
		"dir":      dir,
		"progress": path.Join(dir, runs.AudioProgressFile),
		"output":   "one of " + join(runs.AudioOutputPaths(job.ID)) + ": WAV, MP3, OGG or M4A, up to 25 MB; WAV and MP3 get a waveform and length in the Library",
	}
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func startAudio(ctx context.Context, d Deps, c agents.Caller, in StartAudioIn) (any, error) {
	if err := audioChat(ctx, d, c); err != nil {
		return nil, err
	}
	job, err := d.Runs.StartChatAudio(ctx, c.Agent.WorkspaceID, *c.RunID, runs.StartChatAudioInput{Title: in.Title, DurationSec: in.DurationSeconds, Method: in.Method, Script: in.Script})
	if err != nil {
		return nil, err
	}
	return map[string]any{"audio_id": job.ID, "stage": job.Stage, "files": audioFiles(job),
		"tell_the_person": "The chat now shows a card for this audio that follows its stages. Write the files where files says; Vesper copies the progress file while you work. You can't hear it, so check numbers (length, peak level, silence) and say plainly that you couldn't listen."}, nil
}

func updateAudio(ctx context.Context, d Deps, c agents.Caller, in UpdateAudioIn) (any, error) {
	if err := audioChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.AudioID)
	if err != nil {
		return nil, apperr.Invalid("audio_id", "not an audio id")
	}
	var checks []runs.AudioCheck
	if in.Checks != nil {
		checks = make([]runs.AudioCheck, len(in.Checks))
		for i, ch := range in.Checks {
			checks[i] = runs.AudioCheck{Label: ch.Label, Passed: ch.Passed}
		}
	}
	job, err := d.Runs.UpdateChatAudio(ctx, c.Agent.WorkspaceID, *c.RunID, id, in.Stage, checks, in.Script, in.Method)
	if err != nil {
		return nil, err
	}
	return map[string]any{"audio_id": job.ID, "stage": job.Stage, "files": audioFiles(job)}, nil
}

func finishAudio(ctx context.Context, d Deps, c agents.Caller, in AudioIDIn) (any, error) {
	if err := audioChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.AudioID)
	if err != nil {
		return nil, apperr.Invalid("audio_id", "not an audio id")
	}
	job, err := d.Runs.FinishChatAudio(ctx, c.Agent.WorkspaceID, *c.RunID, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"audio_id": job.ID, "stage": job.Stage,
		"tell_the_person": "Vesper is saving the file in " + runs.AudioDir(job.ID) + " to this chat and the person's Library; the card shows the player once it's stored, or why it couldn't be. Don't link or embed the file in your reply."}, nil
}

func failAudio(ctx context.Context, d Deps, c agents.Caller, in FailAudioIn) (any, error) {
	if err := audioChat(ctx, d, c); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.AudioID)
	if err != nil {
		return nil, apperr.Invalid("audio_id", "not an audio id")
	}
	job, err := d.Runs.FailChatAudio(ctx, c.Agent.WorkspaceID, *c.RunID, id, in.Error, in.Log)
	if err != nil {
		return nil, err
	}
	return map[string]any{"audio_id": job.ID, "state": job.State, "stage": job.Stage}, nil
}
