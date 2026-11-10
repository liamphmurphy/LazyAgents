package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// imageChat is a Claude chat whose persona has images on, with Codex
// available in the catalog; the fake Claude turn asks for an image.
func (e env) imageChat(t *testing.T, step string, prompt string) (store.ChatSession, string) {
	t.Helper()
	chat, logPath := e.startImageChat(t, step, prompt)
	e.until("reply", func() bool { s := e.chat(chat.ID).State; return s == "idle" || s == "failed" })
	return chat, logPath
}

func (e env) startImageChat(t *testing.T, step string, prompt string) (store.ChatSession, string) {
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
	if err := e.pool.QueryRow(e.ctx, "SELECT id, state, model, failure_kind, outbox_files, attachment_id, followup_turn_id FROM chat_image_jobs WHERE run_id=$1", runID).
		Scan(&j.ID, &j.State, &j.Model, &j.FailureKind, &j.OutboxFiles, &j.AttachmentID, &j.FollowupTurnID); err != nil {
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

// turns lists a run's turns, oldest first.
func (e env) turns(runID uuid.UUID) []store.RunTurn {
	e.t.Helper()
	ts, err := e.q.ListTurns(e.ctx, runID)
	if err != nil {
		e.t.Fatal(err)
	}
	return ts
}

// A Claude chat asks for an image: after its reply the runner makes it with a
// one-shot Codex step in the same workspace, attaches it under the reply, and
// leaves the chat's own harness session alone. One automatic follow-up turn
// then shows the assistant the image with its prompt to react to.
func TestChatImageJobMakesImageWithCodex(t *testing.T) {
	e := setup(t)
	chat, logPath := e.imageChat(t, "ok", "image: a red fox")
	msgs := e.chatMsgs(chat.ID)
	run := e.chatRun(chat.ID)
	if len(msgs) != 3 || msgs[1].Role != "assistant" || !strings.Contains(msgs[1].BodyMd, "making it") || msgs[2].Role != "assistant" {
		t.Fatalf("messages %+v", msgs)
	}
	atts, err := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[1].ID})
	if err != nil || len(atts) != 1 || atts[0].Source != "generated" || atts[0].ContentType != "image/png" || !strings.HasPrefix(atts[0].Filename, "exec-") {
		t.Fatalf("attachments %+v %v", atts, err)
	}
	obj, err := e.runner.Attachments.Get(e.ctx, "artifacts", atts[0].BlobKey)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Body.Close()
	if _, err := png.Decode(obj.Body); err != nil {
		t.Fatalf("delivered image cannot be rendered: %v", err)
	}
	// The run keeps the chat harness's own session, and the chat its harness.
	claudeSession := strings.TrimSuffix(strings.TrimPrefix(msgs[1].BodyMd[strings.Index(msgs[1].BodyMd, "session="):], "session="), "): making it (true)")
	if run.HarnessSessionID == nil || *run.HarnessSessionID != claudeSession || run.Harness != agentdef.HarnessClaudeCode || run.State != "waiting" {
		t.Fatalf("run %+v want session %q", run, claudeSession)
	}
	job := e.imageJob(run.ID)
	if job.State != "done" || job.CodexSessionID == nil || *job.CodexSessionID == claudeSession || job.Model != "codex-main" || job.FailureKind != nil {
		t.Fatalf("job %+v", job)
	}
	// The job knows its outbox file and the attachment it became.
	if len(job.OutboxFiles) != 1 || job.OutboxFiles[0] != atts[0].Filename || job.AttachmentID == nil || *job.AttachmentID != atts[0].ID {
		t.Fatalf("job files %v attachment %v, want %s", job.OutboxFiles, job.AttachmentID, atts[0].ID)
	}
	// The follow-up turn has no user message; it got the image as a file with
	// its prompt, couldn't ask for another, and its reply is a normal one.
	turns := e.turns(run.ID)
	if len(turns) != 2 || turns[1].Trigger != runs.TriggerImageFollowup || job.FollowupTurnID == nil || *job.FollowupTurnID != turns[1].ID {
		t.Fatalf("turns %+v job %+v", turns, job)
	}
	if msgs[2].RunTurnID == nil || *msgs[2].RunTurnID != turns[1].ID {
		t.Fatalf("follow-up reply %+v", msgs[2])
	}
	follow := msgs[2].BodyMd
	for _, want := range []string{"seen=1", "isn't available while you react", `Prompt: "a red fox"`, "`.lazyagents/attachments/generated/turn-1/1-" + atts[0].Filename + "`", "Open each image file"} {
		if !strings.Contains(follow, want) {
			t.Fatalf("follow-up lacks %q:\n%s", want, follow)
		}
	}
	// The Claude turns and the Codex step all ran; the step has no session to
	// resume, and the follow-up resumed the chat's own.
	var step, followLog map[string]string
	for _, l := range readLog(t, logPath) {
		if l["harness"] == "codex" {
			step = l
		} else if strings.HasPrefix(l["prompt"], "(Vesper: this is an automatic follow-up") {
			followLog = l
		}
	}
	if step == nil || step["resume"] != "" || step["model"] != "codex-main" || !strings.HasPrefix(step["prompt"], "Generate exactly one image") || step["request"] != "a red fox" {
		t.Fatalf("codex step %+v", step)
	}
	if followLog == nil || followLog["resume"] != claudeSession {
		t.Fatalf("follow-up turn %+v", followLog)
	}
	// Synthetic events carry no prompt; usage from all three processes is on the run.
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
	if run.InputTokens != 4000 || run.OutputTokens != 600 {
		t.Fatalf("usage %d/%d", run.InputTokens, run.OutputTokens)
	}
	// The next turn resumes the chat's own session, untouched by the step.
	if reply := e.say(chat.ID, "thanks"); !strings.Contains(reply.BodyMd, "thanks") || e.chatRun(chat.ID).ID != run.ID || *e.chatRun(chat.ID).HarnessSessionID != claudeSession {
		t.Fatalf("second turn %+v", reply)
	}
}

// A step that fails, makes nothing or is refused by moderation still leaves
// the reply, plus a plain system message saying why (a refusal says so), and
// the follow-up turn tells the assistant the image doesn't exist and why.
const fakeRefusal = "I can't create that image: the request was blocked by the safety system under the content policy."

func TestChatImageJobFailures(t *testing.T) {
	for _, tc := range []struct{ step, kind, reason, system, label string }{
		{"fail", "error", "Codex couldn't finish the image", "Couldn't create the image: Codex couldn't finish the image.", "Failed (error)"},
		// A clean exit with nothing made passes on what Codex said, the only
		// clue to why (a denied copy, an image saved elsewhere).
		{"empty", "error", "no image was produced; Codex said: Image step done", "Couldn't create the image: no image was produced; Codex said: Image step done.", "Failed (error)"},
		{"moderation", "moderation", "rejected by moderation: " + fakeRefusal, "The image request was rejected by moderation: " + fakeRefusal, "Failed (refused by moderation)"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			e := setup(t)
			chat, _ := e.imageChat(t, tc.step, "image: a fox")
			msgs := e.chatMsgs(chat.ID)
			if len(msgs) != 4 || msgs[1].Role != "assistant" || msgs[2].Role != "system" || msgs[2].BodyMd != tc.system || msgs[3].Role != "assistant" {
				t.Fatalf("messages %+v", msgs)
			}
			follow := msgs[3].BodyMd
			if !strings.Contains(follow, tc.label+": "+tc.reason+". No image exists") || !strings.Contains(follow, "seen=0") || strings.Contains(follow, "attachments/generated/") {
				t.Fatalf("follow-up:\n%s", follow)
			}
			atts, _ := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[1].ID})
			run := e.chatRun(chat.ID)
			job := e.imageJob(run.ID)
			if len(atts) != 0 || job.State != "failed" || job.Error == nil || *job.Error != tc.reason || job.FailureKind == nil || *job.FailureKind != tc.kind ||
				job.AttachmentID != nil || job.FollowupTurnID == nil || run.HarnessSessionID == nil || e.chat(chat.ID).State != "idle" {
				t.Fatalf("atts %v job %+v run %+v", atts, job, run)
			}
		})
	}
}

func TestQueuedImagesGetFollowupAfterChatTurnFails(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_FAIL_AFTER_IMAGE", "1")
	t.Setenv("FAKE_IMAGE_REQUESTS", "2")
	chat, _ := e.imageChat(t, "ok", "image: a fox")
	run := e.chatRun(chat.ID)
	turns := e.turns(run.ID)
	jobs, err := e.q.ChatImageJobsForTurn(e.ctx, store.ChatImageJobsForTurnParams{RunID: run.ID, TurnID: turns[0].ID})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("queued jobs: %+v, %v", jobs, err)
	}
	for _, job := range jobs {
		if job.State != "failed" || job.Error == nil || job.FailureKind == nil || *job.FailureKind != "error" || job.FollowupTurnID == nil {
			t.Fatalf("image request silently dropped after chat failure: %+v", job)
		}
	}
	msgs := e.chatMsgs(chat.ID)
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" || strings.Count(last.BodyMd, "Failed (error)") != 2 || !strings.Contains(last.BodyMd, "chat turn stopped") {
		t.Fatalf("missing per-request follow-up results: %+v", msgs)
	}
}

func TestTwoImageRequestsAreCopiedAndDelivered(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_IMAGE_REQUESTS", "2")
	chat, _ := e.imageChat(t, "ok", "image: a fox")
	msgs := e.chatMsgs(chat.ID)
	atts, err := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{msgs[1].ID})
	if err != nil || len(atts) != 2 {
		t.Fatalf("attachments: %+v, %v", atts, err)
	}
	if last := msgs[len(msgs)-1]; last.Role != "assistant" || !strings.Contains(last.BodyMd, "seen=2") {
		t.Fatalf("follow-up lost one of the images: %+v", msgs)
	}
}

func TestStoppedQueuedImagesAreReported(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "runner_restart"}[restart], func(t *testing.T) {
			e := setup(t)
			t.Setenv("FAKE_HARNESS_HOLD_AFTER_IMAGE", "1")
			t.Setenv("FAKE_IMAGE_REQUESTS", "2")
			chat, _ := e.startImageChat(t, "ok", "image: a fox")
			ctx, cancel := context.WithCancel(e.ctx)
			defer cancel()
			e.runner.Tick(ctx)
			e.runner.Wait() // workspace setup
			e.runner.Tick(ctx)
			run := e.chatRun(chat.ID)
			turns := e.turns(run.ID)
			deadline := time.Now().Add(10 * time.Second)
			for {
				n, err := e.q.CountChatImageJobsForTurn(e.ctx, store.CountChatImageJobsForTurnParams{RunID: run.ID, TurnID: turns[0].ID})
				if err != nil {
					t.Fatal(err)
				}
				if n == 2 {
					break
				}
				if time.Now().After(deadline) {
					cancel()
					e.runner.Wait()
					t.Fatal("image requests were not queued")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if _, err := e.runs.CancelChat(e.ctx, e.ws, e.me.ID, chat.ID); err != nil {
				t.Fatal(err)
			}
			if restart {
				cancel()
			}
			e.runner.Wait()
			if restart {
				if err := e.runner.Recover(e.ctx); err != nil {
					t.Fatal(err)
				}
			}
			jobs, err := e.q.ChatImageJobsForTurn(e.ctx, store.ChatImageJobsForTurnParams{RunID: run.ID, TurnID: turns[0].ID})
			if err != nil || len(jobs) != 2 {
				t.Fatalf("image jobs: %+v, %v", jobs, err)
			}
			for _, job := range jobs {
				if job.State != "failed" || job.Error == nil || *job.Error != "stopped" || job.FollowupTurnID != nil {
					t.Fatalf("stopped request lost or restarted: %+v", job)
				}
			}
			msgs := e.chatMsgs(chat.ID)
			if len(msgs) != 3 || msgs[1].BodyMd != "Couldn't create the image: stopped." || msgs[2].BodyMd != msgs[1].BodyMd || e.chat(chat.ID).State != "idle" {
				t.Fatalf("missing cancellation results: %+v", msgs)
			}
			if len(e.turns(run.ID)) != 1 {
				t.Fatal("Stop started an unsolicited image follow-up")
			}
		})
	}
}

// unreadableOutbox is a backend whose outbox can't be listed, like a Docker
// helper container that fails to start.
type unreadableOutbox struct{ runner.Backend }

func (unreadableOutbox) ReadOutbox(context.Context, string) ([]runner.OutboxFile, []string, error) {
	return nil, nil, errors.New("outbox container failed")
}

// An outbox the runner can't read is reported as such, not as a step that
// made nothing: the image may well be sitting there.
func TestChatImageJobUnreadableOutbox(t *testing.T) {
	e := setup(t)
	e.runner.Backend = unreadableOutbox{e.runner.Backend}
	chat, _ := e.imageChat(t, "ok", "image: a fox")
	job := e.imageJob(e.chatRun(chat.ID).ID)
	if job.State != "failed" || job.Error == nil || *job.Error != "couldn't read the workspace outbox" || job.FailureKind == nil || *job.FailureKind != "error" {
		t.Fatalf("job %+v", job)
	}
	msgs := e.chatMsgs(chat.ID)
	if last := msgs[len(msgs)-1]; last.Role != "assistant" || !strings.Contains(last.BodyMd, "Failed (error): couldn't read the workspace outbox. No image exists") {
		t.Fatalf("messages %+v", msgs)
	}
}

// A hung step is killed at the timeout and reported as such.
func TestChatImageJobTimeout(t *testing.T) {
	e := setup(t)
	e.runner.Cfg.ChatImageTimeout = 1500 * time.Millisecond
	chat, _ := e.imageChat(t, "hang", "image: a fox")
	msgs := e.chatMsgs(chat.ID)
	if len(msgs) != 4 || msgs[2].Role != "system" || msgs[2].BodyMd != "Couldn't create the image: timed out." || !strings.Contains(msgs[3].BodyMd, "Failed (error): timed out.") {
		t.Fatalf("messages %+v", msgs)
	}
	if job := e.imageJob(e.chatRun(chat.ID).ID); job.State != "failed" || *job.Error != "timed out" || *job.FailureKind != "error" {
		t.Fatalf("job %+v", job)
	}
}

// A turn that asked for no image gets no follow-up: the chat idles after its reply.
func TestChatWithoutImageJobsHasNoFollowup(t *testing.T) {
	e := setup(t)
	chat, _ := e.imageChat(t, "ok", "hello there")
	msgs := e.chatMsgs(chat.ID)
	turns := e.turns(e.chatRun(chat.ID).ID)
	if len(msgs) != 2 || msgs[1].Role != "assistant" || len(turns) != 1 || turns[0].Trigger != "comment" {
		t.Fatalf("messages %+v turns %+v", msgs, turns)
	}
}

// A follow-up whose harness fails ends the chat in the normal failed state.
func TestChatImageFollowupFailureFailsChat(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_FAIL_FOLLOWUP", "1")
	chat, _ := e.imageChat(t, "ok", "image: a fox")
	e.until("failed", func() bool { return e.chat(chat.ID).State == "failed" })
	msgs := e.chatMsgs(chat.ID)
	last := msgs[len(msgs)-1]
	if len(msgs) != 3 || msgs[1].Role != "assistant" || last.Role != "system" || e.chatRun(chat.ID).State != "waiting" {
		t.Fatalf("messages %+v", msgs)
	}
	// The person can carry on.
	if reply := e.say(chat.ID, "again?"); !strings.Contains(reply.BodyMd, "again?") {
		t.Fatalf("next turn %+v", reply)
	}
}
