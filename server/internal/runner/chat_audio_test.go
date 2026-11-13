package runner_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// audioChat is a coding chat whose fake turn renders audio as step says.
func (e env) audioChat(t *testing.T, step string) (store.ChatSession, *putBlobs) {
	t.Helper()
	chat, blobs := e.videoChat(t, "")
	t.Setenv("FAKE_HARNESS_MODE", "chat-audio")
	t.Setenv("FAKE_AUDIO_STEP", step)
	return chat, blobs
}

func (e env) audioJobs(chatID uuid.UUID) []store.ChatAudioJob {
	e.t.Helper()
	jobs, err := e.q.ListChatAudioJobs(e.ctx, &chatID)
	if err != nil {
		e.t.Fatal(err)
	}
	return jobs
}

// A coding chat renders audio: the runner copies its progress, stores the
// finished WAV with the length and waveform read from the file, keeps it in
// the Library and puts the job on the turn's reply.
func TestChatAudioRendersAndSavesToLibrary(t *testing.T) {
	e := setup(t)
	chat, blobs := e.audioChat(t, "ok")
	reply := e.say(chat.ID, "audio: Metal riff demo")
	if !strings.Contains(reply.BodyMd, "Here's the audio (save)") {
		t.Fatalf("reply %q", reply.BodyMd)
	}
	jobs := e.audioJobs(chat.ID)
	if len(jobs) != 1 {
		t.Fatalf("jobs %+v", jobs)
	}
	j := jobs[0]
	if j.State != "done" || j.Stage != runs.AudioStageSave || j.MessageID == nil || *j.MessageID != reply.ID || j.Error != nil {
		t.Fatalf("job state %s stage %s error %q (reply %s)", j.State, j.Stage, deref(j.Error), reply.ID)
	}
	if j.OutputContentType == nil || *j.OutputContentType != "audio/wav" || j.OutputDurationMs == nil || *j.OutputDurationMs != 2000 ||
		j.OutputFilename == nil || *j.OutputFilename != "metal-riff-demo.wav" {
		t.Fatalf("output %+v", j)
	}
	if j.Percent != 100 || j.Method != "numpy sine sweep" || j.ScriptPath != "tone.py" || strings.Count(string(j.Checks), `"passed": true`) != 2 {
		t.Fatalf("progress %d%% method %q script %q checks %s", j.Percent, j.Method, j.ScriptPath, j.Checks)
	}
	if j.LibraryItemID == nil {
		t.Fatal("no library item")
	}
	var kind, source, method string
	var key string
	var peaks []byte
	var dur int32
	var msg *uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT kind, source, blob_key, peaks, duration_ms, message_id, method FROM library_items WHERE id=$1`, *j.LibraryItemID).
		Scan(&kind, &source, &key, &peaks, &dur, &msg, &method); err != nil {
		t.Fatal(err)
	}
	if method != "numpy sine sweep" {
		t.Fatalf("library method %q", method)
	}
	if kind != "audio" || source != "generated" || key != *j.OutputBlobKey || dur != 2000 || msg == nil || *msg != reply.ID {
		t.Fatalf("library %s %s %s %d %v", kind, source, key, dur, msg)
	}
	// The fake tone fades in: the waveform must rise, ending at full scale.
	if len(peaks) != runs.AudioPeakBuckets || peaks[2] > 20 || peaks[len(peaks)-1] != 255 {
		t.Fatalf("peaks %v", peaks)
	}
	if _, ok := blobs.m["artifacts/"+key]; !ok {
		t.Fatal("audio not stored")
	}
}

// A format the server can't decode is still kept, with the planned length and no waveform.
func TestChatAudioUndecodableFormatKeepsNoWaveform(t *testing.T) {
	e := setup(t)
	chat, _ := e.audioChat(t, "ogg")
	e.say(chat.ID, "audio: Field recording")
	j := e.audioJobs(chat.ID)[0]
	if j.State != "done" || j.OutputContentType == nil || *j.OutputContentType != "audio/ogg" || j.OutputDurationMs == nil || *j.OutputDurationMs != 2000 {
		t.Fatalf("job %+v", j)
	}
	var peaks []byte
	if err := e.pool.QueryRow(e.ctx, `SELECT peaks FROM library_items WHERE id=$1`, *j.LibraryItemID).Scan(&peaks); err != nil || peaks != nil {
		t.Fatalf("peaks %v, %v", peaks, err)
	}
}

// fail_audio ends the job at its stage with the reason and log tail.
func TestChatAudioAgentFails(t *testing.T) {
	e := setup(t)
	chat, _ := e.audioChat(t, "fail")
	reply := e.say(chat.ID, "audio: Broken")
	j := e.audioJobs(chat.ID)[0]
	if j.State != "failed" || j.Stage != runs.AudioStageRender || j.Error == nil || *j.Error != "fluidsynth isn't installed in this workspace." ||
		j.FailureKind == nil || *j.FailureKind != runs.VideoFailureError || j.ErrorLog != "sh: 1: fluidsynth: not found" || j.MessageID == nil || *j.MessageID != reply.ID {
		t.Fatalf("job %+v", j)
	}
}

// A job the agent didn't finish fails when the reply ends, at the stage it reached.
func TestChatAudioUnfinishedFailsAtTurnEnd(t *testing.T) {
	e := setup(t)
	chat, _ := e.audioChat(t, "unfinished")
	e.say(chat.ID, "audio: Half done")
	j := e.audioJobs(chat.ID)[0]
	if j.State != "failed" || j.Stage != runs.AudioStageCheck || j.Error == nil || !strings.Contains(*j.Error, "ended before") || j.Percent != 80 {
		t.Fatalf("job %+v", j)
	}
}

// finish_audio without a file fails once the reply ends, naming where it looked.
func TestChatAudioFinishedWithoutAFileFails(t *testing.T) {
	e := setup(t)
	chat, _ := e.audioChat(t, "empty")
	e.say(chat.ID, "audio: Nothing written")
	j := e.audioJobs(chat.ID)[0]
	if j.State != "failed" || j.Error == nil || !strings.Contains(*j.Error, "no audio file") {
		t.Fatalf("job %+v", j)
	}
}

// A chat that can't run code can't start audio, and the agent is told why.
func TestChatAudioNeedsACodingChat(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat-audio")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	p := e.persona("Talker", false, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "No code", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	reply := e.say(chat.ID, "audio: Nope")
	if !strings.Contains(reply.BodyMd, "start_audio failed") || !strings.Contains(reply.BodyMd, "run code") || len(e.audioJobs(chat.ID)) != 0 {
		t.Fatalf("reply %q jobs %v", reply.BodyMd, e.audioJobs(chat.ID))
	}
}
