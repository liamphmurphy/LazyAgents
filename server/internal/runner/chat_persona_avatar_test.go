package runner_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// proposalChat is a Claude chat, with Codex available to make images, whose
// persona may create personas and, with images, make pictures.
func (e env) proposalChat(t *testing.T, images bool, prompt string) store.ChatSession {
	t.Helper()
	t.Setenv("FAKE_HARNESS_MODE", "chat-image")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	e.runs.ModelDiscovery = func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		return []chatmodels.Model{{ID: h + "-main", Default: true}}, nil
	}
	maker := e.persona("Maker", false, images)
	yes := true
	if _, err := (&personas.Service{Pool: e.pool}).Update(e.ctx, personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID}, maker.ID, personas.Patch{Revision: maker.Revision, Personas: &yes}); err != nil {
		t.Fatal(err)
	}
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Make one", PersonaID: &maker.ID, Message: &runs.ChatMessageInput{Body: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("reply", func() bool { s := e.chat(chat.ID).State; return s == "idle" || s == "failed" })
	return chat
}

func (e env) onlyProposal(chatID uuid.UUID) store.PersonaProposal {
	e.t.Helper()
	ps, err := e.runs.ListPersonaProposals(e.ctx, e.ws, e.me.ID, chatID)
	if err != nil || len(ps) != 1 {
		e.t.Fatalf("proposals %+v %v", ps, err)
	}
	return ps[0]
}

func (e env) proposalAvatar(id uuid.UUID) (store.PersonaProposalAvatarsRow, bool) {
	e.t.Helper()
	m, err := runs.PersonaProposalAvatars(e.ctx, e.q, []uuid.UUID{id})
	if err != nil {
		e.t.Fatal(err)
	}
	a, ok := m[id]
	return a, ok
}

// A persona proposes another with a picture: the image is made with the
// turn, approval waits for it, and approving makes it the new persona's
// reference likeness and returns it for the avatar.
func TestPersonaProposalWithAvatar(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_IMAGE_STEP", "ok")
	chat := e.proposalChat(t, true, "persona: Poet | a portrait of a poet with a quill")
	prop := e.onlyProposal(chat.ID)
	if prop.AvatarJobID == nil {
		t.Fatal("the proposal queued no picture")
	}
	var prompt string
	if err := e.pool.QueryRow(e.ctx, "SELECT prompt FROM chat_image_jobs WHERE id = $1", *prop.AvatarJobID).Scan(&prompt); err != nil || prompt != "a portrait of a poet with a quill" {
		t.Fatalf("job prompt %q %v", prompt, err)
	}
	avatar, ok := e.proposalAvatar(prop.ID)
	if !ok || avatar.AvatarState != runs.ProposalAvatarReady || avatar.AttachmentID == nil {
		t.Fatalf("after the turn: %+v %v", avatar, ok)
	}

	// While the picture is still being made (its turn hasn't finished),
	// approving is refused, and nothing is created.
	if _, err := e.pool.Exec(e.ctx, `UPDATE chat_image_jobs SET attachment_id = NULL, state = 'running' WHERE id = $1`, *prop.AvatarJobID); err != nil {
		t.Fatal(err)
	}
	var turnID uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `UPDATE run_turns SET ended_at = NULL FROM chat_image_jobs j WHERE j.id = $1 AND run_turns.id = j.turn_id RETURNING run_turns.id`, *prop.AvatarJobID).Scan(&turnID); err != nil {
		t.Fatal(err)
	}
	if a, _ := e.proposalAvatar(prop.ID); a.AvatarState != runs.ProposalAvatarPending {
		t.Fatalf("mid-turn state %q", a.AvatarState)
	}
	var conflict *apperr.ConflictError
	if _, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, prop.ID, true); !errors.As(err, &conflict) || !strings.Contains(err.Error(), "still being made") {
		t.Fatalf("approved a pending picture: %v", err)
	}
	if p := e.onlyProposal(chat.ID); p.State != "pending" {
		t.Fatalf("refused approval left it %s", p.State)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE chat_image_jobs SET attachment_id = $2, state = 'done' WHERE id = $1`, *prop.AvatarJobID, *avatar.AttachmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE run_turns SET ended_at = now() WHERE id = $1`, turnID); err != nil {
		t.Fatal(err)
	}

	d, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, prop.ID, true)
	if err != nil || d.Proposal.PersonaID == nil || d.Avatar == nil || d.Avatar.ID != *avatar.AttachmentID {
		t.Fatalf("approve: %+v %v", d, err)
	}
	var ref string
	var refPersona *uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT coalesce(reference, ''), reference_persona_id FROM library_items WHERE blob_key = $1`, d.Avatar.BlobKey).Scan(&ref, &refPersona); err != nil {
		t.Fatal(err)
	}
	if ref != "persona" || refPersona == nil || *refPersona != *d.Proposal.PersonaID {
		t.Fatalf("library reference %q for %v, want persona for %v", ref, refPersona, *d.Proposal.PersonaID)
	}
	// Its chats now get its likeness when it asks for its own image.
	refs, err := runs.ChatReferences(e.ctx, e.q, store.ChatSession{WorkspaceID: e.ws, CreatedBy: e.me.ID, PersonaID: d.Proposal.PersonaID})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := refs[runs.ReferencePersona]; !ok {
		t.Fatalf("no persona reference for the new persona: %+v", refs)
	}
}

// A picture that wasn't made doesn't block the persona: approving creates it
// without one.
func TestPersonaProposalAvatarFailed(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_IMAGE_STEP", "moderation")
	chat := e.proposalChat(t, true, "persona: Poet | a portrait")
	prop := e.onlyProposal(chat.ID)
	if a, ok := e.proposalAvatar(prop.ID); !ok || a.AvatarState != runs.ProposalAvatarFailed || a.AttachmentID != nil {
		t.Fatalf("avatar %+v %v", a, ok)
	}
	d, err := e.runs.DecidePersonaProposal(e.ctx, e.ws, e.me.ID, chat.ID, prop.ID, true)
	if err != nil || d.Proposal.PersonaID == nil || d.Avatar != nil {
		t.Fatalf("approve: %+v %v", d, err)
	}
}

// Without the image permission, avatar_prompt is refused, so neither a
// proposal nor an image job is left behind.
func TestPersonaProposalAvatarNeedsImages(t *testing.T) {
	e := setup(t)
	chat := e.proposalChat(t, false, "persona: Poet | a portrait")
	if last := e.lastAssistant(chat.ID); !strings.Contains(last.BodyMd, "avatar_prompt can't be used") {
		t.Fatalf("reply %q", last.BodyMd)
	}
	var props, jobs int
	if err := e.pool.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM persona_proposals WHERE chat_id = $1), (SELECT count(*) FROM chat_image_jobs WHERE chat_id = $1)`, chat.ID).Scan(&props, &jobs); err != nil || props != 0 || jobs != 0 {
		t.Fatalf("left %d proposals, %d jobs (%v)", props, jobs, err)
	}
}
