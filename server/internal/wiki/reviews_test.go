package wiki_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestPageReviewRebasesAndPreservesHumanEdits(t *testing.T) {
	e := setup(t)
	ctx := pageview.Person(e.ctx, e.me.ID)
	base := e.save(t, "Reviews/Test", "# Review\n\nFirst.\n\nMiddle.\n\nLast.\n")
	chat, err := e.q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: e.ws, CreatedBy: e.me.ID, AgentID: e.atlas.ID, Title: "Review conversation", Harness: "codex", Model: "test", Effort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.q.SetPageChat(ctx, store.SetPageChatParams{ID: base.Note.ID, WorkspaceID: e.ws, Viewer: &e.me.ID, ChatID: &chat.ID}); err != nil {
		t.Fatal(err)
	}
	proposed, err := e.svc.AgentWrite(ctx, e.atlas, map[string]agentdef.Mode{"wiki:write": agentdef.Allowed}, wiki.Save{Path: *base.Note.Path, Body: "# Review\n\nFirst changed.\n\nMiddle.\n\nLast changed.\n", ForceSuggestion: true})
	if err != nil {
		t.Fatal(err)
	}
	if proposed.State != artifacts.Suggested {
		t.Fatal(proposed.State)
	}
	var changes []artifacts.PageChange
	json.Unmarshal(proposed.Version.Changes, &changes)
	if len(changes) != 2 {
		t.Fatalf("changes %+v", changes)
	}
	e.save(t, *base.Note.Path, "# Review\n\nHuman changed first.\n\nMiddle.\n\nLast.\n")
	_, err = e.svc.ResolveChanges(ctx, e.ws, e.me.ID, base.Note.ID, proposed.Version.ID, []wiki.ReviewDecision{{Index: 0, Accept: true}, {Index: 1, Accept: true}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.q.GetArtifact(ctx, store.GetArtifactParams{ID: base.Note.ID, WorkspaceID: e.ws, Viewer: &e.me.ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: e.ws, Viewer: &e.me.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*v.BodyMd, "Human changed first.") || !strings.Contains(*v.BodyMd, "Last changed.") {
		t.Fatal(*v.BodyMd)
	}
	if v.CreatedByType != "user" {
		t.Fatal(v.CreatedByType)
	}
	reviewed, _ := e.q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: proposed.Version.ID, ArtifactID: a.ID, WorkspaceID: e.ws, Viewer: &e.me.ID})
	json.Unmarshal(reviewed.Changes, &changes)
	if changes[0].State != "stale" || changes[1].State != "accepted" {
		t.Fatalf("changes %+v", changes)
	}
	if _, err = e.svc.ResolveChanges(ctx, e.ws, e.me.ID, a.ID, proposed.Version.ID, []wiki.ReviewDecision{{Index: 0, Accept: true}}); err == nil {
		t.Fatal("repeated review accepted")
	}
	messages, err := e.q.ListChatMessages(ctx, store.ListChatMessagesParams{SessionID: chat.ID, Seq: 0, Limit: 10})
	if err != nil || len(messages) != 1 || !strings.Contains(messages[0].BodyMd, "stale") {
		t.Fatalf("review feedback missing: %+v %v", messages, err)
	}
	restored, err := e.svc.RestorePage(ctx, e.ws, e.me.ID, a.ID, base.Version.ID)
	if err != nil || *restored.Version.BodyMd != *base.Version.BodyMd {
		t.Fatalf("restore %+v %v", restored, err)
	}
}

func TestPrivateDirectReviewAndPause(t *testing.T) {
	e := setup(t)
	ctx := pageview.Person(e.ctx, e.me.ID)
	base, err := e.svc.SaveAsUser(ctx, e.ws, e.me.ID, wiki.Save{Path: "Private review", Body: "original\n"})
	if err != nil {
		t.Fatal(err)
	}
	edit, err := e.svc.AgentWrite(ctx, e.atlas, nil, wiki.Save{Path: *base.Note.Path, Body: "persona\n", BaseID: &base.Version.ID})
	if err != nil || edit.State != artifacts.Published {
		t.Fatalf("edit %+v %v", edit, err)
	}
	if edit.Version.ReviewState == nil || *edit.Version.ReviewState != "pending" {
		t.Fatal("missing direct review")
	}
	reverted, err := e.svc.ResolveChanges(ctx, e.ws, e.me.ID, base.Note.ID, edit.Version.ID, []wiki.ReviewDecision{{Index: 0, Accept: false, Reason: "Keep original"}})
	if err != nil || *reverted.BodyMd != "original\n" {
		t.Fatalf("revert %+v %v", reverted, err)
	}
	_, err = e.q.SetPageControls(ctx, store.SetPageControlsParams{ID: base.Note.ID, WorkspaceID: e.ws, Viewer: &e.me.ID, Direct: false})
	if err != nil {
		t.Fatal(err)
	}
	suggestion, err := e.svc.AgentWrite(ctx, e.atlas, nil, wiki.Save{Path: *base.Note.Path, Body: "suggestion\n"})
	if err != nil || suggestion.State != artifacts.Suggested {
		t.Fatalf("suggestion %+v %v", suggestion, err)
	}
	_, err = e.q.SetPageControls(ctx, store.SetPageControlsParams{ID: base.Note.ID, WorkspaceID: e.ws, Viewer: &e.me.ID, Paused: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.AgentWrite(ctx, e.atlas, nil, wiki.Save{Path: *base.Note.Path, Body: "refused"}); err == nil {
		t.Fatal("pause ignored")
	}
}
