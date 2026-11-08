package runner_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/chatmodels"
	"github.com/liamphmurphy/lazyagents/server/internal/runs"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// imageChat is a Claude chat whose persona has images on, with Codex
// available in the catalog; the fake Claude turn asks for an image.
func (e env) imageChat(t *testing.T, step string, prompt string) (store.ChatSession, string) {
	t.Helper()
	t.Setenv("FAKE_HARNESS_MODE", "chat-image")
	t.Setenv("FAKE_IMAGE_STEP", step)
	logPath := filepath.Join(t.TempDir(), "harness.log")
	t.Setenv("FAKE_HARNESS_LOG", logPath)
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	e.runs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		return []chatmodels.Model{{ID: h + "-small"}, {ID: h + "-main", Default: true}}, nil
	}
	p := e.persona("Artist", false, true)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })
	return chat, logPath
}

func (e env) chatMsgs(id uuid.UUID) []store.ChatMessage {
	e.t.Helper()
	rows, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: id, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	out := make([]store.ChatMessage, len(rows))
	for i, r := range rows {
		out[i] = store.ChatMessage(r)
	}
	return out
}

func (e env) imageJob(runID uuid.UUID) store.ChatImageJob {
	e.t.Helper()
	var j store.ChatImageJob
	if err := e.pool.QueryRow(e.ctx, "SELECT id, state, coalesce(codex_session_id,''), coalesce(error,''), model FROM chat_image_jobs WHERE run_id=$1", runID).
		Scan(&j.ID, &j.State, new(string), new(string), &j.Model); err != nil {
		e.t.Fatal(err)
	}
	var sess, msg string
	if err := e.pool.QueryRow(e.ctx, "SELECT coalesce(codex_session_id,''), coalesce(error,'') FROM chat_image_jobs WHERE id=$1", j.ID).Scan(&sess, &msg); err != nil {
		e.t.Fatal(err)
	}
	if sess != "" {
		j.CodexSessionID = &sess
	}
	if msg != "" {
		j.Error = &msg
	}
	return j
}

// A Claude chat asks for an image: after its reply the runner makes it with a
// one-shot Codex step in the same workspace, attaches it under the reply, and
// leaves the chat's own harness session alone.
func TestChatImageJobMakesImageWithCodex(t *testing.T) {
	e := setup(t)
	chat, logPath := e.imageChat(t, "ok", "image: a red fox")
	msgs := e.chatMsgs(chat.ID)
	run := e.chatRun(chat.ID)
	if len(msgs) != 2 || msgs[1].Role != "assistant" || !strings.Contains(msgs[1].BodyMd, "making it") {
		t.Fatalf("messages %+v", msgs)
	}
	atts, err := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[1].ID})
	if err != nil || len(atts) != 1 || atts[0].Source != "generated" || atts[0].ContentType != "image/png" || !strings.HasPrefix(atts[0].Filename, "exec-") {
		t.Fatalf("attachments %+v %v", atts, err)
	}
	// The run keeps the chat harness's own session, and the chat its harness.
	claudeSession := strings.TrimSuffix(strings.TrimPrefix(msgs[1].BodyMd[strings.Index(msgs[1].BodyMd, "session="):], "session="), "): making it (true)")
	if run.HarnessSessionID == nil || *run.HarnessSessionID != claudeSession || run.Harness != agentdef.HarnessClaudeCode || run.State != "waiting" {
		t.Fatalf("run %+v want session %q", run, claudeSession)
	}
	job := e.imageJob(run.ID)
	if job.State != "done" || job.CodexSessionID == nil || *job.CodexSessionID == claudeSession || job.Model != "codex-main" {
		t.Fatalf("job %+v", job)
	}
	// The Claude turn and the Codex step both ran; the step has no session to resume.
	var step map[string]string
	for _, l := range readLog(t, logPath) {
		if l["harness"] == "codex" {
			step = l
		}
	}
	if step == nil || step["resume"] != "" || step["model"] != "codex-main" || !strings.HasPrefix(step["prompt"], "Generate exactly one image") || step["request"] != "a red fox" {
		t.Fatalf("codex step %+v", step)
	}
	// Synthetic events carry no prompt; usage from both processes is on the run.
	evs, _ := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: run.ID, Seq: 0})
	var calls, results int
	for _, ev := range evs {
		var p map[string]any
		_ = json.Unmarshal(ev.Payload, &p)
		if p["tool"] != "image_generation" {
			continue
		}
		if strings.Contains(string(ev.Payload), "red fox") || p["id"] != job.ID.String() {
			t.Fatalf("synthetic event %s", ev.Payload)
		}
		switch ev.Type {
		case "tool_call":
			calls++
		case "tool_result":
			results++
			if p["is_error"] != false {
				t.Fatalf("result %s", ev.Payload)
			}
		}
	}
	if calls != 1 || results != 1 {
		t.Fatalf("synthetic events: %d calls %d results", calls, results)
	}
	if run.InputTokens != 2500 || run.OutputTokens != 400 {
		t.Fatalf("usage %d/%d", run.InputTokens, run.OutputTokens)
	}
	// The next turn resumes the chat's own session, untouched by the step.
	if reply := e.say(chat.ID, "thanks"); !strings.Contains(reply.BodyMd, "thanks") || e.chatRun(chat.ID).ID != run.ID || *e.chatRun(chat.ID).HarnessSessionID != claudeSession {
		t.Fatalf("second turn %+v", reply)
	}
}

// A step that fails or makes nothing still leaves the reply, plus a plain
// system message saying why.
func TestChatImageJobFailures(t *testing.T) {
	for _, tc := range []struct{ step, reason string }{
		{"fail", "Codex couldn't finish the image"},
		{"empty", "no image was produced"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			e := setup(t)
			chat, _ := e.imageChat(t, tc.step, "image: a fox")
			msgs := e.chatMsgs(chat.ID)
			if len(msgs) != 3 || msgs[1].Role != "assistant" || msgs[2].Role != "system" || msgs[2].BodyMd != "Couldn't create the image: "+tc.reason+"." {
				t.Fatalf("messages %+v", msgs)
			}
			atts, _ := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[1].ID})
			run := e.chatRun(chat.ID)
			job := e.imageJob(run.ID)
			if len(atts) != 0 || job.State != "failed" || job.Error == nil || *job.Error != tc.reason || run.HarnessSessionID == nil || e.chat(chat.ID).State != "idle" {
				t.Fatalf("atts %v job %+v run %+v", atts, job, run)
			}
		})
	}
}

// A hung step is killed at the timeout and reported as such.
func TestChatImageJobTimeout(t *testing.T) {
	e := setup(t)
	e.runner.Cfg.ChatImageTimeout = 1500 * time.Millisecond
	chat, _ := e.imageChat(t, "hang", "image: a fox")
	msgs := e.chatMsgs(chat.ID)
	if len(msgs) != 3 || msgs[2].Role != "system" || msgs[2].BodyMd != "Couldn't create the image: timed out." {
		t.Fatalf("messages %+v", msgs)
	}
	if job := e.imageJob(e.chatRun(chat.ID).ID); job.State != "failed" || *job.Error != "timed out" {
		t.Fatalf("job %+v", job)
	}
}
