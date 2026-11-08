package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// referenceImage adds an image to the person's Library and marks it as the
// reference for role ('self', or 'persona' for persona).
func (e env) referenceImage(blobs *putBlobs, role string, persona *uuid.UUID, ct, body string) store.LibraryItem {
	e.t.Helper()
	key := runs.ChatAttachmentKey(e.ws, e.me.ID)
	if err := blobs.Put(e.ctx, "artifacts", key, strings.NewReader(body), int64(len(body)), ct); err != nil {
		e.t.Fatal(err)
	}
	it, err := e.q.CreateLibraryItem(e.ctx, store.CreateLibraryItemParams{WorkspaceID: e.ws, UserID: e.me.ID, Kind: "image", Title: "ref", Filename: "ref",
		ContentType: ct, SizeBytes: int64(len(body)), BlobKey: key})
	if err != nil {
		e.t.Fatal(err)
	}
	if it, err = e.q.SetLibraryReference(e.ctx, store.SetLibraryReferenceParams{ID: it.ID, WorkspaceID: e.ws, UserID: e.me.ID, Role: &role, PersonaID: persona}); err != nil {
		e.t.Fatal(err)
	}
	return it
}

func lastCodexLine(t *testing.T, logPath string) map[string]string {
	t.Helper()
	var out map[string]string
	for _, l := range readLog(t, logPath) {
		if l["harness"] == "codex" {
			out = l
		}
	}
	if out == nil {
		t.Fatal("no codex invocation")
	}
	return out
}

// A Claude chat is told which references exist; a generate_image request for
// them gets the reference files written into the Codex step's workspace and
// attached to its prompt as images, and only the ones it asked for.
func TestChatImageJobUsesReferences(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat-image")
	t.Setenv("FAKE_IMAGE_REFERENCES", "persona,person")
	logPath := filepath.Join(t.TempDir(), "harness.log")
	t.Setenv("FAKE_HARNESS_LOG", logPath)
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	e.runs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		return []chatmodels.Model{{ID: h + "-main", Default: true}}, nil
	}
	p := e.persona("Artist", false, true)
	e.referenceImage(blobs, "persona", &p.ID, "image/png", "persona-likeness")
	e.referenceImage(blobs, "self", nil, "image/jpeg", "person-likeness")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: "image: us at the beach"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("reply", func() bool { return e.chat(chat.ID).State == "idle" })

	var claude map[string]string
	for _, l := range readLog(t, logPath) {
		if l["harness"] == "claude" {
			claude = l
		}
	}
	if claude == nil || !strings.Contains(claude["full_prompt"], `references "persona" and "person"`) {
		t.Fatalf("claude turn wasn't told about the references: %+v", claude)
	}
	run := e.chatRun(chat.ID)
	var roles []string
	if err := e.pool.QueryRow(e.ctx, "SELECT reference_roles FROM chat_image_jobs WHERE run_id=$1", run.ID).Scan(&roles); err != nil || strings.Join(roles, ",") != "persona,person" {
		t.Fatalf("job roles %v %v", roles, err)
	}
	step := lastCodexLine(t, logPath)
	wantImages := ".lazyagents/references/persona.png=persona-likeness\n.lazyagents/references/person.jpg=person-likeness"
	if step["images"] != wantImages || step["request"] != "us at the beach" ||
		!strings.Contains(step["full_prompt"], ".lazyagents/references/persona.png shows the chat's persona; .lazyagents/references/person.jpg shows the person the chat is with") {
		t.Fatalf("codex step %+v", step)
	}
	if job := e.imageJob(run.ID); job.State != "done" {
		t.Fatalf("job %+v", job)
	}
	// No reference bytes reach the run's events.
	evs, _ := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: run.ID, Seq: 0})
	for _, ev := range evs {
		if strings.Contains(string(ev.Payload), "likeness") {
			t.Fatalf("reference in event %s", ev.Payload)
		}
	}

	// Asking for the persona only leaves the person's photo out of the step's
	// workspace (blanked, not just unattached).
	t.Setenv("FAKE_IMAGE_REFERENCES", "persona")
	e.say(chat.ID, "image: just you")
	step = lastCodexLine(t, logPath)
	if step["images"] != ".lazyagents/references/persona.png=persona-likeness" || strings.Contains(step["full_prompt"], "person.jpg") {
		t.Fatalf("persona-only step %+v", step)
	}
	if b, err := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), ".lazyagents/references/person.jpg")); err != nil || len(b) != 0 {
		t.Fatalf("person reference left in the workspace: %q %v", b, err)
	}
}

// generate_image refuses a reference the person hasn't set, so the model can
// ask again without it; nothing is queued.
func TestChatImageJobMissingReference(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_IMAGE_REFERENCES", "person")
	chat, logPath := e.imageChat(t, "ok", "image: me as a pirate")
	if reply := e.chatMsgs(chat.ID)[1].BodyMd; !strings.Contains(reply, "making it (<nil>)") {
		t.Fatalf("reply %q", reply)
	}
	var n int
	if err := e.pool.QueryRow(e.ctx, "SELECT count(*) FROM chat_image_jobs WHERE chat_id=$1", chat.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("jobs %d %v", n, err)
	}
	for _, l := range readLog(t, logPath) {
		if l["harness"] == "codex" {
			t.Fatalf("a step ran: %+v", l)
		}
	}
}

// A Codex chat that makes its own images gets the references in its
// workspace each turn, named in the prompt; a cleared one is blanked.
func TestCodexChatGetsReferences(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	logPath := filepath.Join(t.TempDir(), "harness.log")
	t.Setenv("FAKE_HARNESS_LOG", logPath)
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessCodex, Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	p := e.persona("Artist", false, true)
	e.referenceImage(blobs, "persona", &p.ID, "image/webp", "persona-likeness")
	self := e.referenceImage(blobs, "self", nil, "image/png", "person-likeness")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	reply := e.postSh(chat.ID, "cat .lazyagents/references/persona.webp .lazyagents/references/person.png")
	if !strings.Contains(reply, "persona-likenessperson-likeness") {
		t.Fatalf("reply %q", reply)
	}
	if l := lastCodexLine(t, logPath); !strings.Contains(l["full_prompt"], ".lazyagents/references/persona.webp shows you; .lazyagents/references/person.png shows the person") || l["images"] != "" {
		t.Fatalf("codex turn %+v", l)
	}

	if _, err := e.q.SetLibraryReference(e.ctx, store.SetLibraryReferenceParams{ID: self.ID, WorkspaceID: e.ws, UserID: e.me.ID}); err != nil {
		t.Fatal(err)
	}
	reply = e.postSh(chat.ID, "wc -c < .lazyagents/references/person.png")
	if !strings.Contains(reply, "): 0") {
		t.Fatalf("cleared reference kept: %q", reply)
	}
	_, note, _ := strings.Cut(lastCodexLine(t, logPath)["full_prompt"], "(Vesper: reference images")
	if strings.Contains(note, "person.png") || !strings.Contains(note, "persona.webp shows you") {
		t.Fatalf("codex turn after clearing: note %q", note)
	}
}
