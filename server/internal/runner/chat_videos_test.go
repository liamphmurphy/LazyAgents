package runner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// videoChat is a coding chat (a persona that can code, on a local repo) whose
// fake turn renders a video as step says.
func (e env) videoChat(t *testing.T, step string) (store.ChatSession, *putBlobs) {
	t.Helper()
	t.Setenv("FAKE_HARNESS_MODE", "chat-video")
	t.Setenv("FAKE_VIDEO_STEP", step)
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	bare, seed := filepath.Join(t.TempDir(), "origin.git"), t.TempDir()
	gitIn(t, seed, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, seed, "add", ".")
	gitIn(t, seed, "commit", "-qm", "init")
	gitIn(t, filepath.Dir(bare), "clone", "-q", "--bare", seed, bare)
	p := e.persona("Animator", true, false, "https://github.com/acme/app")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Videos", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `WITH attached AS (UPDATE chat_repositories SET repo_url=$2 WHERE chat_id=$1) UPDATE chat_sessions SET repo_url=$2 WHERE id=$1`, chat.ID, bare); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE personas SET repos=ARRAY[$2]::text[] WHERE id=$1`, p.ID, bare); err != nil {
		t.Fatal(err)
	}
	return chat, blobs
}

func (e env) videoJobs(chatID uuid.UUID) []store.ChatVideoJob {
	e.t.Helper()
	jobs, err := e.q.ListChatVideoJobs(e.ctx, &chatID)
	if err != nil {
		e.t.Fatal(err)
	}
	return jobs
}

// A coding chat renders a video: the runner copies its stills, filmstrip,
// preview and poster, stores the finished MP4 with what its boxes say, keeps
// it in the Library and puts the job on the turn's reply.
func TestChatVideoRendersAndSavesToLibrary(t *testing.T) {
	e := setup(t)
	chat, blobs := e.videoChat(t, "ok")
	reply := e.say(chat.ID, "video: Black cat on a rainy street")
	if !strings.Contains(reply.BodyMd, "Here's the render (save)") {
		t.Fatalf("reply %q", reply.BodyMd)
	}
	jobs := e.videoJobs(chat.ID)
	if len(jobs) != 1 {
		t.Fatalf("jobs %+v", jobs)
	}
	j := jobs[0]
	if j.State != "done" || j.Stage != runs.VideoStageSave || j.MessageID == nil || *j.MessageID != reply.ID || j.Error != nil {
		t.Fatalf("job %+v (reply %s)", j, reply.ID)
	}
	if j.OutputContentType == nil || *j.OutputContentType != "video/mp4" || j.OutputCodec != "H.264" || j.OutputDurationMs == nil || *j.OutputDurationMs != 2000 ||
		j.OutputFilename == nil || *j.OutputFilename != "black-cat-on-a-rainy-street.mp4" {
		t.Fatalf("output %+v", j)
	}
	if j.FramesDone != 60 || j.FramesTotal != 60 || j.StageNote != "H.264 (libx264, CRF 18)" || j.ScriptLines != 7 || !strings.HasSuffix(j.ScriptTail, "    main()") {
		t.Fatalf("progress %d/%d %q lines %d tail %q", j.FramesDone, j.FramesTotal, j.StageNote, j.ScriptLines, j.ScriptTail)
	}
	if strings.Count(string(j.Checks), `"passed": true`) != 2 || j.RenderStartedAt == nil {
		t.Fatalf("checks %s started %v", j.Checks, j.RenderStartedAt)
	}
	frames, err := e.q.ChatVideoFrames(e.ctx, []uuid.UUID{j.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range frames {
		got[fmt.Sprintf("%s/%d", f.Kind, f.Idx)] = true
		if _, ok := blobs.m["artifacts/"+f.BlobKey]; !ok {
			t.Fatalf("frame %s/%d not stored", f.Kind, f.Idx)
		}
	}
	for _, want := range []string{"still/0", "still/30", "strip/0", "strip/29", "preview/0", "poster/0"} {
		if !got[want] {
			t.Fatalf("frames %v missing %s", got, want)
		}
	}
	if len(frames) != 6 {
		t.Fatalf("%d frames: %v", len(frames), got)
	}
	// The Library keeps it with its own poster, on the reply.
	if j.LibraryItemID == nil {
		t.Fatal("no library item")
	}
	var kind, key string
	var poster *string
	var dur, w, h int32
	var msg *uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT kind, blob_key, poster_key, duration_ms, width, height, message_id FROM library_items WHERE id=$1`, *j.LibraryItemID).
		Scan(&kind, &key, &poster, &dur, &w, &h, &msg); err != nil {
		t.Fatal(err)
	}
	if kind != "video" || key != *j.OutputBlobKey || poster == nil || dur != 2000 || w != 640 || h != 360 || msg == nil || *msg != reply.ID {
		t.Fatalf("library %s %s %v %d %dx%d %v", kind, key, poster, dur, w, h, msg)
	}
	if _, ok := blobs.m["artifacts/"+*poster]; !ok {
		t.Fatal("poster not stored")
	}
	if _, ok := blobs.m["artifacts/"+key]; !ok {
		t.Fatal("video not stored")
	}
}

// fail_video ends the job at its stage with the reason and log tail.
func TestChatVideoAgentFails(t *testing.T) {
	e := setup(t)
	chat, _ := e.videoChat(t, "fail")
	reply := e.say(chat.ID, "video: Broken")
	j := e.videoJobs(chat.ID)[0]
	if j.State != "failed" || j.Stage != runs.VideoStageRender || j.Error == nil || *j.Error != "ffmpeg isn't installed in this workspace." ||
		j.FailureKind == nil || *j.FailureKind != runs.VideoFailureError || j.ErrorLog != "sh: 1: ffmpeg: not found" || j.MessageID == nil || *j.MessageID != reply.ID {
		t.Fatalf("job %+v", j)
	}
}

// A job the agent didn't finish fails when the reply ends, keeping what was copied.
func TestChatVideoUnfinishedFailsAtTurnEnd(t *testing.T) {
	e := setup(t)
	chat, _ := e.videoChat(t, "unfinished")
	e.say(chat.ID, "video: Half done")
	j := e.videoJobs(chat.ID)[0]
	if j.State != "failed" || j.Stage != runs.VideoStageEncode || j.Error == nil || !strings.Contains(*j.Error, "ended before") || j.FramesDone != 60 {
		t.Fatalf("job %+v", j)
	}
}

// finish_video without a file fails once the reply ends.
func TestChatVideoMissingFileFails(t *testing.T) {
	e := setup(t)
	chat, _ := e.videoChat(t, "empty")
	e.say(chat.ID, "video: Nothing")
	j := e.videoJobs(chat.ID)[0]
	if j.State != "failed" || j.Error == nil || !strings.Contains(*j.Error, "no video file") || j.LibraryItemID != nil {
		t.Fatalf("job %+v", j)
	}
}

// A runner without storage fails a finished video instead of crashing.
func TestChatVideoWithoutStorageFails(t *testing.T) {
	e := setup(t)
	chat, _ := e.videoChat(t, "ok")
	e.runner.Blobs, e.runner.Attachments = nil, nil
	e.say(chat.ID, "video: Nowhere to keep it")
	j := e.videoJobs(chat.ID)[0]
	if j.State != "failed" || j.Error == nil || !strings.Contains(*j.Error, "no storage") || j.LibraryItemID != nil {
		t.Fatalf("job %+v", j)
	}
}

// A chat that can't run code can't start a video.
func TestChatVideoNeedsCodingChat(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat-video")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	p := e.persona("Talker", false, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "No code", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	reply := e.say(chat.ID, "video: Nope")
	if !strings.Contains(reply.BodyMd, "start_video failed") || !strings.Contains(reply.BodyMd, "run code") || len(e.videoJobs(chat.ID)) != 0 {
		t.Fatalf("reply %q", reply.BodyMd)
	}
}
